package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/aitoooooo/binlogx/pkg/checkpoint"
	"github.com/aitoooooo/binlogx/pkg/config"
	"github.com/aitoooooo/binlogx/pkg/filter"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/processor"
	"github.com/aitoooooo/binlogx/pkg/source"
	"github.com/aitoooooo/binlogx/pkg/util"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var parseCmd = &cobra.Command{
	Use:   "parse",
	Short: "Parse and display binlog events",
	Long:  "Parse binlog events with streaming output and interactive paging",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 保存原始的日志输出
		originalLogWriter := log.Writer()

		// 初始化配置
		cfg, err := config.InitConfig(cmd)
		if err != nil {
			return err
		}

		// 在交互模式下禁用标准日志输出（重定向到 /dev/null）
		log.SetOutput(io.Discard)

		// 获取 parse 命令的局部参数
		startLogFile, _ := cmd.Flags().GetString("start-log-file")
		startLogPos, _ := cmd.Flags().GetUint32("start-log-pos")

		// 如果只指定了 start-log-file，默认从该文件开头（位置 4）开始
		if startLogFile != "" && startLogPos == 0 {
			startLogPos = 4
		}

		// ========== 参数验证 ==========
		var validationErrors []string

		// 验证1: 断点续看参数只能用于在线 MySQL 数据源
		if startLogFile != "" && cfg.Source != "" {
			validationErrors = append(validationErrors,
				"❌ --start-log-file 只能与 --db-connection 一起使用，不能用于 --source（离线文件）")
		}

		// 验证2: 如果指定了 start-log-pos，必须同时指定 start-log-file
		if startLogPos > 0 && startLogFile == "" {
			validationErrors = append(validationErrors,
				"❌ 指定了 --start-log-pos 时必须同时指定 --start-log-file")
		}

		// 验证3: binlog 位置必须 >= 4（binlog 文件头占用前 4 字节）
		if startLogPos > 0 && startLogPos < 4 {
			validationErrors = append(validationErrors,
				"❌ --start-log-pos 必须 >= 4（binlog 文件头占用前 4 字节）")
		}

		// 如果有验证错误，显示错误信息并返回
		if len(validationErrors) > 0 {
			fmt.Fprintf(os.Stderr, "\n参数验证失败:\n\n")
			for _, err := range validationErrors {
				fmt.Fprintf(os.Stderr, "  %s\n", err)
			}
			fmt.Fprintf(os.Stderr, "\n提示:\n")
			fmt.Fprintf(os.Stderr, "  • 查看在线数据源:     binlogx parse --db-connection='user:pass@tcp(host:port)/'\n")
			fmt.Fprintf(os.Stderr, "  • 从指定文件开始:     binlogx parse --db-connection='...' --start-log-file=mysql-bin.000002\n")
			fmt.Fprintf(os.Stderr, "  • 从指定位置开始:     binlogx parse --db-connection='...' --start-log-file=mysql-bin.000001 --start-log-pos=4\n")
			fmt.Fprintf(os.Stderr, "  • 查看离线文件:       binlogx parse --source=/path/to/binlog\n\n")
			return fmt.Errorf("参数验证失败")
		}
		// ========== 参数验证结束 ==========

		// 创建数据源
		var ds source.DataSource
		var sourceType string
		if cfg.Source != "" {
			ds = source.NewFileSource(cfg.Source)
			sourceType = "file"
		} else {
			mysqlSource := source.NewMySQLSource(cfg.DBConnection)
			ds = mysqlSource
			sourceType = "mysql"

			// 处理断点续看逻辑（仅在线 MySQL 数据源支持）
			var startFile string
			var startPos uint32

			// 优先级1：命令行指定的位置（最高优先级）
			if startLogFile != "" && startLogPos > 0 {
				startFile = startLogFile
				startPos = startLogPos
				fmt.Printf("使用命令行指定的起始位置: %s:%d\n", startFile, startPos)
			} else {
				// 优先级2：检查断点文件
				source := cfg.Source
				if source == "" {
					source = cfg.DBConnection
				}
				checkpointMgr := checkpoint.NewManager(source, sourceType)
				savedPos, err := checkpointMgr.Load()
				if err == nil && savedPos != nil {
					// 有保存的断点，提示用户选择
					fmt.Printf("\n找到上次的断点位置:\n")
					fmt.Printf("  文件: %s\n", savedPos.File)
					fmt.Printf("  位置: %d\n", savedPos.Pos)
					fmt.Printf("  时间: %s\n", savedPos.Timestamp.Format("2006-01-02 15:04:05"))
					if savedPos.Database != "" || savedPos.Table != "" {
						fmt.Printf("  最后事件: %s.%s (%s)\n", savedPos.Database, savedPos.Table, savedPos.EventType)
					}
					fmt.Printf("\n是否从断点继续？(y/n，默认y): ")

					reader := bufio.NewReader(os.Stdin)
					input, _ := reader.ReadString('\n')
					input = strings.TrimSpace(strings.ToLower(input))

					if input == "" || input == "y" || input == "yes" {
						startFile = savedPos.File
						startPos = savedPos.Pos
						fmt.Printf("从断点继续: %s:%d\n\n", startFile, startPos)
					} else {
						fmt.Println("从头开始读取...")
						checkpointMgr.Clear() // 清除断点
					}
				}
			}

			// 设置起始位置
			if startFile != "" && startPos > 0 {
				mysqlSource.SetStartPosition(startFile, startPos)
			}
		}

		// 打开数据源
		if err := ds.Open(cmd.Context()); err != nil {
			return err
		}
		defer ds.Close()

		// 创建过滤器
		rf, err := filter.NewRouteFilter(cfg.SchemaTableRegex)
		if err != nil {
			return err
		}

		// 创建命令助手（包含列名缓存和映射功能）
		helper := NewCommandHelper(cfg.DBConnection)

		// 创建流式处理器 - 立即输出事件，不缓存
		eventChan := make(chan *models.Event, 100)
		parser := &streamParseHandler{
			sqlGenerator: util.NewSQLGenerator(config.GlobalMonitor),
			helper:       helper,
			eventChan:    eventChan,
		}

		// 创建处理器
		proc := processor.NewEventProcessor(ds, rf, cfg.Workers)
		proc.AddHandler(parser)

		// 在单独的 goroutine 中启动处理
		go func() {
			if err := proc.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Error starting processor: %v\n", err)
			}
			if err := proc.Wait(); err != nil {
				fmt.Fprintf(os.Stderr, "Error waiting for processor: %v\n", err)
			}
			close(eventChan)
		}()

		// 创建 checkpoint manager（如果适用）
		var checkpointMgr *checkpoint.Manager
		if sourceType == "mysql" {
			source := cfg.Source
			if source == "" {
				source = cfg.DBConnection
			}
			checkpointMgr = checkpoint.NewManager(source, sourceType)
		}

		// 在主线程中交互式显示事件
		displayEventsStreamingInteractive(eventChan, checkpointMgr)

		// 恢复日志输出
		log.SetOutput(originalLogWriter)

		return nil
	},
}

// streamParseHandler 流式处理器：立即发送事件，不缓存
type streamParseHandler struct {
	sqlGenerator *util.SQLGenerator
	helper       *CommandHelper
	mu           sync.Mutex
	eventChan    chan *models.Event
	count        int
}

func (ph *streamParseHandler) Handle(event *models.Event) error {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	// 重要：先映射列名，再生成 SQL
	// 这样 AfterValues 和 BeforeValues 中的 col_N 会被替换为实际列名
	ph.helper.MapColumnNames(event)

	// 生成 SQL（此时 AfterValues 已经有实际列名了）
	if event.Action != "QUERY" {
		switch event.Action {
		case "INSERT":
			event.SQL = ph.sqlGenerator.GenerateInsertSQL(event)
		case "UPDATE":
			event.SQL = ph.sqlGenerator.GenerateUpdateSQL(event)
		case "DELETE":
			event.SQL = ph.sqlGenerator.GenerateDeleteSQL(event)
		}
	}

	ph.count++

	// 立即发送到输出通道
	select {
	case ph.eventChan <- event:
	default:
		// 通道满了，阻塞发送
		ph.eventChan <- event
	}

	return nil
}

func (ph *streamParseHandler) Flush() error {
	return nil
}

// displayEventsStreamingInteractive 交互式显示流式事件，类似 more 命令的分页
func displayEventsStreamingInteractive(eventChan chan *models.Event, checkpointMgr *checkpoint.Manager) {
	// 将终端设为原始模式，以便实时读取键盘输入（不需要按Enter）
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		// 如果设置原始模式失败，回退到普通模式
		fmt.Fprintf(os.Stderr, "警告: 无法设置终端为原始模式，使用普通输入模式\n")
		displayEventsStreamingInteractiveNormal(eventChan, checkpointMgr)
		return
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	eventCount := 0
	var lastEvent *models.Event
	shouldExit := false

	// 获取终端尺寸
	width, height, _ := term.GetSize(int(os.Stdin.Fd()))
	_ = width // 暂时不用宽度
	if height <= 0 {
		height = 24 // 默认高度
	}
	_ = height // 暂时不用高度

	for event := range eventChan {
		if event == nil {
			continue
		}

		if shouldExit {
			break
		}

		eventCount++
		lastEvent = event

		// 显示当前事件
		data, _ := json.MarshalIndent(event, "", "  ")

		// 在原始模式下，需要将 \n 转换为 \r\n
		eventStr := fmt.Sprintf("[Event %d]\r\n%s\r\n", eventCount, string(data))
		// 替换 JSON 中的 \n 为 \r\n
		eventStr = strings.ReplaceAll(eventStr, "\n", "\r\n")

		// 写入事件内容
		fmt.Fprint(os.Stdout, eventStr)
		os.Stdout.Sync()

		// 显示分页提示并等待按键
		if !showPaginationAndWait() {
			shouldExit = true
		}
	}

	// 清空最后的提示行
	fmt.Fprint(os.Stdout, "\r\n")
	os.Stdout.Sync()

	// 保存断点（如果有 checkpoint manager 且有事件被处理）
	if checkpointMgr != nil && lastEvent != nil {
		if err := checkpointMgr.Save(
			lastEvent.LogName,
			lastEvent.LogPos,
			lastEvent.EventType,
			lastEvent.Database,
			lastEvent.Table,
		); err != nil {
			fmt.Fprintf(os.Stderr, "警告: 保存断点失败: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "断点已保存: %s:%d\n", lastEvent.LogName, lastEvent.LogPos)
		}
	}

	fmt.Fprintf(os.Stderr, "总共显示事件数: %d\n", eventCount)
}

// showPaginationAndWait 显示分页提示并等待用户按键，返回 true 表示继续，false 表示退出
func showPaginationAndWait() bool {
	const promptMsg = "Press SPACE/Enter for next, 'q' to quit:"

	// 显示提示（不带换行，这样光标停留在行尾）
	fmt.Fprint(os.Stdout, promptMsg)
	os.Stdout.Sync()

	// 等待按键
	buf := make([]byte, 1)
	for {
		n, _ := os.Stdin.Read(buf)
		if n == 0 {
			continue
		}

		char := buf[0]
		switch char {
		case ' ', '\n', '\r': // 空格或Enter
			// 按键后用 \r\n 打印换行
			fmt.Fprint(os.Stdout, "\r\n")
			os.Stdout.Sync()
			return true
		case 'q', 'Q': // q 或 Q
			fmt.Fprint(os.Stdout, "\r\n")
			os.Stdout.Sync()
			return false
		case 3: // Ctrl+C
			fmt.Fprint(os.Stdout, "\r\n")
			os.Stdout.Sync()
			return false
		default:
			// 其他按键，继续等待（不做任何响应）
			continue
		}
	}
}

// displayEventsStreamingInteractiveNormal 回退模式：使用普通的行输入方式
func displayEventsStreamingInteractiveNormal(eventChan chan *models.Event, checkpointMgr *checkpoint.Manager) {
	reader := bufio.NewReader(os.Stdin)
	eventCount := 0
	var lastEvent *models.Event

	for event := range eventChan {
		if event == nil {
			continue
		}

		eventCount++
		lastEvent = event

		// 显示当前事件
		data, _ := json.MarshalIndent(event, "", "  ")
		fmt.Printf("\n[Event %d]\n%s\n", eventCount, string(data))

		// 提示用户，等待输入
		fmt.Print("Press SPACE/Enter for next, 'q' to quit: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "q" || input == "Q" {
			fmt.Println("正在退出...")
			break
		}
	}

	// 保存断点（如果有 checkpoint manager 且有事件被处理）
	if checkpointMgr != nil && lastEvent != nil {
		if err := checkpointMgr.Save(
			lastEvent.LogName,
			lastEvent.LogPos,
			lastEvent.EventType,
			lastEvent.Database,
			lastEvent.Table,
		); err != nil {
			fmt.Fprintf(os.Stderr, "\n警告: 保存断点失败: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "\n断点已保存: %s:%d\n", lastEvent.LogName, lastEvent.LogPos)
		}
	}

	fmt.Fprintf(os.Stderr, "总共显示事件数: %d\n", eventCount)
}

func init() {
	// 添加 parse 命令的局部参数（仅适用于在线 MySQL 数据源）
	parseCmd.Flags().String("start-log-file", "", "起始 binlog 文件名（例如 mysql-bin.000001，仅用于 --db-connection）")
	parseCmd.Flags().Uint32("start-log-pos", 0, "起始 binlog 位置（仅用于 --db-connection）")
}
