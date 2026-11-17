// Package server hosts the icehealth web dashboard: a JSON API plus one static
// HTML page. Both the API and the CLI render the same metrics.HealthReport.
package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/deepak-2605/iceberg-health/internal/catalog"
	"github.com/deepak-2605/iceberg-health/internal/metrics"
)

// staticFS embeds the dashboard's HTML so the binary is fully self-contained —
// no files to ship alongside it.
//
//go:embed static
var staticFS embed.FS

// Options configures the dashboard server.
type Options struct {
	Host           string // interface to bind; "" means 127.0.0.1
	Port           int
	CatalogURI     string
	RetentionLimit int
	MaxAge         time.Duration
}

// Run starts the dashboard and blocks until the server exits.
func Run(opts Options) error {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("preparing static assets: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/report", reportHandler(opts))
	mux.Handle("/", http.FileServer(http.FS(sub)))

	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(opts.Port))
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		log.Printf("WARNING: binding to %s exposes local file reads via /api/report?table=<path> to the network", host)
	}
	log.Printf("icehealth dashboard listening on http://%s", addr)
	return http.ListenAndServe(addr, mux)
}

// reportHandler serves GET /api/report?table=<identifier|path> as JSON.
//
// Note: this is a local dev dashboard — the table parameter may be a local file
// path, so Run binds to 127.0.0.1 unless --host says otherwise.
func reportHandler(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("table")
		if source == "" {
			http.Error(w, `missing required "table" query parameter`, http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		meta, tbl, err := catalog.Resolve(ctx, source, opts.CatalogURI)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		report := metrics.BuildHealthReport(meta, source, metrics.Config{
			RetentionLimit: opts.RetentionLimit,
			MaxAge:         opts.MaxAge,
		})
		if tbl != nil {
			if sk, skErr := metrics.AnalyzeSkew(ctx, tbl); skErr == nil {
				report.Skew = sk
			}
		}

		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			log.Printf("encoding report: %v", err)
		}
	}
}
