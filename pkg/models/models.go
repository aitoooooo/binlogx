package models

import (
	"time"
)

// Event 代表一个 binlog 事件
type Event struct {
	Timestamp      time.Time              `json:"timestamp"`
	EventType      string                 `json:"event_type"`
	ServerID       uint32                 `json:"server_id"`
	LogPos         uint32                 `json:"log_pos"`
	Database       string                 `json:"database"`
	Table          string                 `json:"table"`
	Action         string                 `json:"action"` // INSERT, UPDATE, DELETE
	SQL            string                 `json:"sql"`
	BeforeValues   map[string]interface{} `json:"before_values"`
	AfterValues    map[string]interface{} `json:"after_values"`
	RawData        []byte                 `json:"-"`
}

// GlobalConfig 全局配置
type GlobalConfig struct {
	// 数据源（二选一）
	Source        string        // 离线文件路径
	DBConnection  string        // 在线 DSN
	StartTime     time.Time     // 开始时间
	EndTime       time.Time     // 结束时间
	Action        []string      // 操作类型过滤
	SlowThreshold time.Duration // 慢方法阈值，默认 1s
	EventSizeThreshold int64     // 事件大小阈值（字节），默认 0（不检测）

	// 分库表正则路由
	DBRegex        string   // 分库正则
	TableRegex     string   // 分表正则
	IncludeDB      []string // 精确库列表
	IncludeTable   []string // 精确表列表
	Workers        int      // worker 数量，默认 0=CPU 数

	// 命令专属参数
	// parse
	PageSize int

	// export
	ExportType string
	Output     string

	// rollback-sql
	Bulk bool

	// stat
	Top int
}

// StatResult 统计结果
type StatResult struct {
	TotalEvents   int64
	DatabaseDist  map[string]int64
	TableDist     map[string]int64
	ActionDist    map[string]int64
}

// ColumnMeta 列元数据
type ColumnMeta struct {
	Name     string
	Type     string      // INT, VARCHAR, DECIMAL, JSON, etc.
	Unsigned bool
	Nullable bool
	Default  interface{}
}

// TableMeta 表元数据
type TableMeta struct {
	Columns    []ColumnMeta
	PrimaryKey []string // 主键列名
}
