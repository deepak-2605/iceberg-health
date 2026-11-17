// Package render turns a metrics.HealthReport into human- or machine-readable
// output. It only reads the report — never computes — keeping analysis and
// presentation cleanly separated.
package render

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/deepak-2605/iceberg-health/internal/metrics"
)

// RenderCLI writes a human-readable health report to w.
func RenderCLI(w io.Writer, r metrics.HealthReport) {
	fmt.Fprintf(w, "Iceberg Table Health — %s\n", r.Table)
	fmt.Fprintf(w, "UUID %s · format v%d · %s\n", r.UUID, r.FormatVersion, r.Location)
	fmt.Fprintf(w, "generated %s\n\n", r.GeneratedAt.Format(time.RFC3339))

	renderSnapshots(w, r.Snapshots)
	renderSchema(w, r.Schema)
	renderSkew(w, r.Skew)
}

func renderSnapshots(w io.Writer, s metrics.SnapshotReport) {
	fmt.Fprintln(w, "SNAPSHOTS")
	fmt.Fprintf(w, "  total %d · bloat %d (keep newest %d) · stale %d (> %s)\n",
		s.Total, s.BloatCount, s.RetentionLimit, s.StaleCount, humanDur(s.MaxAge))
	if s.Total > 0 {
		fmt.Fprintf(w, "  span %s → %s (%d days)\n",
			s.Oldest.Format("2006-01-02"), s.Newest.Format("2006-01-02"), days(s.Span))
	}
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  WHEN\tSNAPSHOT\tFILES\tRECORDS\tAGE\tSTATUS")
	for _, snap := range s.Snapshots {
		status := "ok"
		if snap.Referenced {
			status = "pinned"
		} else if snap.Stale {
			status = "STALE"
		}
		fmt.Fprintf(tw, "  %s\t%d\t%s\t%s\t%dd\t%s\n",
			snap.Timestamp.Format("2006-01-02"), snap.ID,
			count(snap.TotalDataFiles), count(snap.TotalRecords),
			days(snap.Age), status)
	}
	tw.Flush()
	fmt.Fprintln(w)
}

func renderSchema(w io.Writer, sc metrics.SchemaReport) {
	fmt.Fprintln(w, "SCHEMA EVOLUTION")
	fmt.Fprintf(w, "  current schema #%d · %d columns · %d versions in history\n",
		sc.CurrentSchemaID, sc.CurrentColumns, sc.TotalSchemas)
	if len(sc.Diffs) == 0 {
		fmt.Fprintln(w, "  (no changes recorded)")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  TRANSITION\tCHANGE\tCOLUMN\tDETAIL")
	for _, d := range sc.Diffs {
		for _, c := range d.Changes {
			fmt.Fprintf(tw, "  #%d→#%d\t%s\tid %d (%s)\t%s\n",
				d.FromSchemaID, d.ToSchemaID, c.Kind, c.ColumnID, c.Name, changeDetail(c))
		}
	}
	tw.Flush()
	fmt.Fprintln(w)
}

func renderSkew(w io.Writer, sk *metrics.SkewReport) {
	fmt.Fprintln(w, "PARTITION SKEW")
	if sk == nil {
		fmt.Fprintln(w, "  (not computed — needs a live catalog table with manifest access)")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "  partitions %d · total records %d · max/median %.1fx · CV %.2f\n\n",
		sk.PartitionCount, sk.TotalRecords, sk.MaxMedianRatio, sk.CoefficientOfVariation)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  PARTITION\tRECORDS\tFILES\tBYTES")
	for _, p := range sk.Partitions {
		fmt.Fprintf(tw, "  %s\t%s\t%d\t%d\n", p.Key, count(p.RecordCount), p.FileCount, p.SizeBytes)
	}
	tw.Flush()
	fmt.Fprintln(w)
}

func changeDetail(c metrics.SchemaChange) string {
	switch c.Kind {
	case metrics.ColumnAdded:
		return c.NewType
	case metrics.ColumnDropped:
		return "was " + c.OldType
	case metrics.ColumnRenamed:
		return c.OldName + " → " + c.NewName
	case metrics.ColumnRetyped:
		return c.OldType + " → " + c.NewType
	}
	return ""
}

// count renders a count, showing "-" for the -1 "absent" sentinel.
func count(n int64) string {
	if n < 0 {
		return "-"
	}
	return fmt.Sprintf("%d", n)
}

func days(d time.Duration) int { return int(d.Hours() / 24) }

func humanDur(d time.Duration) string {
	if d >= 24*time.Hour {
		return fmt.Sprintf("%dd", days(d))
	}
	return d.String()
}
