package metrics

import (
	"os"
	"testing"
	"time"

	"github.com/apache/iceberg-go/table"
)

// loadTestMeta parses the shared sample fixture (3 snapshots, 2 schemas).
func loadTestMeta(t *testing.T) table.Metadata {
	t.Helper()
	data, err := os.ReadFile("testdata/sample.metadata.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	meta, err := table.ParseMetadataBytes(data)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return meta
}

func TestAnalyzeSnapshots(t *testing.T) {
	meta := loadTestMeta(t)
	// Fixed reference time = 2024-01-16, one day after the newest snapshot. With
	// an injected Now the test is fully deterministic.
	now := time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		retention int
		maxAge    time.Duration
		wantBloat int
		wantStale int
	}{
		// snapshots at 2024-01-01, -01-10, -01-15; now = 2024-01-16.
		{"keep2_7d", 2, 7 * 24 * time.Hour, 1, 1},   // bloat: oldest; stale: only 2024-01-01
		{"keep5_7d", 5, 7 * 24 * time.Hour, 0, 1},   // nothing beyond retention
		{"keep2_30d", 2, 30 * 24 * time.Hour, 1, 0}, // none older than 30d
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AnalyzeSnapshots(meta, SnapshotConfig{
				RetentionLimit: tc.retention,
				MaxAge:         tc.maxAge,
				Now:            now,
			})

			if got.Total != 3 {
				t.Errorf("Total = %d, want 3", got.Total)
			}
			if got.BloatCount != tc.wantBloat {
				t.Errorf("BloatCount = %d, want %d", got.BloatCount, tc.wantBloat)
			}
			if got.StaleCount != tc.wantStale {
				t.Errorf("StaleCount = %d, want %d", got.StaleCount, tc.wantStale)
			}
			// Snapshots must be newest-first.
			if got.Snapshots[0].ID != 3 {
				t.Errorf("first snapshot ID = %d, want 3 (newest)", got.Snapshots[0].ID)
			}
			// Summary parsing: newest snapshot reported 4 files / 400 records.
			if got.Snapshots[0].TotalDataFiles != 4 || got.Snapshots[0].TotalRecords != 400 {
				t.Errorf("newest summary = files %d records %d, want 4/400",
					got.Snapshots[0].TotalDataFiles, got.Snapshots[0].TotalRecords)
			}
		})
	}
}

// The current snapshot (and anything a branch/tag points at) can't be expired, so
// it must never be counted as stale, no matter how old it is.
func TestAnalyzeSnapshots_CurrentSnapshotNeverStale(t *testing.T) {
	meta := loadTestMeta(t)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) // all 3 snapshots are ~1y old

	got := AnalyzeSnapshots(meta, SnapshotConfig{MaxAge: 7 * 24 * time.Hour, Now: now})

	if got.StaleCount != 2 {
		t.Errorf("StaleCount = %d, want 2 (current snapshot excluded)", got.StaleCount)
	}
	for _, s := range got.Snapshots {
		isCurrent := s.ID == 3
		if s.Referenced != isCurrent {
			t.Errorf("snapshot %d Referenced = %v, want %v", s.ID, s.Referenced, isCurrent)
		}
		if s.Stale == isCurrent {
			t.Errorf("snapshot %d Stale = %v, want %v", s.ID, s.Stale, !isCurrent)
		}
	}
}
