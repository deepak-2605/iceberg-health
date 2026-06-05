package metrics

import (
	"time"

	"github.com/apache/iceberg-go/table"
)

// Config bundles the tunable knobs for a full health analysis.
type Config struct {
	RetentionLimit int           // keep this many newest snapshots before counting bloat
	MaxAge         time.Duration // snapshots older than this are stale
	Now            time.Time     // analysis reference time (defaults to time.Now())
}

// HealthReport is the single source of truth that both the CLI and the web
// server render. It is built purely from table.Metadata (plus optional skew,
// which needs a live data layer).
type HealthReport struct {
	Table         string         `json:"table"`
	UUID          string         `json:"uuid"`
	Location      string         `json:"location"`
	FormatVersion int            `json:"format_version"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Snapshots     SnapshotReport `json:"snapshots"`
	Schema        SchemaReport   `json:"schema"`
	Skew          *SkewReport    `json:"skew,omitempty"` // nil unless computed from a live table
}

// BuildHealthReport assembles the metadata-only metrics (snapshots + schema).
// Partition skew is attached separately via AnalyzeSkew when a live table is
// available, since it requires reading manifests.
func BuildHealthReport(meta table.Metadata, source string, cfg Config) HealthReport {
	now := cfg.Now
	if now.IsZero() {
		now = time.Now()
	}

	return HealthReport{
		Table:         source,
		UUID:          meta.TableUUID().String(),
		Location:      meta.Location(),
		FormatVersion: meta.Version(),
		GeneratedAt:   now,
		Snapshots: AnalyzeSnapshots(meta, SnapshotConfig{
			RetentionLimit: cfg.RetentionLimit,
			MaxAge:         cfg.MaxAge,
			Now:            now,
		}),
		Schema: AnalyzeSchema(meta),
	}
}
