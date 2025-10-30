package cmd

import (
	"database/sql"
	"fmt"

	"github.com/aitoooooo/binlogx/pkg/cache"
	"github.com/aitoooooo/binlogx/pkg/config"
	"github.com/aitoooooo/binlogx/pkg/models"
)

// CommandHelper 提供命令的公共功能
type CommandHelper struct {
	metaCache *cache.MetaCache
}

// NewCommandHelper 创建命令助手
func NewCommandHelper(dbConnection string) *CommandHelper {
	var metaCache *cache.MetaCache
	if dbConnection != "" {
		if db, err := sql.Open("mysql", dbConnection); err == nil {
			metaCache = cache.NewMetaCache(db, 10000, config.GlobalMonitor)
		}
	}
	return &CommandHelper{
		metaCache: metaCache,
	}
}

// MapColumnNames 将事件中的列占位符映射到实际列名
func (ch *CommandHelper) MapColumnNames(event *models.Event) {
	if ch.metaCache == nil || (event.AfterValues == nil && event.BeforeValues == nil) {
		return
	}

	columnNames := ch.getColumnNameMapping(event.Database, event.Table)
	if columnNames == nil {
		return
	}

	if event.AfterValues != nil {
		event.AfterValues = mapColNamesToValues(event.AfterValues, columnNames)
	}
	if event.BeforeValues != nil {
		event.BeforeValues = mapColNamesToValues(event.BeforeValues, columnNames)
	}
}

// getColumnNameMapping 获取表的列名映射（col_N -> 实际列名）
func (ch *CommandHelper) getColumnNameMapping(database, table string) map[string]string {
	if ch.metaCache == nil {
		return nil
	}

	meta, err := ch.metaCache.GetTableMeta(database, table)
	if err != nil || meta == nil {
		return nil
	}

	columnNames := make(map[string]string)
	for i, col := range meta.Columns {
		columnNames[fmt.Sprintf("col_%d", i)] = col.Name
	}
	return columnNames
}

// mapColNamesToValues 将 col_N 映射到实际列名
func mapColNamesToValues(values map[string]interface{}, columnNames map[string]string) map[string]interface{} {
	if values == nil || columnNames == nil {
		return values
	}

	result := make(map[string]interface{})
	for key, val := range values {
		if realName, ok := columnNames[key]; ok {
			result[realName] = val
		} else {
			result[key] = val
		}
	}
	return result
}
