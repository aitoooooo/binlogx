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
	totalEvents int64 // 总事件数（用于显示进度百分比）
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

func (pt *ProgressTracker) SetTotalEvents(total int64) {
	atomic.StoreInt64(&pt.totalEvents, total)
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

func (pt *ProgressTracker) PrintProgress() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(pt.startTime)
	currentProcessed := atomic.LoadInt64(&pt.processed)
	currentExported := atomic.LoadInt64(&pt.exported)
	totalEvents := atomic.LoadInt64(&pt.totalEvents)

	// 计算速率 (events/sec)
	timeSinceLastCheck := now.Sub(pt.lastTime).Seconds()
	eventsSinceLastCheck := currentProcessed - pt.lastCount
	ratePerSec := float64(eventsSinceLastCheck) / timeSinceLastCheck

	pt.lastTime = now
	pt.lastCount = currentProcessed

	// 格式化输出 - 如果有总数，显示百分比
	if totalEvents > 0 {
		percentage := float64(currentProcessed) * 100.0 / float64(totalEvents)
		// 估计剩余时间
		remainingEvents := totalEvents - currentProcessed
		var etaStr string
		if ratePerSec > 0 {
			remainingSeconds := float64(remainingEvents) / ratePerSec
			etaStr = pt.formatDuration(time.Duration(remainingSeconds) * time.Second)
		} else {
			etaStr = "未知"
		}
		fmt.Fprintf(os.Stderr, "\n[进度] 耗时: %s | 进度: %.1f%% (%d/%d) | 已导出: %d | 速率: %.1f events/sec | ETA: %s\n",
			pt.formatDuration(elapsed),
			percentage,
			currentProcessed,
			totalEvents,
			currentExported,
			ratePerSec,
			etaStr,
		)
	} else {
		// 没有总数，只显示绝对值
		fmt.Fprintf(os.Stderr, "\n[进度] 耗时: %s | 已处理: %d | 已导出: %d | 速率: %.1f events/sec\n",
			pt.formatDuration(elapsed),
			currentProcessed,
			currentExported,
			ratePerSec,
		)
	}
}

func (pt *ProgressTracker) PrintSummary() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	elapsed := time.Now().Sub(pt.startTime)
	processed := atomic.LoadInt64(&pt.processed)
	exported := atomic.LoadInt64(&pt.exported)
	totalEvents := atomic.LoadInt64(&pt.totalEvents)

	avgRate := float64(processed) / elapsed.Seconds()

	fmt.Fprintf(os.Stderr, "\n[完成] 总耗时: %s\n", pt.formatDuration(elapsed))

	if totalEvents > 0 {
		percentage := float64(processed) * 100.0 / float64(totalEvents)
		fmt.Fprintf(os.Stderr, "[统计] 进度: %.1f%% (%d/%d) | 已导出: %d | 平均速率: %.1f events/sec\n",
			percentage,
			processed,
			totalEvents,
			exported,
			avgRate,
		)
	} else {
		fmt.Fprintf(os.Stderr, "[统计] 已处理: %d | 已导出: %d | 平均速率: %.1f events/sec\n",
			processed,
			exported,
			avgRate,
		)
	}
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
	inner   processor.EventHandler
	tracker *ProgressTracker
	actions map[string]bool
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
		rf, err := filter.NewRouteFilter(cfg.SchemaTableRegex)
		if err != nil {
			return err
		}

		// 创建命令助手（包含列名缓存和映射功能）
		helper := NewCommandHelper(cfg.DBConnection)

		// 创建进度跟踪器
		tracker := NewProgressTracker()

		// 如果启用了 --estimate-total，先扫描一遍计算总事件数
		estimateTotal, _ := cmd.Flags().GetBool("estimate-total")
		if estimateTotal {
			fmt.Fprintf(os.Stderr, "[预扫描] 正在统计总事件数，请稍候...\n")

			// 创建一个简单的空处理器，只用来计数
			nullHandler := &nullEventHandler{}
			countHandlerImpl := &ProgressWrappedHandler{
				inner:   nullHandler,
				tracker: &ProgressTracker{}, // 临时的，不用显示进度
				actions: actions,
			}

			// 创建临时处理器来计数
			proc := processor.NewEventProcessor(ds, rf, cfg.Workers)
			proc.AddHandler(countHandlerImpl)

			// 启动扫描
			if err := proc.Start(); err != nil {
				return err
			}

			// 等待完成
			if err := proc.Wait(); err != nil {
				return err
			}

			// 获取处理的事件数
			totalCount := atomic.LoadInt64(&countHandlerImpl.tracker.processed)
			fmt.Fprintf(os.Stderr, "[预扫描] 总事件数: %d\n", totalCount)

			// 重新打开数据源用于实际导出
			ds.Close()
			if cfg.Source != "" {
				ds = source.NewFileSource(cfg.Source)
			} else {
				ds = source.NewMySQLSource(cfg.DBConnection)
			}
			if err := ds.Open(cmd.Context()); err != nil {
				return err
			}

			// 设置总数
			tracker.SetTotalEvents(totalCount)
		}

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
		sqlGenerator: util.NewSQLGenerator(config.GlobalMonitor),
		actions:      actions,
	}

	// 写入头
	headers := []string{"Timestamp", "EventType", "ServerID", "LogPos", "Database", "Table", "Action", "SQL"}
	writer.Write(headers)

	return exporter, nil
}

func (ce *CSVExporter) Handle(event *models.Event) error {
	// 映射列名和生成 SQL 在锁外进行（这些是 CPU 密集操作）
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

	// 过滤：只导出指定的 action（在锁外判断）
	if !ce.actions[event.Action] {
		return nil
	}

	// 只在写入文件时使用锁（最小化临界区）
	ce.mu.Lock()
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
	err := ce.writer.Write(record)
	ce.mu.Unlock()

	return err
}

func (ce *CSVExporter) Flush() error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	ce.writer.Flush()
	return ce.file.Close()
}

// nullEventHandler 空事件处理器，用于预扫描计数
type nullEventHandler struct{}

func (neh *nullEventHandler) Handle(event *models.Event) error {
	return nil
}

func (neh *nullEventHandler) Flush() error {
	return nil
}

func init() {
	exportCmd.Flags().StringP("type", "t", "csv", "导出介质：csv,sqlite,h2,hive,es (默认: csv)")
	exportCmd.Flags().StringP("output", "o", "", "输出路径或连接串 (必填)")
	exportCmd.Flags().StringP("action", "a", "INSERT,UPDATE,DELETE", "要导出的事件类型，以逗号分隔（默认: INSERT,UPDATE,DELETE）")
	exportCmd.Flags().BoolP("estimate-total", "e", false, "在导出前快速扫描统计总事件数，以便显示更准确的进度百分比 (默认: false)")
}
