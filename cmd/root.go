package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "binlogx",
	Short: "High-performance MySQL binlog processing tool",
	Long: `binlogx is a powerful tool for processing MySQL binlog files.
It supports offline files and online database sources, providing
statistics, parsing, SQL generation, rollback SQL, and multi-format export.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	// 添加全局 flag
	addGlobalFlags(rootCmd)

	// 添加子命令
	rootCmd.AddCommand(statCmd)
	rootCmd.AddCommand(parseCmd)
	rootCmd.AddCommand(sqlCmd)
	rootCmd.AddCommand(rollbackSqlCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(versionCmd)
}

func addGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().String("source", "", "离线 binlog 文件路径")
	cmd.PersistentFlags().String("db-connection", "", "在线 DSN user:pass@tcp(host:port)/dbname?charset=utf8mb4")
	cmd.PersistentFlags().String("start-time", "", "开始时间 YYYY-MM-DD HH:MM:SS")
	cmd.PersistentFlags().String("end-time", "", "结束时间 YYYY-MM-DD HH:MM:SS")
	cmd.PersistentFlags().StringSlice("action", []string{}, "操作类型过滤 (INSERT,UPDATE,DELETE)")
	cmd.PersistentFlags().String("slow-threshold", "1s", "慢方法阈值")
	cmd.PersistentFlags().Int64("event-size-threshold", 0, "事件大小阈值（字节）")
	cmd.PersistentFlags().String("db-regex", "", "分库正则")
	cmd.PersistentFlags().String("table-regex", "", "分表正则")
	cmd.PersistentFlags().StringSlice("include-db", []string{}, "精确库列表")
	cmd.PersistentFlags().StringSlice("include-table", []string{}, "精确表列表")
	cmd.PersistentFlags().Int("workers", 0, "worker 数量，默认 0=CPU 数")
}
