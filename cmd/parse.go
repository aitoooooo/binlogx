package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/aitoooooo/binlogx/pkg/config"
	"github.com/aitoooooo/binlogx/pkg/filter"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/processor"
	"github.com/aitoooooo/binlogx/pkg/source"
	"github.com/aitoooooo/binlogx/pkg/util"
	"github.com/spf13/cobra"
)

var parseCmd = &cobra.Command{
	Use:   "parse",
	Short: "Parse and display binlog events",
	Long:  "Parse binlog events with streaming output and interactive paging",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 初始化配置
		cfg, err := config.InitConfig(cmd)
		if err != nil {
			return err
		}

		// 创建数据源
		var ds source.DataSource
		if cfg.Source != "" {
			ds = source.NewFileSource(cfg.Source)
		} else {
			ds = source.NewMySQLSource(cfg.DBConnection)
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

		// 在主线程中交互式显示事件
		displayEventsStreamingInteractive(eventChan)

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

// displayEventsStreamingInteractive 交互式显示流式事件，逐个输出，按空格继续
func displayEventsStreamingInteractive(eventChan chan *models.Event) {
	reader := bufio.NewReader(os.Stdin)
	eventCount := 0

	for event := range eventChan {
		if event == nil {
			continue
		}

		eventCount++

		// 显示当前事件
		data, _ := json.MarshalIndent(event, "", "  ")
		fmt.Printf("\n[Event %d]\n%s\n", eventCount, string(data))

		// 提示用户，等待空格继续
		fmt.Print("Press SPACE/Enter for next, 'q' to quit: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "q" || input == "Q" {
			fmt.Println("Exiting...")
			break
		}
	}

	fmt.Fprintf(os.Stderr, "\n[DEBUG] Total events displayed: %d\n", eventCount)
}
