package main

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "icehealth",
	Short: "Health metrics for Apache Iceberg tables",
	Long: `icehealth analyzes the health of Apache Iceberg tables — snapshot bloat,
stale snapshots, schema-evolution history, and partition skew — from a CLI or a
small web dashboard.

A "table" argument is either a catalog identifier (e.g. nyc.taxis) or a path to
a local metadata.json file.`,
	// Don't dump usage on runtime errors; only on actual flag/arg misuse.
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(analyzeCmd, serveCmd)
}
