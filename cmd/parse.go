package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/aitoooooo/binlogx/pkg/config"
	"github.com/aitoooooo/binlogx/pkg/filter"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/processor"
	"github.com/aitoooooo/binlogx/pkg/source"
	"github.com/spf13/cobra"
)

var parseCmd = &cobra.Command{
	Use:   "parse",
	Short: "Interactively view binlog events",
	Long:  "Parse and display binlog events in a paginated format",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 初始化配置
		cfg, err := config.InitConfig(cmd)
		if err != nil {
			return err
		}

		pageSize, _ := cmd.Flags().GetInt("page-size")
		if pageSize <= 0 {
			pageSize = 20
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
		rf, err := filter.NewRouteFilter(cfg.IncludeDB, cfg.IncludeTable, cfg.DBRegex, cfg.TableRegex)
		if err != nil {
			return err
		}

		// 收集事件
		parser := &parseHandler{
			pageSize: pageSize,
			events:   make([]*models.Event, 0),
		}

		// 创建处理器
		proc := processor.NewEventProcessor(ds, rf, cfg.Workers)
		proc.AddHandler(parser)

		// 启动处理
		if err := proc.Start(); err != nil {
			return err
		}

		// 等待完成
		if err := proc.Wait(); err != nil {
			return err
		}

		// 交互式浏览
		displayPaginatedEvents(parser.events, pageSize)
		return nil
	},
}

type parseHandler struct {
	pageSize int
	events   []*models.Event
	mu       sync.Mutex
}

func (ph *parseHandler) Handle(event *models.Event) error {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	ph.events = append(ph.events, event)
	return nil
}

func (ph *parseHandler) Flush() error {
	return nil
}

func displayPaginatedEvents(events []*models.Event, pageSize int) {
	if len(events) == 0 {
		fmt.Println("No events found")
		return
	}

	totalPages := (len(events) + pageSize - 1) / pageSize
	currentPage := 0
	scanner := bufio.NewScanner(os.Stdin)

	for {
		// 显示当前页
		start := currentPage * pageSize
		end := start + pageSize
		if end > len(events) {
			end = len(events)
		}

		fmt.Printf("\n=== Page %d/%d ===\n", currentPage+1, totalPages)
		for i, event := range events[start:end] {
			data, _ := json.MarshalIndent(event, "", "  ")
			fmt.Printf("[%d] %s\n", start+i+1, string(data))
		}

		if currentPage == totalPages-1 {
			fmt.Println("(End of results)")
			break
		}

		fmt.Print("Press 'n' for next page, 'q' to quit: ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		if input == "q" {
			break
		} else if input == "n" {
			currentPage++
			if currentPage >= totalPages {
				currentPage = totalPages - 1
			}
		}
	}
}

func init() {
	parseCmd.Flags().IntP("page-size", "p", 20, "每页事件数，默认 20")
}
