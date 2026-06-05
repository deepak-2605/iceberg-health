package render

import (
	"encoding/json"
	"io"

	"github.com/deepak-2605/iceberg-health/internal/metrics"
)

// RenderJSON writes the report as indented JSON. This is exactly what the web
// server's /api/report endpoint serves, so the CLI's --json output and the API
// payload are identical by construction.
func RenderJSON(w io.Writer, r metrics.HealthReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
