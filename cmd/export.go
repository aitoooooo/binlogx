package cmd

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/aitoooooo/binlogx/pkg/config"
	"github.com/aitoooooo/binlogx/pkg/filter"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/processor"
	"github.com/aitoooooo/binlogx/pkg/source"
	"github.com/spf13/cobra"
)

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

		if exportType == "" || output == "" {
			return fmt.Errorf("--type and --output are required")
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

		// 创建导出处理器
		var exportHandler processor.EventHandler
		switch exportType {
		case "csv":
			handler, err := newCSVExporter(output, helper)
			if err != nil {
				return err
			}
			exportHandler = handler
		case "sqlite":
			handler, err := newSQLiteExporter(output, helper)
			if err != nil {
				return err
			}
			exportHandler = handler
		case "h2":
			handler, err := newH2Exporter(output, helper)
			if err != nil {
				return err
			}
			exportHandler = handler
		case "hive":
			handler, err := newHiveExporter(output, helper)
			if err != nil {
				return err
			}
			exportHandler = handler
		case "es":
			handler, err := newESExporter(output, helper)
			if err != nil {
				return err
			}
			exportHandler = handler
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
	file   *os.File
	writer *csv.Writer
	helper *CommandHelper
	mu     sync.Mutex
}

func newCSVExporter(output string, helper *CommandHelper) (*CSVExporter, error) {
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
		file:   file,
		writer: writer,
		helper: helper,
	}

	// 写入头
	headers := []string{"Timestamp", "EventType", "ServerID", "LogPos", "Database", "Table", "Action", "SQL"}
	writer.Write(headers)

	return exporter, nil
}

func (ce *CSVExporter) Handle(event *models.Event) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	// 映射列名：将 col_N 替换为实际列名
	ce.helper.MapColumnNames(event)

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
	exportCmd.Flags().StringP("type", "t", "", "导出介质：csv,sqlite,h2,hive,es (必填)")
	exportCmd.Flags().StringP("output", "o", "", "输出路径或连接串 (必填)")
}
