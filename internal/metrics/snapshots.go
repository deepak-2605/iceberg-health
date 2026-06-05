// Package metrics computes Iceberg table-health metrics from table metadata.
//
// Everything here is PURE: functions take metadata + config and return report
// structs. No printing, no I/O, no globals. The CLI and the web server both
// consume these same structs — that's what keeps analysis and rendering apart.
package metrics

import (
	"sort"
	"strconv"
	"time"

	"github.com/apache/iceberg-go/table"
)

// SnapshotConfig tunes the snapshot analysis.
//
// Now is injected rather than read from time.Now() inside the function. That's
// the idiomatic Go way to make time-dependent logic testable: a test passes a
// fixed Now and gets deterministic ages/staleness. If left zero, we fall back to
// time.Now() so real callers don't have to set it.
type SnapshotConfig struct {
	RetentionLimit int           // keep this many newest snapshots; the rest count as "bloat"
	MaxAge         time.Duration // snapshots older than this are "stale"
	Now            time.Time     // reference time for age/staleness (defaults to time.Now())
}

// SnapshotInfo is a single snapshot enriched with parsed and derived fields.
// A -1 in a count field means the snapshot's summary didn't report that number.
type SnapshotInfo struct {
	ID             int64         `json:"id"`
	ParentID       *int64        `json:"parent_id,omitempty"` // nil for the first snapshot
	SequenceNumber int64         `json:"sequence_number"`
	Timestamp      time.Time     `json:"timestamp"`
	Operation      string        `json:"operation,omitempty"`
	TotalDataFiles int64         `json:"total_data_files"`
	TotalRecords   int64         `json:"total_records"`
	Age            time.Duration `json:"age_ns"`
	Stale          bool          `json:"stale"`
}

// SnapshotReport is the result of analyzing a table's snapshots.
type SnapshotReport struct {
	Total          int            `json:"total"`
	Oldest         time.Time      `json:"oldest"`
	Newest         time.Time      `json:"newest"`
	Span           time.Duration  `json:"span_ns"`
	RetentionLimit int            `json:"retention_limit"`
	BloatCount     int            `json:"bloat_count"` // snapshots beyond the retention limit
	MaxAge         time.Duration  `json:"max_age_ns"`
	StaleCount     int            `json:"stale_count"` // snapshots older than MaxAge
	Snapshots      []SnapshotInfo `json:"snapshots"`   // newest first
}

// AnalyzeSnapshots computes snapshot-health metrics from table metadata.
func AnalyzeSnapshots(meta table.Metadata, cfg SnapshotConfig) SnapshotReport {
	if cfg.Now.IsZero() {
		cfg.Now = time.Now()
	}

	raw := meta.Snapshots() // []table.Snapshot, in metadata order

	// Preallocate: we know exactly how many we'll append, so size the backing
	// array once instead of letting append grow it repeatedly.
	infos := make([]SnapshotInfo, 0, len(raw))
	for i := range raw {
		s := raw[i]
		ts := time.UnixMilli(s.TimestampMs)

		info := SnapshotInfo{
			ID:             s.SnapshotID,
			ParentID:       s.ParentSnapshotID,
			SequenceNumber: s.SequenceNumber,
			Timestamp:      ts,
			TotalDataFiles: -1,
			TotalRecords:   -1,
			Age:            cfg.Now.Sub(ts),
		}
		if s.Summary != nil {
			info.Operation = string(s.Summary.Operation)
			info.TotalDataFiles = summaryInt(s.Summary, "total-data-files")
			info.TotalRecords = summaryInt(s.Summary, "total-records")
		}
		info.Stale = cfg.MaxAge > 0 && info.Age > cfg.MaxAge

		infos = append(infos, info)
	}

	// Newest first. sort.Slice takes a "less" closure — return true when i should
	// sort before j; here, when i's timestamp is later than j's.
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Timestamp.After(infos[j].Timestamp)
	})

	report := SnapshotReport{
		Total:          len(infos),
		RetentionLimit: cfg.RetentionLimit,
		MaxAge:         cfg.MaxAge,
		Snapshots:      infos,
	}

	if len(infos) > 0 {
		report.Newest = infos[0].Timestamp
		report.Oldest = infos[len(infos)-1].Timestamp
		report.Span = report.Newest.Sub(report.Oldest)
	}

	// Bloat: keep RetentionLimit newest, everything past that is excess metadata.
	if cfg.RetentionLimit > 0 && len(infos) > cfg.RetentionLimit {
		report.BloatCount = len(infos) - cfg.RetentionLimit
	}

	// Stale: count snapshots older than MaxAge.
	for i := range infos {
		if infos[i].Stale {
			report.StaleCount++
		}
	}

	return report
}

// summaryInt pulls an integer-valued key out of a snapshot summary's string
// properties (Iceberg stores all summary values as strings), returning -1 if the
// key is missing or doesn't parse.
func summaryInt(s *table.Summary, key string) int64 {
	if s == nil || s.Properties == nil {
		return -1
	}
	// comma-ok map lookup: ok is false when the key isn't present.
	v, ok := s.Properties[key]
	if !ok {
		return -1
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return -1
	}
	return n
}
