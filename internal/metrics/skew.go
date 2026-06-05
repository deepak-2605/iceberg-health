package metrics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/apache/iceberg-go/table"
)

// PartitionStat is the aggregated footprint of a single partition.
type PartitionStat struct {
	Key         string `json:"key"`
	RecordCount int64  `json:"record_count"`
	FileCount   int    `json:"file_count"`
	SizeBytes   int64  `json:"size_bytes"`
}

// SkewReport describes how unevenly data is spread across partitions.
type SkewReport struct {
	PartitionCount         int             `json:"partition_count"`
	TotalRecords           int64           `json:"total_records"`
	MaxMedianRatio         float64         `json:"max_median_ratio"`         // largest partition / median partition
	CoefficientOfVariation float64         `json:"coefficient_of_variation"` // stddev / mean
	Largest                *PartitionStat  `json:"largest,omitempty"`
	Partitions             []PartitionStat `json:"partitions"` // sorted by record count, descending
}

// ComputeSkew is the PURE skew calculation: given per-partition stats, derive
// the skew indicators. Split out from manifest I/O so it is unit-testable with
// synthetic input.
func ComputeSkew(stats []PartitionStat) SkewReport {
	report := SkewReport{PartitionCount: len(stats)}
	if len(stats) == 0 {
		return report
	}

	// Work on a copy; sort largest-first.
	s := append([]PartitionStat(nil), stats...)
	sort.Slice(s, func(i, j int) bool { return s[i].RecordCount > s[j].RecordCount })
	report.Partitions = s
	report.Largest = &s[0]

	counts := make([]float64, len(s))
	var total int64
	for i := range s {
		counts[i] = float64(s[i].RecordCount)
		total += s[i].RecordCount
	}
	report.TotalRecords = total

	mean := float64(total) / float64(len(s))
	med := median(counts) // counts is largest-first; median() sorts its own copy

	var variance float64
	for _, c := range counts {
		d := c - mean
		variance += d * d
	}
	variance /= float64(len(s))
	std := math.Sqrt(variance)

	if med > 0 {
		report.MaxMedianRatio = counts[0] / med // counts[0] is the max (sorted desc)
	}
	if mean > 0 {
		report.CoefficientOfVariation = std / mean
	}
	return report
}

// AnalyzeSkew walks the current snapshot's manifests of a LIVE table, aggregates
// record counts per partition, and computes skew. Requires a real table with an
// accessible data layer (manifests + FileIO) — it cannot run against a
// metadata-only file load, which is why it lives behind the catalog path.
func AnalyzeSkew(ctx context.Context, tbl *table.Table) (*SkewReport, error) {
	fs, err := tbl.FS(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening table filesystem: %w", err)
	}

	agg := map[string]*PartitionStat{}
	for mf, err := range tbl.AllManifests(ctx) {
		if err != nil {
			return nil, fmt.Errorf("listing manifests: %w", err)
		}
		entries, err := mf.FetchEntries(fs, true) // discardDeleted=true
		if err != nil {
			return nil, fmt.Errorf("reading manifest %s: %w", mf.FilePath(), err)
		}
		for _, e := range entries {
			df := e.DataFile()
			key := partitionKey(df.Partition())
			ps := agg[key]
			if ps == nil {
				ps = &PartitionStat{Key: key}
				agg[key] = ps
			}
			ps.RecordCount += df.Count()
			ps.FileCount++
			ps.SizeBytes += df.FileSizeBytes()
		}
	}

	stats := make([]PartitionStat, 0, len(agg))
	for _, ps := range agg {
		stats = append(stats, *ps)
	}

	r := ComputeSkew(stats)
	return &r, nil
}

// partitionKey builds a stable, readable key from a data file's partition map
// (partition field-id -> value). The exact label is cosmetic; what matters is
// that identical partition tuples produce the same key.
func partitionKey(p map[int]any) string {
	if len(p) == 0 {
		return "<unpartitioned>"
	}
	ids := make([]int, 0, len(p))
	for id := range p {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d=%v", id, p[id]))
	}
	return strings.Join(parts, "/")
}

func median(xs []float64) float64 {
	n := len(xs)
	if n == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}
