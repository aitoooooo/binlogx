package config

import (
	"fmt"
	"runtime"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/spf13/cobra"
)

var GlobalConfig *models.GlobalConfig

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
	cfg.DBRegex, _ = cmd.Flags().GetString("db-regex")
	cfg.TableRegex, _ = cmd.Flags().GetString("table-regex")
	cfg.IncludeDB, _ = cmd.Flags().GetStringSlice("include-db")
	cfg.IncludeTable, _ = cmd.Flags().GetStringSlice("include-table")

	// Worker 数量
	workers, _ := cmd.Flags().GetInt("workers")
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	cfg.Workers = workers

	GlobalConfig = cfg
	return cfg, nil
}

func AddGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("source", "", "离线 binlog 文件路径")
	cmd.PersistentFlags().String("db-connection", "", "在线 DSN user:pass@tcp(host:port)/dbname?charset=utf8mb4")
	cmd.PersistentFlags().String("start-time", "", "开始时间 YYYY-MM-DD HH:MM:SS")
	cmd.PersistentFlags().String("end-time", "", "结束时间 YYYY-MM-DD HH:MM:SS")
	cmd.PersistentFlags().StringSlice("action", []string{}, "操作类型过滤 (INSERT,UPDATE,DELETE)")
	cmd.PersistentFlags().String("slow-threshold", "1s", "慢方法阈值")
	cmd.PersistentFlags().Int64("event-size-threshold", 0, "事件大小阈值（字节），默认 0（不检测）")
	cmd.PersistentFlags().String("db-regex", "", "分库正则 例 db_[0-3]")
	cmd.PersistentFlags().String("table-regex", "", "分表正则 例 table_[0-15]")
	cmd.PersistentFlags().StringSlice("include-db", []string{}, "精确库列表")
	cmd.PersistentFlags().StringSlice("include-table", []string{}, "精确表列表")
	cmd.PersistentFlags().Int("workers", 0, "worker 数量，默认 0=CPU 数")
}
