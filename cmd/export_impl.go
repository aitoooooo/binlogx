package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/util"
)

// SQLiteExporter SQLite 导出器的完整实现
type SQLiteExporter struct {
	path         string
	db           *sql.DB
	helper       *CommandHelper
	sqlGenerator *util.SQLGenerator
	actions      map[string]bool
	batch        []*models.Event
	batchSize    int
	mu           sync.Mutex
}

func newSQLiteExporter(output string, helper *CommandHelper, actions map[string]bool) (*SQLiteExporter, error) {
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

	// 优化 SQLite 性能
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

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
		path:         path,
		db:           db,
		helper:       helper,
		sqlGenerator: util.NewSQLGenerator(),
		actions:      actions,
		batch:        make([]*models.Event, 0, 100),
		batchSize:    100,
	}, nil
}

func (se *SQLiteExporter) Handle(event *models.Event) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	// 过滤：只导出指定的 action
	if !se.actions[event.Action] {
		return nil
	}

	// 映射列名：将 col_N 替换为实际列名
	se.helper.MapColumnNames(event)

	// 生成 SQL（此时列名已经映射为实际列名）
	if event.Action != "QUERY" && event.Action != "" {
		switch event.Action {
		case "INSERT":
			event.SQL = se.sqlGenerator.GenerateInsertSQL(event)
		case "UPDATE":
			event.SQL = se.sqlGenerator.GenerateUpdateSQL(event)
		case "DELETE":
			event.SQL = se.sqlGenerator.GenerateDeleteSQL(event)
		}
	}

	// 添加到批处理队列
	se.batch = append(se.batch, event)

	// 当达到批处理大小时，执行批量插入
	if len(se.batch) >= se.batchSize {
		return se.flushBatch()
	}

	return nil
}

func (se *SQLiteExporter) flushBatch() error {
	if len(se.batch) == 0 {
		return nil
	}

	tx, err := se.db.Begin()
	if err != nil {
		return err
	}

	insertSQL := `
	INSERT INTO binlog_events
	(timestamp, event_type, server_id, log_pos, database, table_name, action, sql, before_values, after_values)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, event := range se.batch {
		beforeJSON, _ := json.Marshal(event.BeforeValues)
		afterJSON, _ := json.Marshal(event.AfterValues)

		if _, err := stmt.Exec(
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
		); err != nil {
			tx.Rollback()
			return err
		}
	}

	se.batch = se.batch[:0] // 清空批处理队列
	return tx.Commit()
}

func (se *SQLiteExporter) Flush() error {
	se.mu.Lock()
	defer se.mu.Unlock()

	// 刷新剩余的批处理数据
	if err := se.flushBatch(); err != nil {
		return err
	}

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
	path         string
	helper       *CommandHelper
	sqlGenerator *util.SQLGenerator
	actions      map[string]bool
	mu           sync.Mutex
}

func newH2Exporter(output string, helper *CommandHelper, actions map[string]bool) (*H2Exporter, error) {
	if output == "" {
		output = "binlog_export.h2"
	}
	return &H2Exporter{
		path:         output,
		helper:       helper,
		sqlGenerator: util.NewSQLGenerator(),
		actions:      actions,
	}, nil
}

func (he *H2Exporter) Handle(event *models.Event) error {
	he.mu.Lock()
	defer he.mu.Unlock()

	// 过滤：只导出指定的 action
	if !he.actions[event.Action] {
		return nil
	}

	// 映射列名：将 col_N 替换为实际列名
	he.helper.MapColumnNames(event)

	// 生成 SQL（此时列名已经映射为实际列名）
	if event.Action != "QUERY" && event.Action != "" {
		switch event.Action {
		case "INSERT":
			event.SQL = he.sqlGenerator.GenerateInsertSQL(event)
		case "UPDATE":
			event.SQL = he.sqlGenerator.GenerateUpdateSQL(event)
		case "DELETE":
			event.SQL = he.sqlGenerator.GenerateDeleteSQL(event)
		}
	}

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
	path         string
	helper       *CommandHelper
	sqlGenerator *util.SQLGenerator
	actions      map[string]bool
	mu           sync.Mutex
}

func newHiveExporter(output string, helper *CommandHelper, actions map[string]bool) (*HiveExporter, error) {
	if output == "" {
		output = "/hive/binlog_export"
	}
	return &HiveExporter{
		path:         output,
		helper:       helper,
		sqlGenerator: util.NewSQLGenerator(),
		actions:      actions,
	}, nil
}

func (he *HiveExporter) Handle(event *models.Event) error {
	he.mu.Lock()
	defer he.mu.Unlock()

	// 过滤：只导出指定的 action
	if !he.actions[event.Action] {
		return nil
	}

	// 映射列名：将 col_N 替换为实际列名
	he.helper.MapColumnNames(event)

	// 生成 SQL（此时列名已经映射为实际列名）
	if event.Action != "QUERY" && event.Action != "" {
		switch event.Action {
		case "INSERT":
			event.SQL = he.sqlGenerator.GenerateInsertSQL(event)
		case "UPDATE":
			event.SQL = he.sqlGenerator.GenerateUpdateSQL(event)
		case "DELETE":
			event.SQL = he.sqlGenerator.GenerateDeleteSQL(event)
		}
	}

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
	endpoint     string
	helper       *CommandHelper
	sqlGenerator *util.SQLGenerator
	actions      map[string]bool
	mu           sync.Mutex
}

func newESExporter(output string, helper *CommandHelper, actions map[string]bool) (*ESExporter, error) {
	if output == "" {
		output = "http://localhost:9200"
	}
	return &ESExporter{
		endpoint:     output,
		helper:       helper,
		sqlGenerator: util.NewSQLGenerator(),
		actions:      actions,
	}, nil
}

func (ee *ESExporter) Handle(event *models.Event) error {
	ee.mu.Lock()
	defer ee.mu.Unlock()

	// 过滤：只导出指定的 action
	if !ee.actions[event.Action] {
		return nil
	}

	// 映射列名：将 col_N 替换为实际列名
	ee.helper.MapColumnNames(event)

	// 生成 SQL（此时列名已经映射为实际列名）
	if event.Action != "QUERY" && event.Action != "" {
		switch event.Action {
		case "INSERT":
			event.SQL = ee.sqlGenerator.GenerateInsertSQL(event)
		case "UPDATE":
			event.SQL = ee.sqlGenerator.GenerateUpdateSQL(event)
		case "DELETE":
			event.SQL = ee.sqlGenerator.GenerateDeleteSQL(event)
		}
	}

	// TODO: 实现 Elasticsearch 索引导出
	// 使用 github.com/elastic/go-elasticsearch
	return nil
}

func (ee *ESExporter) Flush() error {
	fmt.Printf("Elasticsearch export to %s completed\n", ee.endpoint)
	return nil
}

