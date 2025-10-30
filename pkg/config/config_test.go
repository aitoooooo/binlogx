package config

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestInitConfig(t *testing.T) {
	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {},
	}
	cmd.Flags().String("source", "/tmp/test.binlog", "source")
	cmd.Flags().String("db-connection", "", "db-connection")
	cmd.Flags().StringSlice("action", []string{}, "action")
	cmd.Flags().String("slow-threshold", "2s", "slow-threshold")
	cmd.Flags().Int64("event-size-threshold", 0, "event-size-threshold")
	cmd.Flags().String("db-regex", "", "db-regex")
	cmd.Flags().String("table-regex", "", "table-regex")
	cmd.Flags().StringSlice("include-db", []string{}, "include-db")
	cmd.Flags().StringSlice("include-table", []string{}, "include-table")
	cmd.Flags().Int("workers", 4, "workers")

	cfg, err := InitConfig(cmd)
	if err != nil {
		t.Fatalf("InitConfig failed: %v", err)
	}

	if cfg.Source != "/tmp/test.binlog" {
		t.Errorf("Expected source=/tmp/test.binlog, got %s", cfg.Source)
	}

	if cfg.SlowThreshold != 2*time.Second {
		t.Errorf("Expected slow-threshold=2s, got %v", cfg.SlowThreshold)
	}

	if cfg.Workers != 4 {
		t.Errorf("Expected workers=4, got %d", cfg.Workers)
	}
}

func TestInitConfigMissingSource(t *testing.T) {
	cmd := &cobra.Command{}
	AddGlobalFlags(cmd)

	_, err := InitConfig(cmd)
	if err == nil {
		t.Error("Expected error when source and db-connection are missing")
	}
}
