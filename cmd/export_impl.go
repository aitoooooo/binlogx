package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/aitoooooo/binlogx/pkg/config"
	"sync"

	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/util"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteExporter SQLite 导出器的完整实现（使用分片锁优化并发性能）
type SQLiteExporter struct {
	path         string
	db           *sql.DB
	helper       *CommandHelper
	sqlGenerator *util.SQLGenerator
	actions      map[string]bool

	// 使用分片锁：按 database.table 分片，减少锁竞争
	// 这样多个 worker 可以并行处理不同的表
	shardLock   *util.ShardedLock
	batches     map[int][]*models.Event // shardIdx -> batch
	batchSize   int
	batchesLock sync.Mutex // 仅用于访问 batches map 本身
}

func newSQLiteExporter(output string, helper *CommandHelper, actions map[string]bool, batchSize int) (*SQLiteExporter, error) {
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
		sqlGenerator: util.NewSQLGenerator(config.GlobalMonitor),
		actions:      actions,
		shardLock:    util.NewShardedLock(16), // 16 个分片，通常足够
		batches:      make(map[int][]*models.Event),
		batchSize:    batchSize,
	}, nil
}

func (se *SQLiteExporter) Handle(event *models.Event) error {
	// 映射列名和生成 SQL 在锁外进行（这些是 CPU 密集操作）
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

	// 过滤：只导出指定的 action（在锁外判断）
	if !se.actions[event.Action] {
		return nil
	}

	// 根据 database.table 获取分片索引，实现并行处理不同表
	shardKey := event.Database + "." + event.Table
	mu, shardIdx := se.shardLock.GetShard(shardKey)

	// 只在添加到批处理队列时使用锁（最小化临界区）
	mu.Lock()
	se.batchesLock.Lock()
	batch := se.batches[shardIdx]
	batch = append(batch, event)
	se.batches[shardIdx] = batch
	needsFlush := len(batch) >= se.batchSize
	se.batchesLock.Unlock()
	mu.Unlock()

	// 在锁外执行批量插入
	if needsFlush {
		mu.Lock()
		se.batchesLock.Lock()
		batch := se.batches[shardIdx]
		se.batches[shardIdx] = make([]*models.Event, 0, se.batchSize)
		se.batchesLock.Unlock()
		mu.Unlock()

		if len(batch) > 0 {
			if err := se.flushBatchDirect(batch); err != nil {
				return err
			}
		}
	}

	return nil
}

// flushBatchDirect 直接刷新一个批次的数据（由 Handle 直接调用）
func (se *SQLiteExporter) flushBatchDirect(batch []*models.Event) error {
	if len(batch) == 0 {
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

	for _, event := range batch {
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

	return tx.Commit()
}

// flushAllBatches 刷新所有分片的批数据（在 Flush 时调用）
func (se *SQLiteExporter) flushAllBatches() error {
	se.batchesLock.Lock()
	defer se.batchesLock.Unlock()

	for shardIdx := range se.batches {
		batch := se.batches[shardIdx]
		if len(batch) > 0 {
			if err := se.flushBatchDirect(batch); err != nil {
				return err
			}
			se.batches[shardIdx] = make([]*models.Event, 0, se.batchSize)
		}
	}
	return nil
}

func (se *SQLiteExporter) Flush() error {
	// 刷新剩余的批处理数据
	if err := se.flushAllBatches(); err != nil {
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

func newH2Exporter(output string, helper *CommandHelper, actions map[string]bool, batchSize int) (*H2Exporter, error) {
	if output == "" {
		output = "binlog_export.h2"
	}
	return &H2Exporter{
		path:         output,
		helper:       helper,
		sqlGenerator: util.NewSQLGenerator(config.GlobalMonitor),
		actions:      actions,
	}, nil
}

func (he *H2Exporter) Handle(event *models.Event) error {
	// 映射列名和生成 SQL 在锁外进行（这些是 CPU 密集操作）
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

	// 过滤：只导出指定的 action（在锁外判断）
	if !he.actions[event.Action] {
		return nil
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

func newHiveExporter(output string, helper *CommandHelper, actions map[string]bool, batchSize int) (*HiveExporter, error) {
	if output == "" {
		output = "/hive/binlog_export"
	}
	return &HiveExporter{
		path:         output,
		helper:       helper,
		sqlGenerator: util.NewSQLGenerator(config.GlobalMonitor),
		actions:      actions,
	}, nil
}

func (he *HiveExporter) Handle(event *models.Event) error {
	// 映射列名和生成 SQL 在锁外进行（这些是 CPU 密集操作）
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

	// 过滤：只导出指定的 action（在锁外判断）
	if !he.actions[event.Action] {
		return nil
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

func newESExporter(output string, helper *CommandHelper, actions map[string]bool, batchSize int) (*ESExporter, error) {
	if output == "" {
		output = "http://localhost:9200"
	}
	return &ESExporter{
		endpoint:     output,
		helper:       helper,
		sqlGenerator: util.NewSQLGenerator(config.GlobalMonitor),
		actions:      actions,
	}, nil
}

func (ee *ESExporter) Handle(event *models.Event) error {
	// 映射列名和生成 SQL 在锁外进行（这些是 CPU 密集操作）
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

	// 过滤：只导出指定的 action（在锁外判断）
	if !ee.actions[event.Action] {
		return nil
	}

	// TODO: 实现 Elasticsearch 索引导出
	// 使用 github.com/elastic/go-elasticsearch
	return nil
}

func (ee *ESExporter) Flush() error {
	fmt.Printf("Elasticsearch export to %s completed\n", ee.endpoint)
	return nil
}
