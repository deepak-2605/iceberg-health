package main

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/deepak-2605/iceberg-health/internal/server"
)

var (
	serveHost       string
	servePort       int
	serveCatalogURI string
	serveRetention  int
	serveMaxAge     time.Duration
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the web dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		return server.Run(server.Options{
			Host:           serveHost,
			Port:           servePort,
			CatalogURI:     serveCatalogURI,
			RetentionLimit: serveRetention,
			MaxAge:         serveMaxAge,
		})
	},
}

func init() {
	f := serveCmd.Flags()
	f.StringVar(&serveHost, "host", "127.0.0.1", "interface to bind (use 0.0.0.0 to expose on the network — not recommended)")
	f.IntVar(&servePort, "port", 8080, "HTTP port to listen on")
	f.StringVar(&serveCatalogURI, "catalog-uri", "http://localhost:8181", "REST catalog URI")
	f.IntVar(&serveRetention, "retention", 5, "keep this many newest snapshots before counting bloat")
	f.DurationVar(&serveMaxAge, "max-age", 7*24*time.Hour, "snapshots older than this are stale")
}
