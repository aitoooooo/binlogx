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

var rollbackSqlCmd = &cobra.Command{
	Use:   "rollback-sql",
	Short: "Generate reverse SQL for rollback",
	Long:  "Generate reverse SQL statements to undo binlog events",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 初始化配置
		cfg, err := config.InitConfig(cmd)
		if err != nil {
			return err
		}

		bulk, _ := cmd.Flags().GetBool("bulk")

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
		rf, err := filter.NewRouteFilter(cfg.SchemaTableRegex)
		if err != nil {
			return err
		}

		// 创建命令助手（包含列名缓存和映射功能）
		helper := NewCommandHelper(cfg.DBConnection)

		// 处理器
		rollbackHandler := &rollbackSqlHandler{
			bulk:         bulk,
			buffer:       make([]string, 0),
			sqlGenerator: util.NewSQLGenerator(config.GlobalMonitor),
			helper:       helper,
		}

		// 创建处理器
		proc := processor.NewEventProcessor(ds, rf, cfg.Workers)
		proc.AddHandler(rollbackHandler)

		// 启动处理
		if err := proc.Start(); err != nil {
			return err
		}

		// 等待完成
		return proc.Wait()
	},
}

type rollbackSqlHandler struct {
	bulk         bool
	buffer       []string
	sqlGenerator *util.SQLGenerator
	helper       *CommandHelper
	mu           sync.Mutex
}

func (rsh *rollbackSqlHandler) Handle(event *models.Event) error {
	rsh.mu.Lock()
	defer rsh.mu.Unlock()

	// QUERY 事件不处理
	if event.Action == "QUERY" {
		return nil
	}

	// 映射列名：将 col_N 替换为实际列名
	rsh.helper.MapColumnNames(event)

	// 生成回滚 SQL
	sql := generateRollbackSQL(event, rsh.sqlGenerator)
	if sql == "" {
		return nil
	}

	if rsh.bulk {
		rsh.buffer = append(rsh.buffer, sql)
	} else {
		fmt.Println(sql + ";")
	}

	return nil
}

func (rsh *rollbackSqlHandler) Flush() error {
	rsh.mu.Lock()
	defer rsh.mu.Unlock()

	if rsh.bulk && len(rsh.buffer) > 0 {
		fmt.Printf("-- Bulk rollback: %d statements\n", len(rsh.buffer))
		for _, sql := range rsh.buffer {
			fmt.Println(sql + ";")
		}
	}
	return nil
}

// generateRollbackSQL 生成回滚 SQL
func generateRollbackSQL(event *models.Event, sqlGenerator *util.SQLGenerator) string {
	switch event.Action {
	case "INSERT":
		// INSERT 的回滚是 DELETE
		return sqlGenerator.GenerateDeleteSQL(event)
	case "UPDATE":
		// UPDATE 的回滚是 UPDATE（使用 BeforeValues）
		// 创建临时事件用于生成回滚 SQL
		rollbackEvent := &models.Event{
			Database: event.Database,
			Table:    event.Table,
			Action:   "UPDATE",
			// 将 BeforeValues 和 AfterValues 互换
			BeforeValues: event.AfterValues,
			AfterValues:  event.BeforeValues,
		}
		return sqlGenerator.GenerateUpdateSQL(rollbackEvent)
	case "DELETE":
		// DELETE 的回滚是 INSERT
		return sqlGenerator.GenerateInsertSQL(event)
	default:
		return ""
	}
}

func init() {
	rollbackSqlCmd.Flags().BoolP("bulk", "b", false, "合并为批量 SQL，默认 false")
}
