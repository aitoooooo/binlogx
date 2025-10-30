package cmd

import (
	"fmt"

	"github.com/aitoooooo/binlogx/pkg/config"
	"github.com/aitoooooo/binlogx/pkg/filter"
	"github.com/aitoooooo/binlogx/pkg/models"
	"github.com/aitoooooo/binlogx/pkg/processor"
	"github.com/aitoooooo/binlogx/pkg/source"
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

		// 处理器
		sqlHandler := &sqlHandler{}

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
}

func (sh *sqlHandler) Handle(event *models.Event) error {
	// 输出 SQL 语句
	if event.SQL != "" {
		fmt.Println(event.SQL + ";")
	}
	return nil
}

func (sh *sqlHandler) Flush() error {
	return nil
}

func generateForwardSQL(event *models.Event) string {
	// TODO: 根据事件类型生成 SQL
	// 这里是简化的实现
	switch event.Action {
	case "INSERT":
		return fmt.Sprintf("-- INSERT event: %s.%s", event.Database, event.Table)
	case "UPDATE":
		return fmt.Sprintf("-- UPDATE event: %s.%s", event.Database, event.Table)
	case "DELETE":
		return fmt.Sprintf("-- DELETE event: %s.%s", event.Database, event.Table)
	default:
		return ""
	}
}
