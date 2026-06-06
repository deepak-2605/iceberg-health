package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/deepak-2605/iceberg-health/internal/metrics"
)

// Tests run with the working directory set to this package, so fixtures live two
// levels up at the repo-root testdata/.
const (
	taxisFixture  = "../../testdata/nyc_taxis.metadata.json"
	eventsFixture = "../../testdata/web_events.metadata.json"
)

// analyzeAll fans out concurrently but must return reports in input order.
func TestAnalyzeAllPreservesOrder(t *testing.T) {
	analyzeConcurrency = 4
	ctx := context.Background()

	reports, errs := analyzeAll(ctx, []string{taxisFixture, eventsFixture},
		metrics.Config{RetentionLimit: 5, MaxAge: 7 * 24 * time.Hour})

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(reports) != 2 {
		t.Fatalf("got %d reports, want 2", len(reports))
	}
	if !strings.Contains(reports[0].Table, "taxis") {
		t.Errorf("reports[0] = %q, want the taxis table first", reports[0].Table)
	}
	if !strings.Contains(reports[1].Table, "web_events") {
		t.Errorf("reports[1] = %q, want web_events second", reports[1].Table)
	}
}

// A single bad source must not sink the good ones.
func TestAnalyzeAllPartialFailure(t *testing.T) {
	analyzeConcurrency = 4
	ctx := context.Background()

	reports, errs := analyzeAll(ctx, []string{taxisFixture, "../../testdata/missing.json"},
		metrics.Config{RetentionLimit: 5})

	if len(reports) != 1 {
		t.Errorf("got %d reports, want 1 (the good one survives)", len(reports))
	}
	if len(errs) != 1 {
		t.Errorf("got %d errors, want 1", len(errs))
	}
}
