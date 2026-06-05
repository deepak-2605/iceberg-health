package metrics

import (
	"math"
	"testing"
)

func TestComputeSkew(t *testing.T) {
	cases := []struct {
		name           string
		stats          []PartitionStat
		wantPartitions int
		wantTotal      int64
		wantLargestKey string
		wantRatio      float64 // max / median
	}{
		{
			name:           "empty",
			stats:          nil,
			wantPartitions: 0,
		},
		{
			name: "uniform",
			stats: []PartitionStat{
				{Key: "a", RecordCount: 100},
				{Key: "b", RecordCount: 100},
				{Key: "c", RecordCount: 100},
			},
			wantPartitions: 3,
			wantTotal:      300,
			wantLargestKey: "a",
			wantRatio:      1.0,
		},
		{
			name: "skewed",
			stats: []PartitionStat{
				{Key: "small1", RecordCount: 100},
				{Key: "huge", RecordCount: 1000},
				{Key: "small2", RecordCount: 100},
			},
			wantPartitions: 3,
			wantTotal:      1200,
			wantLargestKey: "huge",
			wantRatio:      10.0, // 1000 / median(100)
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeSkew(tc.stats)

			if got.PartitionCount != tc.wantPartitions {
				t.Errorf("PartitionCount = %d, want %d", got.PartitionCount, tc.wantPartitions)
			}
			if got.TotalRecords != tc.wantTotal {
				t.Errorf("TotalRecords = %d, want %d", got.TotalRecords, tc.wantTotal)
			}
			if tc.wantPartitions == 0 {
				if got.Largest != nil {
					t.Errorf("Largest = %+v, want nil for empty input", got.Largest)
				}
				return
			}
			if got.Largest == nil || got.Largest.Key != tc.wantLargestKey {
				t.Errorf("Largest = %+v, want key %q", got.Largest, tc.wantLargestKey)
			}
			if math.Abs(got.MaxMedianRatio-tc.wantRatio) > 1e-9 {
				t.Errorf("MaxMedianRatio = %v, want %v", got.MaxMedianRatio, tc.wantRatio)
			}
			// Partitions must come out sorted largest-first.
			for i := 1; i < len(got.Partitions); i++ {
				if got.Partitions[i-1].RecordCount < got.Partitions[i].RecordCount {
					t.Errorf("partitions not sorted desc at %d", i)
				}
			}
		})
	}
}
