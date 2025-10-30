package source

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"github.com/aitoooooo/binlogx/pkg/models"
)

// MySQLSource 在线 MySQL 数据源
type MySQLSource struct {
	dsn string
	db  *sql.DB
	eof bool
}

// NewMySQLSource 创建 MySQL 数据源
func NewMySQLSource(dsn string) *MySQLSource {
	return &MySQLSource{
		dsn: dsn,
	}
}

// Open 连接到 MySQL
func (ms *MySQLSource) Open(ctx context.Context) error {
	db, err := sql.Open("mysql", ms.dsn)
	if err != nil {
		return fmt.Errorf("failed to open MySQL connection: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping MySQL: %w", err)
	}

	ms.db = db
	ms.eof = false
	return nil
}

// Close 关闭连接
func (ms *MySQLSource) Close() error {
	if ms.db != nil {
		return ms.db.Close()
	}
	return nil
}

// Read 读取下一个事件
func (ms *MySQLSource) Read() (*models.Event, error) {
	if ms.eof {
		return nil, fmt.Errorf("EOF")
	}

	// TODO: 实现从 MySQL binlog 读取事件的逻辑
	// 这需要使用专门的 MySQL binlog 协议库

	return nil, fmt.Errorf("not implemented")
}

// HasMore 是否还有更多数据
func (ms *MySQLSource) HasMore() bool {
	return !ms.eof
}

// GetDB 获取数据库连接（用于列名缓存）
func (ms *MySQLSource) GetDB() *sql.DB {
	return ms.db
}
