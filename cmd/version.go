package cmd

import (
	"fmt"

	"github.com/aitoooooo/binlogx/pkg/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("binlogx version " + version.Version)
		fmt.Println("Build time: " + version.BuildTime)
		fmt.Println("Git commit: " + version.GitCommit)
		fmt.Println("Git branch: " + version.GitBranch)
	},
}
