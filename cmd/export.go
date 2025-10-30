package cmd

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aitoooooo/binlogx/pkg/config"
	"github.com/aitoooooo/binlogx/pkg/filter"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/processor"
	"github.com/aitoooooo/binlogx/pkg/source"
	"github.com/aitoooooo/binlogx/pkg/util"
	"github.com/spf13/cobra"
)

// ProgressTracker 进度跟踪器
type ProgressTracker struct {
	processed   int64
	exported    int64
	filtered    int64
	startTime   time.Time
	lastTime    time.Time
	lastCount   int64
	stopChan    chan struct{}
	mu          sync.Mutex
}

func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		startTime: time.Now(),
		lastTime:  time.Now(),
		stopChan:  make(chan struct{}),
	}
}

func (pt *ProgressTracker) Start() {
	// 每分钟打印一次进度
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
				pt.PrintProgress()
			case <-pt.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}

func (pt *ProgressTracker) Stop() {
	close(pt.stopChan)
}

func (pt *ProgressTracker) AddProcessed(count int64) {
	atomic.AddInt64(&pt.processed, count)
}

func (pt *ProgressTracker) AddExported(count int64) {
	atomic.AddInt64(&pt.exported, count)
}

func (pt *ProgressTracker) AddFiltered(count int64) {
	atomic.AddInt64(&pt.filtered, count)
}

func (pt *ProgressTracker) PrintProgress() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(pt.startTime)
	currentProcessed := atomic.LoadInt64(&pt.processed)
	currentExported := atomic.LoadInt64(&pt.exported)
	currentFiltered := atomic.LoadInt64(&pt.filtered)

	// 计算速率 (events/sec)
	timeSinceLastCheck := now.Sub(pt.lastTime).Seconds()
	eventsSinceLastCheck := currentProcessed - pt.lastCount
	ratePerSec := float64(eventsSinceLastCheck) / timeSinceLastCheck

	pt.lastTime = now
	pt.lastCount = currentProcessed

	// 格式化输出
	fmt.Fprintf(os.Stderr, "\n[进度] 耗时: %s | 已处理: %d | 已导出: %d | 已过滤: %d | 速率: %.1f events/sec\n",
		pt.formatDuration(elapsed),
		currentProcessed,
		currentExported,
		currentFiltered,
		ratePerSec,
	)
}

func (pt *ProgressTracker) PrintSummary() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	elapsed := time.Now().Sub(pt.startTime)
	processed := atomic.LoadInt64(&pt.processed)
	exported := atomic.LoadInt64(&pt.exported)
	filtered := atomic.LoadInt64(&pt.filtered)

	avgRate := float64(processed) / elapsed.Seconds()

	fmt.Fprintf(os.Stderr, "\n[完成] 总耗时: %s\n", pt.formatDuration(elapsed))
	fmt.Fprintf(os.Stderr, "[统计] 已处理: %d | 已导出: %d | 已过滤: %d | 平均速率: %.1f events/sec\n",
		processed,
		exported,
		filtered,
		avgRate,
	)
}

func (pt *ProgressTracker) formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// ProgressWrappedHandler 包装的导出处理器，用于跟踪进度
type ProgressWrappedHandler struct {
	inner      processor.EventHandler
	tracker    *ProgressTracker
	actions    map[string]bool
}

func NewProgressWrappedHandler(inner processor.EventHandler, tracker *ProgressTracker, actions map[string]bool) *ProgressWrappedHandler {
	return &ProgressWrappedHandler{
		inner:   inner,
		tracker: tracker,
		actions: actions,
	}
}

func (pwh *ProgressWrappedHandler) Handle(event *models.Event) error {
	// 计数已处理的事件
	pwh.tracker.AddProcessed(1)

	// 检查是否被过滤
	if !pwh.actions[event.Action] {
		pwh.tracker.AddFiltered(1)
		return nil
	}

	// 交由实际处理器处理
	err := pwh.inner.Handle(event)
	if err == nil {
		pwh.tracker.AddExported(1)
	}
	return err
}

func (pwh *ProgressWrappedHandler) Flush() error {
	return pwh.inner.Flush()
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export binlog events to various formats",
	Long:  "Export binlog events to CSV, SQLite, H2, Hive, or Elasticsearch",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 初始化配置
		cfg, err := config.InitConfig(cmd)
		if err != nil {
			return err
		}

		exportType, _ := cmd.Flags().GetString("type")
		output, _ := cmd.Flags().GetString("output")
		actionStr, _ := cmd.Flags().GetString("action")

		if exportType == "" {
			return fmt.Errorf("--type is required")
		}
		if output == "" {
			return fmt.Errorf("--output is required")
		}

		// 解析 action 过滤器
		actions := make(map[string]bool)
		if actionStr != "" {
			for _, action := range strings.Split(actionStr, ",") {
				actions[strings.TrimSpace(action)] = true
			}
		} else {
			// 默认：INSERT, UPDATE, DELETE
			actions["INSERT"] = true
			actions["UPDATE"] = true
			actions["DELETE"] = true
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

		// 创建命令助手（包含列名缓存和映射功能）
		helper := NewCommandHelper(cfg.DBConnection)

		// 创建进度跟踪器
		tracker := NewProgressTracker()
		tracker.Start()
		defer tracker.Stop()
		defer tracker.PrintSummary()

		// 创建导出处理器
		var exportHandler processor.EventHandler
		switch exportType {
		case "csv":
			handler, err := newCSVExporter(output, helper, actions)
			if err != nil {
				return err
			}
			exportHandler = NewProgressWrappedHandler(handler, tracker, actions)
		case "sqlite":
			handler, err := newSQLiteExporter(output, helper, actions)
			if err != nil {
				return err
			}
			exportHandler = NewProgressWrappedHandler(handler, tracker, actions)
		case "h2":
			handler, err := newH2Exporter(output, helper, actions)
			if err != nil {
				return err
			}
			exportHandler = NewProgressWrappedHandler(handler, tracker, actions)
		case "hive":
			handler, err := newHiveExporter(output, helper, actions)
			if err != nil {
				return err
			}
			exportHandler = NewProgressWrappedHandler(handler, tracker, actions)
		case "es":
			handler, err := newESExporter(output, helper, actions)
			if err != nil {
				return err
			}
			exportHandler = NewProgressWrappedHandler(handler, tracker, actions)
		default:
			return fmt.Errorf("unsupported export type: %s", exportType)
		}

		// 创建处理器
		proc := processor.NewEventProcessor(ds, rf, cfg.Workers)
		proc.AddHandler(exportHandler)

		// 启动处理
		if err := proc.Start(); err != nil {
			return err
		}

		// 等待完成
		return proc.Wait()
	},
}

// CSVExporter CSV 导出器
type CSVExporter struct {
	file         *os.File
	writer       *csv.Writer
	helper       *CommandHelper
	sqlGenerator *util.SQLGenerator
	actions      map[string]bool
	mu           sync.Mutex
}

func newCSVExporter(output string, helper *CommandHelper, actions map[string]bool) (*CSVExporter, error) {
	// 处理输出路径
	path := output
	if stat, err := os.Stat(output); err == nil && stat.IsDir() {
		path = filepath.Join(output, "binlog_export.csv")
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	writer := csv.NewWriter(file)
	exporter := &CSVExporter{
		file:         file,
		writer:       writer,
		helper:       helper,
		sqlGenerator: util.NewSQLGenerator(),
		actions:      actions,
	}

	// 写入头
	headers := []string{"Timestamp", "EventType", "ServerID", "LogPos", "Database", "Table", "Action", "SQL"}
	writer.Write(headers)

	return exporter, nil
}

func (ce *CSVExporter) Handle(event *models.Event) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	// 过滤：只导出指定的 action
	if !ce.actions[event.Action] {
		return nil
	}

	// 映射列名：将 col_N 替换为实际列名
	ce.helper.MapColumnNames(event)

	// 生成 SQL（此时列名已经映射为实际列名）
	if event.Action != "QUERY" && event.Action != "" {
		switch event.Action {
		case "INSERT":
			event.SQL = ce.sqlGenerator.GenerateInsertSQL(event)
		case "UPDATE":
			event.SQL = ce.sqlGenerator.GenerateUpdateSQL(event)
		case "DELETE":
			event.SQL = ce.sqlGenerator.GenerateDeleteSQL(event)
		}
	}

	record := []string{
		event.Timestamp.String(),
		event.EventType,
		fmt.Sprintf("%d", event.ServerID),
		fmt.Sprintf("%d", event.LogPos),
		event.Database,
		event.Table,
		event.Action,
		event.SQL,
	}
	return ce.writer.Write(record)
}

func (ce *CSVExporter) Flush() error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	ce.writer.Flush()
	return ce.file.Close()
}

func init() {
	exportCmd.Flags().StringP("type", "t", "csv", "导出介质：csv,sqlite,h2,hive,es (默认: csv)")
	exportCmd.Flags().StringP("output", "o", "", "输出路径或连接串 (必填)")
	exportCmd.Flags().StringP("action", "a", "INSERT,UPDATE,DELETE", "要导出的事件类型，以逗号分隔（默认: INSERT,UPDATE,DELETE）")
}


