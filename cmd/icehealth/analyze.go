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
	analyzeRetention   int
	analyzeMaxAge      time.Duration
	analyzeJSON        bool
	analyzeCatalogURI  string
	analyzeConcurrency int
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze <table|metadata.json> [more tables...]",
	Short: "Analyze the health of one or more Iceberg tables",
	Args:  cobra.MinimumNArgs(1), // one or more tables
	RunE:  runAnalyze,
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := metrics.Config{
		RetentionLimit: analyzeRetention,
		MaxAge:         analyzeMaxAge,
	}

	// Analyze every table (concurrently when there's more than one).
	reports, errs := analyzeAll(ctx, args, cfg)

	// Render the successful reports (input order is preserved by analyzeAll).
	if analyzeJSON {
		if len(reports) == 1 {
			if err := render.RenderJSON(os.Stdout, reports[0]); err != nil {
				return err
			}
		} else if err := render.RenderJSONMany(os.Stdout, reports); err != nil {
			return err
		}
	} else {
		for _, r := range reports {
			render.RenderCLI(os.Stdout, r)
		}
	}

	// A failure on one table doesn't abort the others; report them at the end.
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "error:", e)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d of %d table(s) failed", len(errs), len(args))
	}
	return nil
}

// analyzeAll fans out per-table analysis across goroutines, bounded by
// --concurrency, and collects the results back in the original input order.
//
// The pattern: a buffered "sem" channel acts as a counting semaphore that caps
// how many goroutines run at once; a buffered "out" channel collects each
// result. We then drain exactly len(sources) results and slot each one back into
// place by its index, so output order matches input order regardless of which
// table finished first.
func analyzeAll(ctx context.Context, sources []string, cfg metrics.Config) ([]metrics.HealthReport, []error) {
	type result struct {
		index  int
		report metrics.HealthReport
		err    error
	}

	limit := analyzeConcurrency
	if limit < 1 {
		limit = 1
	}
	sem := make(chan struct{}, limit)      // capacity = max goroutines in flight
	out := make(chan result, len(sources)) // each goroutine sends exactly one result

	for i, src := range sources {
		sem <- struct{}{} // acquire a slot; blocks here once 'limit' are running
		// i and src are passed as arguments so each goroutine gets its own copy.
		// (Go 1.22+ also makes loop variables per-iteration, but passing them is
		// the clearest, version-proof habit.)
		go func(i int, src string) {
			defer func() { <-sem }() // release the slot when this goroutine returns
			rep, err := analyzeOne(ctx, src, cfg)
			out <- result{index: i, report: rep, err: err}
		}(i, src)
	}

	// Drain all results. Reorder by index → input order.
	results := make([]result, len(sources))
	for range sources {
		r := <-out
		results[r.index] = r
	}

	reports := make([]metrics.HealthReport, 0, len(sources))
	var errs []error
	for i, r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", sources[i], r.err))
			continue
		}
		reports = append(reports, r.report)
	}
	return reports, errs
}

// analyzeOne runs the full pipeline for a single table: load → build report →
// (live tables only) attach partition skew.
func analyzeOne(ctx context.Context, source string, cfg metrics.Config) (metrics.HealthReport, error) {
	meta, tbl, err := catalog.Resolve(ctx, source, analyzeCatalogURI)
	if err != nil {
		return metrics.HealthReport{}, err
	}

	report := metrics.BuildHealthReport(meta, source, cfg)
	if tbl != nil {
		if sk, skErr := metrics.AnalyzeSkew(ctx, tbl); skErr == nil {
			report.Skew = sk
		} else {
			fmt.Fprintf(os.Stderr, "note: partition skew unavailable for %s: %v\n", source, skErr)
		}
	}
	return report, nil
}

func init() {
	f := analyzeCmd.Flags()
	f.IntVar(&analyzeRetention, "retention", 5, "keep this many newest snapshots before counting bloat")
	f.DurationVar(&analyzeMaxAge, "max-age", 7*24*time.Hour, "snapshots older than this are stale (e.g. 168h, 720h)")
	f.BoolVar(&analyzeJSON, "json", false, "emit JSON instead of a table")
	f.StringVar(&analyzeCatalogURI, "catalog-uri", "http://localhost:8181", "REST catalog URI (used for table identifiers)")
	f.IntVar(&analyzeConcurrency, "concurrency", 4, "max tables to analyze in parallel")
}
