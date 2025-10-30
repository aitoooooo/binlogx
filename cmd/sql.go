package cmd

import (
	"fmt"
	"sync"

	"github.com/aitoooooo/binlogx/pkg/config"
	"github.com/aitoooooo/binlogx/pkg/filter"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/processor"
	"github.com/aitoooooo/binlogx/pkg/source"
	"github.com/aitoooooo/binlogx/pkg/util"
	"github.com/spf13/cobra"
)

var sqlCmd = &cobra.Command{
	Use:   "sql",
	Short: "Generate executable SQL statements",
	Long:  "Parse binlog and output forward SQL statements",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 初始化配置
		cfg, err := config.InitConfig(cmd)
		if err != nil {
			return err
		}

		// 创建数据源
		var ds source.DataSource
		if cfg.Source != "" {
			ds = source.NewFileSource(cfg.Source)
		} else {
			ds = source.NewMySQLSource(cfg.DBConnection)
		}

		// 打开数据源
		if err := ds.Open(cmd.Context()); err != nil {
			return err
		}
		defer ds.Close()

		// 创建过滤器
		rf, err := filter.NewRouteFilter(cfg.IncludeDB, cfg.IncludeTable, cfg.DBRegex, cfg.TableRegex)
		if err != nil {
			return err
		}

		// 创建命令助手（包含列名缓存和映射功能）
		helper := NewCommandHelper(cfg.DBConnection)

		// 处理器
		sqlHandler := &sqlHandler{
			sqlGenerator: util.NewSQLGenerator(),
			helper:       helper,
		}

		// 创建处理器
		proc := processor.NewEventProcessor(ds, rf, cfg.Workers)
		proc.AddHandler(sqlHandler)

		// 启动处理
		if err := proc.Start(); err != nil {
			return err
		}

		// 等待完成
		return proc.Wait()
	},
}

type sqlHandler struct {
	sqlGenerator *util.SQLGenerator
	helper       *CommandHelper
	mu           sync.Mutex
	count        int
}

func (sh *sqlHandler) Handle(event *models.Event) error {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	// QUERY 事件不处理
	if event.Action == "QUERY" {
		return nil
	}

	// 重要：先映射列名，再生成 SQL
	// 这样生成的 SQL 中列名是真实的，而不是 col_N
	sh.helper.MapColumnNames(event)

	// 生成 SQL（此时列名已经映射为实际列名）
	var sql string
	switch event.Action {
	case "INSERT":
		sql = sh.sqlGenerator.GenerateInsertSQL(event)
	case "UPDATE":
		sql = sh.sqlGenerator.GenerateUpdateSQL(event)
	case "DELETE":
		sql = sh.sqlGenerator.GenerateDeleteSQL(event)
	default:
		return nil
	}

	if sql != "" {
		// 输出注释标记事件信息
		fmt.Printf("-- %s at %s (LogPos: %d)\n",
			event.Action, event.Timestamp.Format("2006-01-02 15:04:05"), event.LogPos)
		fmt.Printf("-- Database: %s, Table: %s\n", event.Database, event.Table)
		fmt.Println(sql + ";")
		sh.count++
	}
	return nil
}

func (sh *sqlHandler) Flush() error {
	return nil
}
