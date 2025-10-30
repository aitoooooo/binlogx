package cmd

import (
	"fmt"
	"sync"

	"github.com/aitoooooo/binlogx/pkg/config"
	"github.com/aitoooooo/binlogx/pkg/filter"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/processor"
	"github.com/aitoooooo/binlogx/pkg/source"
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
		rf, err := filter.NewRouteFilter(cfg.IncludeDB, cfg.IncludeTable, cfg.DBRegex, cfg.TableRegex)
		if err != nil {
			return err
		}

		// 处理器
		rollbackHandler := &rollbackSqlHandler{
			bulk:   bulk,
			buffer: make([]string, 0),
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
	bulk   bool
	buffer []string
	mu     sync.Mutex
}

func (rsh *rollbackSqlHandler) Handle(event *models.Event) error {
	sql := generateRollbackSQL(event)
	if sql == "" {
		return nil
	}

	rsh.mu.Lock()
	defer rsh.mu.Unlock()

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

func generateRollbackSQL(event *models.Event) string {
	// TODO: 生成反向 SQL
	// 这里是简化的实现
	switch event.Action {
	case "INSERT":
		// INSERT 的回滚是 DELETE
		return fmt.Sprintf("-- ROLLBACK DELETE for: %s.%s", event.Database, event.Table)
	case "UPDATE":
		// UPDATE 的回滚是 UPDATE（使用 beforeValues）
		return fmt.Sprintf("-- ROLLBACK UPDATE for: %s.%s", event.Database, event.Table)
	case "DELETE":
		// DELETE 的回滚是 INSERT
		return fmt.Sprintf("-- ROLLBACK INSERT for: %s.%s", event.Database, event.Table)
	default:
		return ""
	}
}

func init() {
	rollbackSqlCmd.Flags().BoolP("bulk", "b", false, "合并为批量 SQL，默认 false")
}
