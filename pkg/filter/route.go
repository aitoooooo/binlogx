package filter

import (
	"fmt"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/util"
)

// RouteFilter 分库表正则路由过滤器
type RouteFilter struct {
	rangeMatcher []*util.RangeMatcher
}

// NewRouteFilter 创建路由过滤器
func NewRouteFilter(schemaTableRegexStr []string) (*RouteFilter, error) {
	rf := &RouteFilter{}

	if len(schemaTableRegexStr) != 0 {
		for _, s := range schemaTableRegexStr {
			re, err := util.NewRangeMatcher(s)
			if err != nil {
				return nil, err
			}

			rf.rangeMatcher = append(rf.rangeMatcher, re)
		}
	}
	return rf, nil
}

// Match 检查事件是否匹配过滤条件
func (rf *RouteFilter) Match(event *models.Event) bool {
	// 检查数据库
	if !rf.matchDatabase(event.Database, event.Table) {
		return false
	}

	return true
}

// matchDatabase 检查数据库名
func (rf *RouteFilter) matchDatabase(schema, table string) bool {
	if len(rf.rangeMatcher) > 0 {
		input := fmt.Sprintf("%s.%s", schema, table)
		for _, m := range rf.rangeMatcher {
			if m.Match(input) {
				// log.Printf("matched %s", input)
				return true
			}
		}
		return false
	}

	// 未指定不需要检查
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
