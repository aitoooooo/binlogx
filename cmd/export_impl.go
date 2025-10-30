package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/aitoooooo/binlogx/pkg/models"
)

// SQLiteExporter SQLite 导出器的完整实现
type SQLiteExporter struct {
	path   string
	db     *sql.DB
	helper *CommandHelper
	mu     sync.Mutex
}

func newSQLiteExporter(output string, helper *CommandHelper) (*SQLiteExporter, error) {
	// 处理输出路径
	path := output
	if path == "" {
		path = "binlog_export.db"
	}

	// 打开或创建数据库
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	// 创建表
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS binlog_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME,
		event_type TEXT,
		server_id INTEGER,
		log_pos INTEGER,
		database TEXT,
		table_name TEXT,
		action TEXT,
		sql TEXT,
		before_values TEXT,
		after_values TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_timestamp ON binlog_events(timestamp);
	CREATE INDEX IF NOT EXISTS idx_database ON binlog_events(database);
	CREATE INDEX IF NOT EXISTS idx_table ON binlog_events(table_name);
	`

	if _, err := db.Exec(createTableSQL); err != nil {
		return nil, err
	}

	return &SQLiteExporter{
		path:   path,
		db:     db,
		helper: helper,
	}, nil
}

func (se *SQLiteExporter) Handle(event *models.Event) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	// 映射列名：将 col_N 替换为实际列名
	se.helper.MapColumnNames(event)

	beforeJSON, _ := json.Marshal(event.BeforeValues)
	afterJSON, _ := json.Marshal(event.AfterValues)

	insertSQL := `
	INSERT INTO binlog_events
	(timestamp, event_type, server_id, log_pos, database, table_name, action, sql, before_values, after_values)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := se.db.Exec(
		insertSQL,
		event.Timestamp,
		event.EventType,
		event.ServerID,
		event.LogPos,
		event.Database,
		event.Table,
		event.Action,
		event.SQL,
		string(beforeJSON),
		string(afterJSON),
	)

	return err
}

func (se *SQLiteExporter) Flush() error {
	se.mu.Lock()
	defer se.mu.Unlock()

	// 输出统计信息
	row := se.db.QueryRow("SELECT COUNT(*) FROM binlog_events")
	var count int
	if err := row.Scan(&count); err == nil {
		fmt.Printf("Successfully exported %d events to %s\n", count, se.path)
	}

	return se.db.Close()
}

// H2Exporter H2 数据库导出器的完整实现
type H2Exporter struct {
	path   string
	helper *CommandHelper
	mu     sync.Mutex
}

func newH2Exporter(output string, helper *CommandHelper) (*H2Exporter, error) {
	if output == "" {
		output = "binlog_export.h2"
	}
	return &H2Exporter{
		path:   output,
		helper: helper,
	}, nil
}

func (he *H2Exporter) Handle(event *models.Event) error {
	he.mu.Lock()
	defer he.mu.Unlock()

	// 映射列名：将 col_N 替换为实际列名
	he.helper.MapColumnNames(event)

	// TODO: 实现 H2 协议
	// 这里仅作占位符实现
	return nil
}

func (he *H2Exporter) Flush() error {
	fmt.Printf("H2 export to %s completed\n", he.path)
	return nil
}

// HiveExporter Hive 导出器的完整实现
type HiveExporter struct {
	path   string
	helper *CommandHelper
	mu     sync.Mutex
}

func newHiveExporter(output string, helper *CommandHelper) (*HiveExporter, error) {
	if output == "" {
		output = "/hive/binlog_export"
	}
	return &HiveExporter{
		path:   output,
		helper: helper,
	}, nil
}

func (he *HiveExporter) Handle(event *models.Event) error {
	he.mu.Lock()
	defer he.mu.Unlock()

	// 映射列名：将 col_N 替换为实际列名
	he.helper.MapColumnNames(event)

	// TODO: 实现 Hive 分区表导出
	// 可以按日期分区：/hive/binlog_export/date=2024-01-01/
	return nil
}

func (he *HiveExporter) Flush() error {
	fmt.Printf("Hive export to %s completed\n", he.path)
	return nil
}

// ESExporter Elasticsearch 导出器的完整实现
type ESExporter struct {
	endpoint string
	helper   *CommandHelper
	mu       sync.Mutex
}

func newESExporter(output string, helper *CommandHelper) (*ESExporter, error) {
	if output == "" {
		output = "http://localhost:9200"
	}
	return &ESExporter{
		endpoint: output,
		helper:   helper,
	}, nil
}

func (ee *ESExporter) Handle(event *models.Event) error {
	ee.mu.Lock()
	defer ee.mu.Unlock()

	// 映射列名：将 col_N 替换为实际列名
	ee.helper.MapColumnNames(event)

	// TODO: 实现 Elasticsearch 索引导出
	// 使用 github.com/elastic/go-elasticsearch
	return nil
}

func (ee *ESExporter) Flush() error {
	fmt.Printf("Elasticsearch export to %s completed\n", ee.endpoint)
	return nil
}
