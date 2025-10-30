package cache

import (
	"testing"

	"github.com/aitoooooo/binlogx/pkg/models"
)

func TestMetaCacheFallback(t *testing.T) {
	// 无数据库连接时，缓存应该返回回退列名
	mc := NewMetaCache(nil, 100)

	colName := mc.GetColumnName("testdb", "testtable", 5)
	expected := "col_5"

	if colName != expected {
		t.Errorf("GetColumnName with nil db should return col_N format, got %s", colName)
	}
}

func TestMetaCacheSize(t *testing.T) {
	mc := NewMetaCache(nil, 2)

	// 测试 LRU 缓存限制
	mc.cache["db1.table1"] = &models.TableMeta{Columns: []models.ColumnMeta{{Name: "col1", Type: "INT"}}}
	mc.cache["db2.table2"] = &models.TableMeta{Columns: []models.ColumnMeta{{Name: "col2", Type: "VARCHAR"}}}

	if len(mc.cache) > 2 {
		t.Errorf("Cache size exceeded maxSize=2, got %d", len(mc.cache))
	}
}

func TestMetaCacheClear(t *testing.T) {
	mc := NewMetaCache(nil, 100)
	mc.cache["db1.table1"] = &models.TableMeta{Columns: []models.ColumnMeta{{Name: "col1", Type: "INT"}}}

	if len(mc.cache) == 0 {
		t.Error("Cache should have entries before clear")
	}

	mc.Clear()

	if len(mc.cache) != 0 {
		t.Errorf("Cache should be empty after Clear, got %d entries", len(mc.cache))
	}
}

