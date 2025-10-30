package cache

import (
	"database/sql"
	"fmt"
	"sync"

	"github.com/aitoooooo/binlogx/pkg/models"
)

// MetaCache 表元数据缓存
type MetaCache struct {
	mu      sync.RWMutex
	cache   map[string]*models.TableMeta
	maxSize int
	db      *sql.DB
}

// NewMetaCache 创建缓存
func NewMetaCache(db *sql.DB, maxSize int) *MetaCache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &MetaCache{
		cache:   make(map[string]*models.TableMeta),
		maxSize: maxSize,
		db:      db,
	}
}

// GetTableMeta 获取表元数据
func (mc *MetaCache) GetTableMeta(schema, table string) (*models.TableMeta, error) {
	if mc.db == nil {
		// 无数据库连接，回退到默认列名
		return nil, fmt.Errorf("no database connection available")
	}

	key := schema + "." + table

	mc.mu.RLock()
	if meta, ok := mc.cache[key]; ok {
		mc.mu.RUnlock()
		return meta, nil
	}
	mc.mu.RUnlock()

	// 从数据库查询
	meta, err := mc.queryTableMeta(schema, table)
	if err != nil {
		return nil, err
	}

	// 写入缓存
	mc.mu.Lock()
	if len(mc.cache) < mc.maxSize {
		mc.cache[key] = meta
	} else {
		// LRU 淘汰：简单实现为清空一半缓存
		for k := range mc.cache {
			delete(mc.cache, k)
			if len(mc.cache) < mc.maxSize/2 {
				break
			}
		}
		mc.cache[key] = meta
	}
	mc.mu.Unlock()

	return meta, nil
}

// queryTableMeta 从数据库查询表元数据
func (mc *MetaCache) queryTableMeta(schema, table string) (*models.TableMeta, error) {
	query := `
		SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION
	`

	rows, err := mc.db.Query(query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []models.ColumnMeta
	for rows.Next() {
		var colName, colType, isNullable string
		var defaultValue interface{}
		if err := rows.Scan(&colName, &colType, &isNullable, &defaultValue); err != nil {
			return nil, err
		}

		columns = append(columns, models.ColumnMeta{
			Name:     colName,
			Type:     colType,
			Nullable: isNullable == "YES",
			Default:  defaultValue,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(columns) == 0 {
		return nil, fmt.Errorf("table %s.%s not found", schema, table)
	}

	return &models.TableMeta{Columns: columns}, nil
}

// GetColumnName 获取列名，如果失败返回 col_N
func (mc *MetaCache) GetColumnName(schema, table string, index int) string {
	meta, err := mc.GetTableMeta(schema, table)
	if err != nil || index >= len(meta.Columns) {
		return fmt.Sprintf("col_%d", index)
	}
	return meta.Columns[index].Name
}

// Clear 清空缓存
func (mc *MetaCache) Clear() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.cache = make(map[string]*models.TableMeta)
}
