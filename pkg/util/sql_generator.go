package util

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/monitor"
)

// SQLGenerator SQL 生成器
type SQLGenerator struct {
	// columnTypes 用于存储列类型信息（可选）
	// 键为 "schema.table.columnName"
	columnTypes map[string]DataType
	monitor     *monitor.Monitor // 用于性能监控
}

// GenerateInsertSQL 生成 INSERT SQL
func (sg *SQLGenerator) GenerateInsertSQL(event *models.Event) string {
	start := time.Now()
	defer func() {
		if sg.monitor != nil {
			duration := time.Since(start)
			sg.monitor.LogSlowMethod("GenerateInsertSQL", duration, fmt.Sprintf("db=%s,table=%s", event.Database, event.Table))
		}
	}()

	if event.Action != "INSERT" {
		return ""
	}

	// 构建列列表和值列表
	columns := make([]string, 0)
	values := make([]string, 0)

	for k, v := range event.AfterValues {
		columns = append(columns, fmt.Sprintf("`%s`", escapeBacktick(k)))
		values = append(values, sg.formatValue(v))
	}

	if len(columns) == 0 {
		return ""
	}

	sql := fmt.Sprintf(
		"INSERT INTO `%s`.`%s` (%s) VALUES (%s)",
		escapeBacktick(event.Database),
		escapeBacktick(event.Table),
		strings.Join(columns, ", "),
		strings.Join(values, ", "),
	)
	return sql
}

// GenerateUpdateSQL 生成 UPDATE SQL
func (sg *SQLGenerator) GenerateUpdateSQL(event *models.Event) string {
	start := time.Now()
	defer func() {
		if sg.monitor != nil {
			duration := time.Since(start)
			sg.monitor.LogSlowMethod("GenerateUpdateSQL", duration, fmt.Sprintf("db=%s,table=%s", event.Database, event.Table))
		}
	}()

	if event.Action != "UPDATE" {
		return ""
	}

	// 构建 SET 子句
	setParts := make([]string, 0)
	for k, v := range event.AfterValues {
		setParts = append(setParts, fmt.Sprintf("`%s`=%s", escapeBacktick(k), sg.formatValue(v)))
	}

	if len(setParts) == 0 {
		return ""
	}

	// WHERE 子句
	whereParts := make([]string, 0)
	for k, v := range event.BeforeValues {
		whereParts = append(whereParts, fmt.Sprintf("`%s`=%s", escapeBacktick(k), sg.formatValue(v)))
	}

	sql := fmt.Sprintf(
		"UPDATE `%s`.`%s` SET %s WHERE %s",
		escapeBacktick(event.Database),
		escapeBacktick(event.Table),
		strings.Join(setParts, ", "),
		strings.Join(whereParts, " AND "),
	)
	return sql
}

// GenerateDeleteSQL 生成 DELETE SQL
func (sg *SQLGenerator) GenerateDeleteSQL(event *models.Event) string {
	start := time.Now()
	defer func() {
		if sg.monitor != nil {
			duration := time.Since(start)
			sg.monitor.LogSlowMethod("GenerateDeleteSQL", duration, fmt.Sprintf("db=%s,table=%s", event.Database, event.Table))
		}
	}()

	if event.Action != "DELETE" {
		return ""
	}

	// WHERE 子句
	whereParts := make([]string, 0)
	for k, v := range event.BeforeValues {
		whereParts = append(whereParts, fmt.Sprintf("`%s`=%s", escapeBacktick(k), sg.formatValue(v)))
	}

	if len(whereParts) == 0 {
		return ""
	}

	sql := fmt.Sprintf(
		"DELETE FROM `%s`.`%s` WHERE %s",
		escapeBacktick(event.Database),
		escapeBacktick(event.Table),
		strings.Join(whereParts, " AND "),
	)
	return sql
}

// GenerateRollbackSQL 生成回滚 SQL
func (sg *SQLGenerator) GenerateRollbackSQL(event *models.Event) string {
	switch event.Action {
	case "INSERT":
		// INSERT 的回滚是 DELETE
		return sg.GenerateDeleteSQL(&models.Event{
			Database:     event.Database,
			Table:        event.Table,
			Action:       "DELETE",
			BeforeValues: event.AfterValues,
		})
	case "UPDATE":
		// UPDATE 的回滚是反向 UPDATE
		return sg.GenerateUpdateSQL(&models.Event{
			Database:     event.Database,
			Table:        event.Table,
			Action:       "UPDATE",
			BeforeValues: event.AfterValues,
			AfterValues:  event.BeforeValues,
		})
	case "DELETE":
		// DELETE 的回滚是 INSERT
		return sg.GenerateInsertSQL(&models.Event{
			Database:    event.Database,
			Table:       event.Table,
			Action:      "INSERT",
			AfterValues: event.BeforeValues,
		})
	}
	return ""
}

// formatValue 格式化值，支持复杂数据类型
func (sg *SQLGenerator) formatValue(v interface{}) string {
	if v == nil {
		return "NULL"
	}

	switch val := v.(type) {
	// 数值类型 - 整数
	case int:
		return fmt.Sprintf("%d", val)
	case int8:
		return fmt.Sprintf("%d", val)
	case int16:
		return fmt.Sprintf("%d", val)
	case int32:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case uint:
		return fmt.Sprintf("%d", val)
	case uint8:
		return fmt.Sprintf("%d", val)
	case uint16:
		return fmt.Sprintf("%d", val)
	case uint32:
		return fmt.Sprintf("%d", val)
	case uint64:
		return fmt.Sprintf("%d", val)

	// 数值类型 - 浮点数
	case float32:
		return sg.formatDecimal(float64(val))
	case float64:
		return sg.formatDecimal(val)

	// 大整数
	case big.Int:
		return val.String()
	case *big.Int:
		if val == nil {
			return "NULL"
		}
		return val.String()

	// 字符串类型
	case string:
		return fmt.Sprintf("'%s'", escapeSingleQuote(val))

	// 二进制数据
	case []byte:
		return sg.formatBinary(val)

	// 布尔类型
	case bool:
		if val {
			return "1"
		}
		return "0"

	// 时间类型
	case time.Time:
		return sg.formatDateTime(val)

	// JSON 和其他复杂类型
	case map[string]interface{}:
		return sg.formatJSON(val)

	case []interface{}:
		return sg.formatJSONArray(val)

	// 对于其他类型，尝试转换
	default:
		return sg.formatUnknown(v)
	}
}

// formatDecimal 格式化浮点数，支持科学计数法检测和精度处理
func (sg *SQLGenerator) formatDecimal(val float64) string {
	// 检查是否是整数
	if val == math.Floor(val) && val >= math.MinInt64 && val <= math.MaxInt64 {
		return fmt.Sprintf("%d", int64(val))
	}

	// 检查是否是 NaN 或 Inf
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return "NULL"
	}

	// 格式化浮点数，避免过长的小数位
	// 使用 'g' 格式，最多保留 10 位有效数字
	result := fmt.Sprintf("%.10g", val)

	// 确保浮点数包含小数点或 e
	if !strings.Contains(result, ".") && !strings.Contains(result, "e") && !strings.Contains(result, "E") {
		result = result + ".0"
	}

	return result
}

// formatBinary 格式化二进制数据
func (sg *SQLGenerator) formatBinary(data []byte) string {
	if len(data) == 0 {
		return "0x00"
	}

	// 尝试识别特殊的二进制格式
	// UUID 通常是 16 字节
	if len(data) == 16 && isValidUUID(data) {
		// 尝试作为 UUID 处理
		return fmt.Sprintf("'%s'", formatUUID(data))
	}

	// GUID/BINARY(36) 格式
	if len(data) <= 36 {
		str := string(data)
		if isPrintableString(str) {
			return fmt.Sprintf("'%s'", escapeSingleQuote(str))
		}
	}

	// 默认作为十六进制处理
	return fmt.Sprintf("0x%x", data)
}

// formatDateTime 格式化日期时间，支持微秒精度
func (sg *SQLGenerator) formatDateTime(t time.Time) string {
	// 检查是否有非零的纳秒部分
	nanosecond := t.Nanosecond()
	if nanosecond > 0 {
		// 转换为微秒（保留 6 位小数）
		microsecond := nanosecond / 1000
		return fmt.Sprintf("'%s.%06d'", t.Format("2006-01-02 15:04:05"), microsecond)
	}

	return fmt.Sprintf("'%s'", t.Format("2006-01-02 15:04:05"))
}

// formatJSON 格式化 JSON 对象
func (sg *SQLGenerator) formatJSON(val map[string]interface{}) string {
	data, err := json.Marshal(val)
	if err != nil {
		return "'{}'  "
	}
	return fmt.Sprintf("'%s'", escapeSingleQuote(string(data)))
}

// formatJSONArray 格式化 JSON 数组
func (sg *SQLGenerator) formatJSONArray(val []interface{}) string {
	data, err := json.Marshal(val)
	if err != nil {
		return "'[]'"
	}
	return fmt.Sprintf("'%s'", escapeSingleQuote(string(data)))
}

// formatUnknown 格式化未知类型
func (sg *SQLGenerator) formatUnknown(v interface{}) string {
	// 尝试 json.Marshaler 接口
	if marshaler, ok := v.(json.Marshaler); ok {
		data, err := marshaler.MarshalJSON()
		if err == nil {
			return fmt.Sprintf("'%s'", escapeSingleQuote(string(data)))
		}
	}

	// 尝试转换为 JSON 字符串
	data, err := json.Marshal(v)
	if err == nil {
		jsonStr := string(data)
		// 如果是字符串，移除额外的引号
		if strings.HasPrefix(jsonStr, "\"") && strings.HasSuffix(jsonStr, "\"") {
			return fmt.Sprintf("'%s'", escapeSingleQuote(jsonStr[1:len(jsonStr)-1]))
		}
		return fmt.Sprintf("'%s'", escapeSingleQuote(jsonStr))
	}

	// 最后手段：转换为字符串表示
	return fmt.Sprintf("'%s'", escapeSingleQuote(fmt.Sprintf("%v", v)))
}

// escapeSingleQuote 转义单引号和特殊字符
func escapeSingleQuote(s string) string {
	// 替换 ' 为 \'，反斜杠为 \\
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

// escapeBacktick 转义反引号
func escapeBacktick(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}

// isValidUUID 检查是否为有效的 UUID
func isValidUUID(data []byte) bool {
	// UUID 通常不包含 0 字节（除非特殊情况）
	// 我们检查是否大多数字节都是可打印字符或常见的字节值
	printableCount := 0
	for _, b := range data {
		if (b >= 32 && b <= 126) || b == 0 {
			printableCount++
		}
	}
	return printableCount > len(data)/2
}

// formatUUID 格式化 UUID（将二进制转换为标准格式）
func formatUUID(data []byte) string {
	if len(data) != 16 {
		return fmt.Sprintf("%x", data)
	}

	// 标准 UUID 格式：8-4-4-4-12
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		data[0:4], data[4:6], data[6:8], data[8:10], data[10:16],
	)
}

// isPrintableString 检查是否为可打印字符串
func isPrintableString(s string) bool {
	if len(s) == 0 {
		return false
	}

	printableCount := 0
	for _, r := range s {
		if (r >= 32 && r <= 126) || r == '\n' || r == '\r' || r == '\t' {
			printableCount++
		}
	}

	// 至少 80% 的字符是可打印的
	return float64(printableCount)/float64(len(s)) >= 0.8
}

// NewSQLGenerator 创建 SQL 生成器
func NewSQLGenerator() *SQLGenerator {
	return &SQLGenerator{
		columnTypes: make(map[string]DataType),
		monitor:     nil,
	}
}

// SetMonitor 设置监控器
func (sg *SQLGenerator) SetMonitor(m *monitor.Monitor) {
	sg.monitor = m
}

// SetColumnType 设置列的数据类型
func (sg *SQLGenerator) SetColumnType(schemaName, tableName, columnName string, dataType DataType) {
	key := fmt.Sprintf("%s.%s.%s", schemaName, tableName, columnName)
	sg.columnTypes[key] = dataType
}

// GetColumnType 获取列的数据类型
func (sg *SQLGenerator) GetColumnType(schemaName, tableName, columnName string) DataType {
	key := fmt.Sprintf("%s.%s.%s", schemaName, tableName, columnName)
	return sg.columnTypes[key]
}

// FormatColumnValue 格式化单个列值（用于导出或其他场景）
func (sg *SQLGenerator) FormatColumnValue(value interface{}) string {
	return sg.formatValue(value)
}

// ValidateSQL 验证生成的 SQL 是否有效（基本检查）
func (sg *SQLGenerator) ValidateSQL(sql string) bool {
	if sql == "" {
		return false
	}

	// 检查是否以有效的 SQL 关键字开头
	upperSQL := strings.ToUpper(strings.TrimSpace(sql))
	validKeywords := []string{"INSERT", "UPDATE", "DELETE", "SELECT"}

	for _, kw := range validKeywords {
		if strings.HasPrefix(upperSQL, kw) {
			return true
		}
	}

	return false
}
