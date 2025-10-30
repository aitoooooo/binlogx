package filter

import (
	"regexp"

	"github.com/aitoooooo/binlogx/pkg/models"
)

// RouteFilter 分库表正则路由过滤器
type RouteFilter struct {
	includeDB   map[string]bool
	includeTable map[string]bool
	dbRegex     *regexp.Regexp
	tableRegex  *regexp.Regexp
}

// NewRouteFilter 创建路由过滤器
func NewRouteFilter(includeDB, includeTable []string, dbRegexStr, tableRegexStr string) (*RouteFilter, error) {
	rf := &RouteFilter{
		includeDB:   make(map[string]bool),
		includeTable: make(map[string]bool),
	}

	// 精确列表
	for _, db := range includeDB {
		rf.includeDB[db] = true
	}
	for _, table := range includeTable {
		rf.includeTable[table] = true
	}

	// 正则表达式
	if dbRegexStr != "" {
		re, err := regexp.Compile(dbRegexStr)
		if err != nil {
			return nil, err
		}
		rf.dbRegex = re
	}

	if tableRegexStr != "" {
		re, err := regexp.Compile(tableRegexStr)
		if err != nil {
			return nil, err
		}
		rf.tableRegex = re
	}

	return rf, nil
}

// Match 检查事件是否匹配过滤条件
func (rf *RouteFilter) Match(event *models.Event) bool {
	// 检查数据库
	if !rf.matchDatabase(event.Database) {
		return false
	}

	// 检查表
	if !rf.matchTable(event.Table) {
		return false
	}

	return true
}

// matchDatabase 检查数据库名
func (rf *RouteFilter) matchDatabase(db string) bool {
	// 如果指定了精确列表
	if len(rf.includeDB) > 0 {
		if !rf.includeDB[db] {
			return false
		}
	}

	// 如果指定了正则
	if rf.dbRegex != nil {
		if !rf.dbRegex.MatchString(db) {
			return false
		}
	}

	return true
}

// matchTable 检查表名
func (rf *RouteFilter) matchTable(table string) bool {
	// 如果指定了精确列表
	if len(rf.includeTable) > 0 {
		if !rf.includeTable[table] {
			return false
		}
	}

	// 如果指定了正则
	if rf.tableRegex != nil {
		if !rf.tableRegex.MatchString(table) {
			return false
		}
	}

	return true
}

// GetWorkerID 获取 worker ID（用于保证同一表同一主键的事件路由到固定 worker）
func (rf *RouteFilter) GetWorkerID(table string, primaryKey string, workerCount int) int {
	// 简单实现：使用字符串哈希
	hash := 0
	key := table + ":" + primaryKey
	for _, ch := range key {
		hash = ((hash << 5) - hash) + int(ch)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash % workerCount
}
