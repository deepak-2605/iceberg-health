package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/deepak-2605/iceberg-health/internal/metrics"
)

// sampleReport is a small, fully-populated report shared by the render tests.
func sampleReport() metrics.HealthReport {
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	return metrics.HealthReport{
		Table:         "nyc.taxis",
		UUID:          "uuid-1",
		Location:      "s3://warehouse/nyc/taxis",
		FormatVersion: 2,
		GeneratedAt:   now,
		Snapshots: metrics.SnapshotReport{
			Total: 8, BloatCount: 3, RetentionLimit: 5, StaleCount: 7,
			MaxAge: 7 * 24 * time.Hour,
			Oldest: now.Add(-100 * 24 * time.Hour), Newest: now,
			Snapshots: []metrics.SnapshotInfo{
				{ID: 2, Timestamp: now, TotalDataFiles: 28, TotalRecords: 18000, Stale: false},
				{ID: 1, Timestamp: now.Add(-100 * 24 * time.Hour), TotalDataFiles: 2, TotalRecords: 1000, Stale: true},
			},
		},
		Schema: metrics.SchemaReport{CurrentSchemaID: 3, CurrentColumns: 6, TotalSchemas: 4},
		// Skew left nil (the common metadata-only case).
	}
}

func TestRenderCLI(t *testing.T) {
	var buf bytes.Buffer // a bytes.Buffer is an io.Writer — same interface as os.Stdout
	RenderCLI(&buf, sampleReport())
	out := buf.String()

	for _, want := range []string{
		"nyc.taxis", "SNAPSHOTS", "bloat 3", "stale 7",
		"SCHEMA EVOLUTION", "STALE", "PARTITION SKEW", "not computed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("CLI output missing %q\n--- output ---\n%s", want, out)
		}
	}
}
