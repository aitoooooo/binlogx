package cmd

import (
	"fmt"
	"github.com/aitoooooo/binlogx/pkg/config"
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
	config.AddGlobalFlags(rootCmd)

	// 添加子命令
	rootCmd.AddCommand(statCmd)
	rootCmd.AddCommand(parseCmd)
	rootCmd.AddCommand(sqlCmd)
	rootCmd.AddCommand(rollbackSqlCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(versionCmd)
}
