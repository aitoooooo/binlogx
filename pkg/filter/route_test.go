package filter

import (
	"testing"

	"github.com/aitoooooo/binlogx/pkg/models"
)

func TestRouteFilterWithIncludeDB(t *testing.T) {
	rf, err := NewRouteFilter([]string{"db1", "db2"}, []string{}, "", "")
	if err != nil {
		t.Fatalf("NewRouteFilter failed: %v", err)
	}

	tests := []struct {
		db       string
		table    string
		expected bool
	}{
		{"db1", "table1", true},
		{"db2", "table2", true},
		{"db3", "table1", false},
	}

	for _, test := range tests {
		event := &models.Event{Database: test.db, Table: test.table}
		result := rf.Match(event)
		if result != test.expected {
			t.Errorf("Match(%s, %s) = %v, expected %v", test.db, test.table, result, test.expected)
		}
	}
}

func TestRouteFilterWithRegex(t *testing.T) {
	rf, err := NewRouteFilter([]string{}, []string{}, `db_[0-9]`, `table_[0-2]`)
	if err != nil {
		t.Fatalf("NewRouteFilter failed: %v", err)
	}

	tests := []struct {
		db       string
		table    string
		expected bool
	}{
		{"db_1", "table_0", true},
		{"db_5", "table_1", true},
		{"db_1", "table_3", false},
		{"db_a", "table_0", false},
	}

	for _, test := range tests {
		event := &models.Event{Database: test.db, Table: test.table}
		result := rf.Match(event)
		if result != test.expected {
			t.Errorf("Match(%s, %s) = %v, expected %v", test.db, test.table, result, test.expected)
		}
	}
}

func TestGetWorkerID(t *testing.T) {
	rf, _ := NewRouteFilter([]string{}, []string{}, "", "")

	// 同一 table+pk 应该返回相同的 workerID
	id1 := rf.GetWorkerID("users", "user:123", 4)
	id2 := rf.GetWorkerID("users", "user:123", 4)

	if id1 != id2 {
		t.Errorf("GetWorkerID should be deterministic: got %d and %d", id1, id2)
	}

	// workerID 应该在范围内
	if id1 < 0 || id1 >= 4 {
		t.Errorf("GetWorkerID(%d) out of range [0, 4)", id1)
	}
}
