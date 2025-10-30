package cmd

import (
	"fmt"
	"sort"
	"sync"

	"github.com/aitoooooo/binlogx/pkg/config"
	"github.com/aitoooooo/binlogx/pkg/filter"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/processor"
	"github.com/aitoooooo/binlogx/pkg/source"
	"github.com/spf13/cobra"
)

var statCmd = &cobra.Command{
	Use:   "stat",
	Short: "Show binlog statistics",
	Long:  "Show total events, database/table/action distribution, and large event analysis",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 初始化配置
		cfg, err := config.InitConfig(cmd)
		if err != nil {
			return err
		}

		top, _ := cmd.Flags().GetInt("top")

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

		// 统计
		stat := &statHandler{
			result: &models.StatResult{
				DatabaseDist:   make(map[string]int64),
				TableDist:      make(map[string]int64),
				ActionDist:     make(map[string]int64),
				LargeEventDist: make(map[string]int64),
			},
			eventSizeThreshold: cfg.EventSizeThreshold,
		}

		// 创建处理器
		proc := processor.NewEventProcessor(ds, rf, cfg.Workers)
		proc.AddHandler(stat)

		// 启动处理
		if err := proc.Start(); err != nil {
			return err
		}

		// 等待完成
		if err := proc.Wait(); err != nil {
			return err
		}

		// 输出结果
		printStatResult(stat.result, top)
		return nil
	},
}

type statHandler struct {
	result                 *models.StatResult
	mu                     sync.Mutex
	eventSizeThreshold     int64
}

func (sh *statHandler) Handle(event *models.Event) error {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	sh.result.TotalEvents++
	sh.result.DatabaseDist[event.Database]++
	tableKey := event.Database + "." + event.Table
	sh.result.TableDist[tableKey]++
	sh.result.ActionDist[event.Action]++

	// 检查是否是大事件
	eventSize := int64(len(event.RawData))
	if eventSize > sh.eventSizeThreshold {
		sh.result.LargeEvents++
		sh.result.LargeEventDist[tableKey]++
	}

	// 跟踪最大事件
	if eventSize > sh.result.MaxEventSize {
		sh.result.MaxEventSize = eventSize
		sh.result.MaxEventTable = tableKey
	}

	return nil
}

func (sh *statHandler) Flush() error {
	return nil
}

func printStatResult(result *models.StatResult, top int) {
	fmt.Printf("Total Events: %d\n\n", result.TotalEvents)

	fmt.Println("=== Database Distribution ===")
	printDist(result.DatabaseDist, top)

	fmt.Println("\n=== Table Distribution ===")
	printDist(result.TableDist, top)

	fmt.Println("\n=== Action Distribution ===")
	printDist(result.ActionDist, top)

	// 大事件统计
	fmt.Printf("\n=== Large Event Analysis ===\n")
	fmt.Printf("Large Events (> %d bytes): %d\n", 1024, result.LargeEvents)
	fmt.Printf("Max Event Size: %d bytes\n", result.MaxEventSize)
	if result.MaxEventTable != "" {
		fmt.Printf("Max Event Table: %s\n", result.MaxEventTable)
	}

	if result.LargeEvents > 0 {
		fmt.Println("\nLarge Event Distribution:")
		printDist(result.LargeEventDist, top)
	}
}

func printDist(dist map[string]int64, top int) {
	type kv struct {
		Key   string
		Value int64
	}

	var items []kv
	for k, v := range dist {
		items = append(items, kv{k, v})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Value > items[j].Value
	})

	count := len(items)
	if top > 0 && top < count {
		items = items[:top]
	}

	for _, item := range items {
		fmt.Printf("  %s: %d\n", item.Key, item.Value)
	}
}

func init() {
	statCmd.Flags().IntP("top", "t", 0, "只展示前 N 条统计结果，默认 0（全部）")
}
