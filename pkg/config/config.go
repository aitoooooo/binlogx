package config

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/monitor"
	"github.com/spf13/cobra"
)

var GlobalConfig *models.GlobalConfig
var GlobalMonitor *monitor.Monitor

func InitConfig(cmd *cobra.Command) (*models.GlobalConfig, error) {
	cfg := &models.GlobalConfig{}

	// 全局参数
	source, _ := cmd.Flags().GetString("source")
	dbConnection, _ := cmd.Flags().GetString("db-connection")

	if source == "" && dbConnection == "" {
		return nil, fmt.Errorf("must specify either --source or --db-connection")
	}

	cfg.Source = source
	cfg.DBConnection = dbConnection

	// 时间范围
	startTimeStr, _ := cmd.Flags().GetString("start-time")
	endTimeStr, _ := cmd.Flags().GetString("end-time")

	if startTimeStr != "" {
		t, err := time.Parse("2006-01-02 15:04:05", startTimeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid start-time format: %w", err)
		}
		cfg.StartTime = t
	}

	if endTimeStr != "" {
		t, err := time.Parse("2006-01-02 15:04:05", endTimeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid end-time format: %w", err)
		}
		cfg.EndTime = t
	}

	// Action 过滤
	actions, _ := cmd.Flags().GetStringSlice("action")
	cfg.Action = actions

	// 监控阈值
	slowThresholdStr, _ := cmd.Flags().GetString("slow-threshold")
	slowThreshold, err := time.ParseDuration(slowThresholdStr)
	if err != nil {
		return nil, fmt.Errorf("invalid slow-threshold format: %w", err)
	}
	cfg.SlowThreshold = slowThreshold

	eventSizeThreshold, _ := cmd.Flags().GetInt64("event-size-threshold")
	cfg.EventSizeThreshold = eventSizeThreshold

	// 分库表正则
	cfg.SchemaTableRegex, _ = cmd.Flags().GetStringSlice("schema-table-regex")

	// Worker 数量
	workers, _ := cmd.Flags().GetInt("workers")
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	cfg.Workers = workers

	GlobalConfig = cfg

	// 创建全局 Monitor 对象
	GlobalMonitor = monitor.NewMonitor(cfg.SlowThreshold, cfg.EventSizeThreshold)

	// 打印配置信息
	printConfig(cfg)
	return cfg, nil
}

// printConfig 以美化格式打印配置信息
func printConfig(cfg *models.GlobalConfig) {
	log.Println("========================================")
	log.Println("             配置信息")
	log.Println("========================================")

	// 数据源
	log.Println("【数据源】")
	if cfg.Source != "" {
		log.Printf("  离线文件: %s", cfg.Source)
	}
	if cfg.DBConnection != "" {
		// 隐藏密码
		dsn := cfg.DBConnection
		if idx := strings.Index(dsn, "@"); idx > 0 {
			if pidx := strings.Index(dsn[:idx], ":"); pidx > 0 {
				dsn = dsn[:pidx+1] + "****" + dsn[idx:]
			}
		}
		log.Printf("  在线连接: %s", dsn)
	}

	// 断点续看
	if cfg.StartLogFile != "" || cfg.StartLogPos > 0 {
		log.Println("【断点续看】")
		if cfg.StartLogFile != "" {
			log.Printf("  起始文件: %s", cfg.StartLogFile)
		}
		if cfg.StartLogPos > 0 {
			log.Printf("  起始位置: %d", cfg.StartLogPos)
		}
	}

	// 时间范围
	if !cfg.StartTime.IsZero() || !cfg.EndTime.IsZero() {
		log.Println("【时间范围】")
		if !cfg.StartTime.IsZero() {
			log.Printf("  开始时间: %s", cfg.StartTime.Format("2006-01-02 15:04:05"))
		}
		if !cfg.EndTime.IsZero() {
			log.Printf("  结束时间: %s", cfg.EndTime.Format("2006-01-02 15:04:05"))
		}
	}

	// 过滤条件
	hasFilters := len(cfg.Action) > 0 || len(cfg.SchemaTableRegex) > 0
	if hasFilters {
		log.Println("【过滤条件】")
		if len(cfg.Action) > 0 {
			log.Printf("  操作类型: %v", cfg.Action)
		}
		if len(cfg.SchemaTableRegex) > 0 {
			log.Printf("  表匹配:   %v", cfg.SchemaTableRegex)
		}
	}

	// 性能配置
	log.Println("【性能配置】")
	log.Printf("  Worker数量:    %d", cfg.Workers)
	log.Printf("  慢事件阈值:    %s", cfg.SlowThreshold)
	log.Printf("  大事件阈值:    %d 字节", cfg.EventSizeThreshold)

	// 命令专属参数
	if cfg.ExportType != "" || cfg.Output != "" || cfg.Bulk || cfg.Top > 0 {
		log.Println("【命令参数】")
		if cfg.ExportType != "" {
			log.Printf("  导出类型: %s", cfg.ExportType)
		}
		if cfg.Output != "" {
			log.Printf("  输出文件: %s", cfg.Output)
		}
		if cfg.Bulk {
			log.Printf("  批量模式: 是")
		}
		if cfg.Top > 0 {
			log.Printf("  Top数量:  %d", cfg.Top)
		}
	}

	log.Println("========================================")
}

func AddGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("source", "", "离线 binlog 文件路径")
	cmd.PersistentFlags().String("db-connection", "", "在线 DSN user:pass@tcp(host:port)/dbname?charset=utf8mb4")
	cmd.PersistentFlags().String("start-time", "", "开始时间 YYYY-MM-DD HH:MM:SS")
	cmd.PersistentFlags().String("end-time", "", "结束时间 YYYY-MM-DD HH:MM:SS")
	cmd.PersistentFlags().StringSlice("action", []string{}, "操作类型过滤 (INSERT,UPDATE,DELETE)")
	cmd.PersistentFlags().String("slow-threshold", "50ms", "慢事件处理阈值，超过此时间则标记为慢事件（默认 50ms）")
	cmd.PersistentFlags().Int64("event-size-threshold", 1024, "大事件大小阈值（字节），超过此大小则标记为大事件（默认 1KiB=1024字节）")
	cmd.PersistentFlags().StringSlice("schema-table-regex", []string{}, "schema.table 的范围匹配表达式(不是正则表达式)。示例 *.my_table 或者 db_[0-3].my_table_[0-9]")
	cmd.PersistentFlags().Int("workers", 0, "worker 数量，默认 0=CPU 数")
}
