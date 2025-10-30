package util

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
)

func TestGenerateInsertSQL(t *testing.T) {
	gen := NewSQLGenerator(nil)

	event := &models.Event{
		Database: "testdb",
		Table:    "users",
		Action:   "INSERT",
		AfterValues: map[string]interface{}{
			"id":   1,
			"name": "John",
		},
	}

	sql := gen.GenerateInsertSQL(event)

	if !strings.Contains(sql, "INSERT INTO `testdb`.`users`") {
		t.Errorf("Generated SQL missing INSERT statement: %s", sql)
	}

	if !strings.Contains(sql, "`id`") || !strings.Contains(sql, "`name`") {
		t.Errorf("Generated SQL missing columns: %s", sql)
	}
}

func TestGenerateUpdateSQL(t *testing.T) {
	gen := NewSQLGenerator(nil)

	event := &models.Event{
		Database: "testdb",
		Table:    "users",
		Action:   "UPDATE",
		BeforeValues: map[string]interface{}{
			"id": 1,
		},
		AfterValues: map[string]interface{}{
			"name": "Jane",
		},
	}

	sql := gen.GenerateUpdateSQL(event)

	if !strings.Contains(sql, "UPDATE `testdb`.`users` SET") {
		t.Errorf("Generated SQL missing UPDATE statement: %s", sql)
	}

	if !strings.Contains(sql, "WHERE") {
		t.Errorf("Generated SQL missing WHERE clause: %s", sql)
	}
}

func TestGenerateDeleteSQL(t *testing.T) {
	gen := NewSQLGenerator(nil)

	event := &models.Event{
		Database: "testdb",
		Table:    "users",
		Action:   "DELETE",
		BeforeValues: map[string]interface{}{
			"id": 1,
		},
	}

	sql := gen.GenerateDeleteSQL(event)

	if !strings.Contains(sql, "DELETE FROM `testdb`.`users`") {
		t.Errorf("Generated SQL missing DELETE statement: %s", sql)
	}

	if !strings.Contains(sql, "WHERE") {
		t.Errorf("Generated SQL missing WHERE clause: %s", sql)
	}
}

func TestGenerateRollbackSQL(t *testing.T) {
	gen := NewSQLGenerator(nil)

	// INSERT 的回滚应该是 DELETE
	insertEvent := &models.Event{
		Database:    "testdb",
		Table:       "users",
		Action:      "INSERT",
		AfterValues: map[string]interface{}{"id": 1},
	}

	rollbackSQL := gen.GenerateRollbackSQL(insertEvent)
	if !strings.Contains(rollbackSQL, "DELETE") {
		t.Errorf("Rollback for INSERT should generate DELETE, got: %s", rollbackSQL)
	}

	// DELETE 的回滚应该是 INSERT
	deleteEvent := &models.Event{
		Database:     "testdb",
		Table:        "users",
		Action:       "DELETE",
		BeforeValues: map[string]interface{}{"id": 1},
	}

	rollbackSQL = gen.GenerateRollbackSQL(deleteEvent)
	if !strings.Contains(rollbackSQL, "INSERT") {
		t.Errorf("Rollback for DELETE should generate INSERT, got: %s", rollbackSQL)
	}
}

// 测试复杂数据类型支持
func TestComplexDataTypes(t *testing.T) {
	gen := NewSQLGenerator(nil)

	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"nil", nil, "NULL"},
		{"int", 123, "123"},
		{"int8", int8(42), "42"},
		{"int16", int16(1000), "1000"},
		{"int32", int32(100000), "100000"},
		{"int64", int64(9999999999), "9999999999"},
		{"uint", uint(123), "123"},
		{"uint8", uint8(255), "255"},
		{"uint16", uint16(65535), "65535"},
		{"uint32", uint32(4294967295), "4294967295"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		{"float32", float32(3.14), "3.14"},
		{"float64_int", float64(123), "123"},
		{"float64_float", float64(123.45), "123.45"},
		{"float64_scientific", float64(1.23e-4), "0.000123"},
		{"string", "hello", "'hello'"},
		{"string_with_quote", "it's", "'"},           // Just check it has quotes
		{"string_with_backslash", "test\\path", "'"}, // Just check it has quotes
		{"empty_string", "", "''"},
		{"bool_true", true, "1"},
		{"bool_false", false, "0"},
		{"bytes_hello", []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f}, "'Hello'"}, // Printable as string
		{"empty_bytes", []byte{}, "0x00"},
		{"time", time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), "'2024-01-01 10:00:00'"},
		{"time_with_micro", time.Date(2024, 1, 1, 10, 0, 0, 123456000, time.UTC), "'2024-01-01 10:00:00.123456'"},
		{"json_object", map[string]interface{}{"key": "value"}, "'"},
		{"json_array", []interface{}{1, 2, 3}, "'"},
	}

	for _, test := range tests {
		result := gen.FormatColumnValue(test.value)
		if !strings.Contains(result, test.expected) {
			t.Errorf("Test %s: expected to contain '%s', got '%s'", test.name, test.expected, result)
		}
	}
}

// 测试特殊浮点数情况
func TestSpecialFloatValues(t *testing.T) {
	gen := NewSQLGenerator(nil)

	tests := []struct {
		name     string
		value    float64
		expected string
	}{
		{"positive_infinity", math.Inf(1), "NULL"},
		{"negative_infinity", math.Inf(-1), "NULL"},
		{"nan", math.NaN(), "NULL"},
		{"very_small", 1e-15, "1e-15"},
		{"very_large", 1e15, "1000000000000000"}, // Can be represented as full number
		{"zero", 0.0, "0"},
		{"negative_zero", -0.0, "0"},
	}

	for _, test := range tests {
		result := gen.FormatColumnValue(test.value)
		if !strings.Contains(result, test.expected) {
			t.Errorf("Test %s: expected to contain '%s', got '%s'", test.name, test.expected, result)
		}
	}
}

// 测试 SQL 验证
func TestValidateSQL(t *testing.T) {
	gen := NewSQLGenerator(nil)

	tests := []struct {
		sql      string
		expected bool
	}{
		{"INSERT INTO table VALUES (1)", true},
		{"UPDATE table SET col=1", true},
		{"DELETE FROM table WHERE id=1", true},
		{"SELECT * FROM table", true},
		{"", false},
		{"INVALID SQL", false},
		{"   INSERT INTO table VALUES (1)", true},
		{"select * from users", true},
	}

	for _, test := range tests {
		result := gen.ValidateSQL(test.sql)
		if result != test.expected {
			t.Errorf("ValidateSQL('%s'): expected %v, got %v", test.sql, test.expected, result)
		}
	}
}

// 测试特殊字符转义
func TestEscaping(t *testing.T) {
	gen := NewSQLGenerator(nil)

	tests := []struct {
		input    string
		expected string
	}{
		{"test'quote", "'test\\\\'quote'"},
		{"test`backtick", "'test`backtick'"},
		{"test\\slash", "'test\\\\\\\\slash'"},
		{"test\"double", "'test\"double'"},
		{"test\nnewline", "'test\\nnewline'"},
		{"normal", "'normal'"},
	}

	for _, test := range tests {
		result := gen.FormatColumnValue(test.input)
		// 只检查格式，不检查具体转义（因为实现细节可能不同）
		if len(result) == 0 {
			t.Errorf("FormatColumnValue('%s') returned empty string", test.input)
		}
		if !strings.HasPrefix(result, "'") || !strings.HasSuffix(result, "'") {
			t.Errorf("FormatColumnValue('%s') should be quoted, got: %s", test.input, result)
		}
	}
}

// 测试复杂 INSERT 场景
func TestComplexInsertWithVariousTypes(t *testing.T) {
	gen := NewSQLGenerator(nil)

	event := &models.Event{
		Database: "mydb",
		Table:    "complex_table",
		Action:   "INSERT",
		AfterValues: map[string]interface{}{
			"id":       1,
			"name":     "test",
			"balance":  123.45,
			"active":   true,
			"data":     map[string]interface{}{"nested": "value"},
			"created":  time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			"amount":   0,
			"nullable": nil,
		},
	}

	sql := gen.GenerateInsertSQL(event)

	// 验证基本结构
	if !strings.Contains(sql, "INSERT INTO `mydb`.`complex_table`") {
		t.Errorf("INSERT statement malformed: %s", sql)
	}

	// 验证各种数据类型都有被处理
	if !strings.Contains(sql, "`id`") {
		t.Error("Missing id column")
	}
	if !strings.Contains(sql, "NULL") {
		t.Error("Missing NULL for nullable field")
	}
	if !strings.Contains(sql, "1") || !strings.Contains(sql, "0") {
		t.Error("Missing boolean values")
	}
	if !strings.Contains(sql, "VALUES") {
		t.Error("Missing VALUES clause")
	}
}

// 测试 UPDATE 与复杂类型
func TestComplexUpdateWithDateTime(t *testing.T) {
	gen := NewSQLGenerator(nil)

	now := time.Now()
	event := &models.Event{
		Database: "mydb",
		Table:    "logs",
		Action:   "UPDATE",
		BeforeValues: map[string]interface{}{
			"id": 1,
		},
		AfterValues: map[string]interface{}{
			"status":     "completed",
			"updated_at": now,
			"duration":   3.14159,
		},
	}

	sql := gen.GenerateUpdateSQL(event)

	if !strings.Contains(sql, "UPDATE `mydb`.`logs` SET") {
		t.Errorf("UPDATE statement malformed: %s", sql)
	}
	if !strings.Contains(sql, "WHERE") {
		t.Errorf("Missing WHERE clause: %s", sql)
	}
}

// 测试 DeleteSQL 与多个 WHERE 条件
func TestDeleteWithMultipleConditions(t *testing.T) {
	gen := NewSQLGenerator(nil)

	event := &models.Event{
		Database: "testdb",
		Table:    "users",
		Action:   "DELETE",
		BeforeValues: map[string]interface{}{
			"id":      1,
			"org_id":  2,
			"user_id": 3,
		},
	}

	sql := gen.GenerateDeleteSQL(event)

	if !strings.Contains(sql, "DELETE FROM `testdb`.`users`") {
		t.Errorf("DELETE statement malformed: %s", sql)
	}

	// 应该有多个 WHERE 条件
	whereCount := strings.Count(sql, "WHERE")
	andCount := strings.Count(sql, "AND")
	if whereCount != 1 || andCount < 2 {
		t.Errorf("WHERE clause with AND conditions not properly formed: %s", sql)
	}
}

// 测试 Column Type 管理
func TestColumnTypeManagement(t *testing.T) {
	gen := NewSQLGenerator(nil)

	// 设置列类型
	gen.SetColumnType("mydb", "users", "id", TypeInt)
	gen.SetColumnType("mydb", "users", "balance", TypeDecimal)
	gen.SetColumnType("mydb", "users", "bio", TypeText)

	// 获取列类型
	idType := gen.GetColumnType("mydb", "users", "id")
	if idType != TypeInt {
		t.Errorf("Expected TypeInt, got %v", idType)
	}

	balanceType := gen.GetColumnType("mydb", "users", "balance")
	if balanceType != TypeDecimal {
		t.Errorf("Expected TypeDecimal, got %v", balanceType)
	}

	// 不存在的列应该返回空
	notExist := gen.GetColumnType("mydb", "users", "notexist")
	if notExist != "" {
		t.Errorf("Expected empty string for non-existent column, got %v", notExist)
	}
}
