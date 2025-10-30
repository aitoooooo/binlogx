package cache

import (
	"testing"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/monitor"
)

func TestMetaCacheFallback(t *testing.T) {
	// 无数据库连接时，缓存应该返回回退列名
	mc := NewMetaCache(nil, 100, nil)

	colName := mc.GetColumnName("testdb", "testtable", 5)
	expected := "col_5"

	if colName != expected {
		t.Errorf("GetColumnName with nil db should return col_N format, got %s", colName)
	}
}

func TestMetaCacheSize(t *testing.T) {
	mc := NewMetaCache(nil, 2, nil)

	// 测试 LRU 缓存限制
	mc.cache["db1.table1"] = &models.TableMeta{Columns: []models.ColumnMeta{{Name: "col1", Type: "INT"}}}
	mc.cache["db2.table2"] = &models.TableMeta{Columns: []models.ColumnMeta{{Name: "col2", Type: "VARCHAR"}}}

	if len(mc.cache) > 2 {
		t.Errorf("Cache size exceeded maxSize=2, got %d", len(mc.cache))
	}
}

func TestMetaCacheClear(t *testing.T) {
	mc := NewMetaCache(nil, 100, nil)
	mc.cache["db1.table1"] = &models.TableMeta{Columns: []models.ColumnMeta{{Name: "col1", Type: "INT"}}}

	if len(mc.cache) == 0 {
		t.Error("Cache should have entries before clear")
	}

	mc.Clear()

	if len(mc.cache) != 0 {
		t.Errorf("Cache should be empty after Clear, got %d entries", len(mc.cache))
	}
}

func TestTableNotFoundCache(t *testing.T) {
	// 创建一个不存在的表缓存条目
	tnc := &TableNotFoundCache{
		timestamp: time.Now(),
		ttl:       100 * time.Millisecond,
	}

	// 立即检查，应该未过期
	if tnc.IsExpired() {
		t.Errorf("Expected cache to be valid, but it's expired")
	}

	// 等待超时
	time.Sleep(150 * time.Millisecond)

	// 现在检查，应该已过期
	if !tnc.IsExpired() {
		t.Errorf("Expected cache to be expired, but it's valid")
	}
}

func TestMetaCacheSetMonitor(t *testing.T) {
	mc := NewMetaCache(nil, 100, nil)
	mon := monitor.NewMonitor(100*time.Millisecond, 0)

	// 设置监控器
	mc.SetMonitor(mon)

	if mc.monitor != mon {
		t.Errorf("Expected monitor to be set, but got %v", mc.monitor)
	}
}

func TestClearNotFoundCache(t *testing.T) {
	mc := NewMetaCache(nil, 100, nil)

	// 手动添加一个表不存在的缓存条目
	key := "testdb.testtable"
	mc.notFoundCache[key] = &TableNotFoundCache{
		timestamp: time.Now(),
		ttl:       1 * time.Minute,
	}

	// 验证缓存条目存在
	if _, ok := mc.notFoundCache[key]; !ok {
		t.Errorf("Expected to find cache entry for %s", key)
	}

	// 清空缓存
	mc.Clear()

	// 验证缓存被清空
	if _, ok := mc.notFoundCache[key]; ok {
		t.Errorf("Expected cache to be empty after Clear()")
	}
}
