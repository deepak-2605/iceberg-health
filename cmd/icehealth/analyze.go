package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/deepak-2605/iceberg-health/internal/catalog"
	"github.com/deepak-2605/iceberg-health/internal/metrics"
	"github.com/deepak-2605/iceberg-health/internal/render"
)

var (
	analyzeRetention  int
	analyzeMaxAge     time.Duration
	analyzeJSON       bool
	analyzeCatalogURI string
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze <table|metadata.json>",
	Short: "Analyze the health of an Iceberg table",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		source := args[0]
		meta, tbl, err := catalog.Resolve(ctx, source, analyzeCatalogURI)
		if err != nil {
			return err
		}

		report := metrics.BuildHealthReport(meta, source, metrics.Config{
			RetentionLimit: analyzeRetention,
			MaxAge:         analyzeMaxAge,
		})

		// Partition skew needs a live data layer (manifests + FileIO), so attempt
		// it only when Resolve gave us a real table. Failure is non-fatal.
		if tbl != nil {
			if sk, skErr := metrics.AnalyzeSkew(ctx, tbl); skErr == nil {
				report.Skew = sk
			} else {
				fmt.Fprintln(os.Stderr, "note: partition skew unavailable:", skErr)
			}
		}

		if analyzeJSON {
			return render.RenderJSON(os.Stdout, report)
		}
		render.RenderCLI(os.Stdout, report)
		return nil
	},
}

func init() {
	f := analyzeCmd.Flags()
	f.IntVar(&analyzeRetention, "retention", 5, "keep this many newest snapshots before counting bloat")
	f.DurationVar(&analyzeMaxAge, "max-age", 7*24*time.Hour, "snapshots older than this are stale (e.g. 168h, 720h)")
	f.BoolVar(&analyzeJSON, "json", false, "emit JSON instead of a table")
	f.StringVar(&analyzeCatalogURI, "catalog-uri", "http://localhost:8181", "REST catalog URI (used for table identifiers)")
}
