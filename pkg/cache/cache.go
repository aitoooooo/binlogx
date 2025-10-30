package cache

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/monitor"
	"golang.org/x/sync/singleflight"
)

// TableNotFoundCache 表示表不存在缓存条目
type TableNotFoundCache struct {
	timestamp time.Time
	ttl       time.Duration
}

// IsExpired 检查缓存条目是否过期
func (tnc *TableNotFoundCache) IsExpired() bool {
	return time.Since(tnc.timestamp) > tnc.ttl
}

// MetaCache 表元数据缓存
type MetaCache struct {
	mu               sync.RWMutex
	cache            map[string]*models.TableMeta
	notFoundCache    map[string]*TableNotFoundCache // 表不存在的缓存，TTL=1分钟
	maxSize          int
	notFoundCacheTTL time.Duration
	db               *sql.DB
	monitor          *monitor.Monitor   // 用于性能监控
	sf               singleflight.Group // 用于防止并发查询同一个表
}

// NewMetaCache 创建缓存
func NewMetaCache(db *sql.DB, maxSize int, m *monitor.Monitor) *MetaCache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &MetaCache{
		cache:            make(map[string]*models.TableMeta),
		notFoundCache:    make(map[string]*TableNotFoundCache),
		maxSize:          maxSize,
		notFoundCacheTTL: 1 * time.Minute, // 表不存在缓存1分钟
		db:               db,
		monitor:          m,
	}
}

// GetTableMeta 获取表元数据
func (mc *MetaCache) GetTableMeta(schema, table string) (*models.TableMeta, error) {
	start := time.Now()
	defer func() {
		if mc.monitor != nil {
			mc.monitor.LogSlowMethod("GetTableMeta", start, fmt.Sprintf("schema=%s,table=%s", schema, table))
		}
	}()

	if mc.db == nil {
		// 无数据库连接，回退到默认列名
		return nil, fmt.Errorf("no database connection available")
	}

	key := schema + "." + table

	// 先检查表是否在不存在缓存中（在 1 分钟内不再查询）
	mc.mu.RLock()
	if nfc, ok := mc.notFoundCache[key]; ok && !nfc.IsExpired() {
		mc.mu.RUnlock()
		return nil, fmt.Errorf("table %s.%s not found (cached)", schema, table)
	}
	mc.mu.RUnlock()

	// 检查表元数据缓存
	mc.mu.RLock()
	if meta, ok := mc.cache[key]; ok {
		// log.Printf("命中  %s", key)
		mc.mu.RUnlock()
		return meta, nil
	}
	mc.mu.RUnlock()

	// 使用 singleflight 防止并发查询同一个表
	result, err, _ := mc.sf.Do(key, func() (interface{}, error) {
		// 再次检查缓存，因为在等待 singleflight 过程中可能已经被其他 goroutine 写入
		mc.mu.RLock()
		if meta, ok := mc.cache[key]; ok {
			mc.mu.RUnlock()
			return meta, nil
		}
		mc.mu.RUnlock()

		// 从数据库查询
		meta, err := mc.queryTableMeta(schema, table)
		if err != nil {
			// 表不存在，添加到不存在缓存，1 分钟内不再查询
			mc.mu.Lock()
			mc.notFoundCache[key] = &TableNotFoundCache{
				timestamp: time.Now(),
				ttl:       mc.notFoundCacheTTL,
			}
			mc.mu.Unlock()
			return nil, err
		}

		// 写入缓存
		mc.mu.Lock()
		if len(mc.cache) < mc.maxSize {
			mc.cache[key] = meta
		} else {
			// LRU 淘汰：简单实现为清空一半缓存
			for k := range mc.cache {
				log.Printf("淘汰  %s", key)
				delete(mc.cache, k)
				if len(mc.cache) < mc.maxSize/2 {
					break
				}
			}
			mc.cache[key] = meta
		}
		mc.mu.Unlock()

		return meta, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*models.TableMeta), nil
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
	mc.notFoundCache = make(map[string]*TableNotFoundCache)
}
