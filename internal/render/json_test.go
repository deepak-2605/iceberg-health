package render

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/deepak-2605/iceberg-health/internal/metrics"
)

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderJSON(&buf, sampleReport()); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}

	// It must be valid JSON.
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if m["table"] != "nyc.taxis" {
		t.Errorf(`m["table"] = %v, want "nyc.taxis"`, m["table"])
	}

	// Numbers decode to float64 from arbitrary JSON.
	snap, ok := m["snapshots"].(map[string]any)
	if !ok {
		t.Fatalf("snapshots missing or wrong type: %v", m["snapshots"])
	}
	if snap["bloat_count"] != float64(3) {
		t.Errorf("bloat_count = %v, want 3", snap["bloat_count"])
	}

	// Skew is a nil *SkewReport with omitempty → the key must be absent.
	if _, has := m["skew"]; has {
		t.Error(`"skew" key should be omitted when nil (omitempty)`)
	}
}

func TestRenderJSONMany(t *testing.T) {
	var buf bytes.Buffer
	reports := []metrics.HealthReport{sampleReport(), sampleReport()}
	if err := RenderJSONMany(&buf, reports); err != nil {
		t.Fatalf("RenderJSONMany: %v", err)
	}

	var arr []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &arr); err != nil {
		t.Fatalf("output is not a valid JSON array: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("array length = %d, want 2", len(arr))
	}
	if arr[0]["table"] != "nyc.taxis" {
		t.Errorf("arr[0].table = %v, want nyc.taxis", arr[0]["table"])
	}
}
