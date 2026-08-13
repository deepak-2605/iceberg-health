//go:build integration

// This file validates the REST-catalog CLIENT path — catalog.Connect and
// catalog.LoadTable/Resolve — against a REAL HTTP server speaking the Iceberg
// REST Catalog wire protocol, with zero external dependencies (no Docker, no
// Java, no network beyond localhost). It uses net/http/httptest to stand up a
// minimal but spec-compliant REST catalog: just the two endpoints our client
// actually calls, GET /v1/config and GET /v1/namespaces/{ns}/tables/{table},
// per the documented Iceberg REST OpenAPI contract.
//
// It is NOT the official apache/iceberg-rest-fixture — it's a lightweight
// stand-in server built for this test. But our production client code (the
// same rest.NewCatalog / LoadTable calls Connect/LoadTable make) talks to it
// over real HTTP exactly as it would talk to a real catalog, which is what
// proves the wire-protocol path end-to-end.
//
// Run with: go test -tags=integration ./internal/catalog/... -run TestRESTCatalog -v
package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// configResponse and loadTableResponse mirror the (unexported) response shapes
// the rest.Catalog client expects, per the Iceberg REST OpenAPI spec.
type configResponse struct {
	Defaults  map[string]string `json:"defaults"`
	Overrides map[string]string `json:"overrides"`
}

type loadTableResponse struct {
	MetadataLocation string          `json:"metadata-location"`
	Metadata         json.RawMessage `json:"metadata"`
}

func TestRESTCatalog_LoadTable_RealHTTP(t *testing.T) {
	metaBytes, err := os.ReadFile("../../testdata/nyc_taxis.metadata.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(configResponse{
			Defaults:  map[string]string{},
			Overrides: map[string]string{},
		})
	})
	mux.HandleFunc("/v1/namespaces/nyc/tables/taxis", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(loadTableResponse{
			MetadataLocation: "file:///fake/warehouse/nyc/taxis/metadata/v1.metadata.json",
			Metadata:         metaBytes,
		})
	})
	mux.HandleFunc("/v1/namespaces/nyc/tables/does-not-exist", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"no such table"}}`, http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()

	// This is our REAL, unmodified production code path: Connect dials the
	// server's /v1/config over HTTP (exactly as it would a live catalog), and
	// LoadTable fetches the table over HTTP too.
	cat, err := Connect(ctx, srv.URL)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	tbl, err := LoadTable(ctx, cat, "nyc.taxis")
	if err != nil {
		t.Fatalf("LoadTable: %v", err)
	}

	meta := tbl.Metadata()
	if got, want := meta.TableUUID().String(), "7a3e1c44-9b2d-4f6a-bc11-1a2b3c4d5e6f"; got != want {
		t.Errorf("TableUUID = %s, want %s", got, want)
	}
	if got, want := len(meta.Snapshots()), 8; got != want {
		t.Errorf("len(Snapshots) = %d, want %d", got, want)
	}
	if got, want := len(meta.Schemas()), 4; got != want {
		t.Errorf("len(Schemas) = %d, want %d", got, want)
	}

	// Resolve() must take the same live-catalog branch (non-nil *table.Table),
	// since that's what analyze.go checks before attempting partition skew.
	resolvedMeta, resolvedTbl, err := Resolve(ctx, "nyc.taxis", srv.URL)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolvedTbl == nil {
		t.Error("Resolve returned a nil *table.Table for a live catalog table")
	}
	if resolvedMeta.TableUUID() != meta.TableUUID() {
		t.Error("Resolve's metadata does not match LoadTable's metadata")
	}

	// A missing table should surface catalog.ErrNoSuchTable-flavored errors,
	// not a crash or a nil table.
	if _, err := LoadTable(ctx, cat, "nyc.does-not-exist"); err == nil {
		t.Error("LoadTable for a missing table should return an error")
	}
}
