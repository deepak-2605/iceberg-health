// Package catalog is a thin adapter over apache/iceberg-go for connecting to a
// catalog and loading tables by identifier. It exists so the rest of the program
// depends on a small, stable surface ("connect, then load a table") instead of
// the full upstream catalog API.
package catalog

import (
	"context"
	"fmt"
	"os"
	"strings"

	// Aliased because our own package is also called "catalog". The alias keeps
	// the two clearly distinct everywhere they appear in this file.
	icecat "github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/rest"
	"github.com/apache/iceberg-go/table"
)

// Connect dials an Iceberg REST catalog at uri and returns a handle to it.
//
// It returns the icecat.Catalog *interface* rather than the concrete
// *rest.Catalog on purpose: callers only ever need to load tables, and coding
// against the interface means adding Glue/Hive support later won't change this
// signature. (Same reasoning as net.Dial returning net.Conn.)
//
// Extra behaviour — auth tokens, S3/MinIO settings, TLS — is supplied by the
// caller through rest.Option values, which we simply forward. We don't wrap or
// reinvent them.
func Connect(ctx context.Context, uri string, opts ...rest.Option) (icecat.Catalog, error) {
	cat, err := rest.NewCatalog(ctx, "icehealth", uri, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to REST catalog at %q: %w", uri, err)
	}
	return cat, nil
}

// LoadTable loads a single table given a dotted identifier such as "nyc.taxis".
//
// Iceberg identifiers are []string (namespace levels + table name); we split the
// human-friendly dotted form into that shape via icecat.ToIdentifier.
//
// Note for later: the REST catalog returns the table *metadata* inline, so the
// snapshot/schema metrics (Days 1–3) work without any S3 access. Reading the
// manifest/data layer (Day 4 partition skew) will additionally need S3/MinIO
// options passed through Connect.
func LoadTable(ctx context.Context, cat icecat.Catalog, identifier string) (*table.Table, error) {
	parts := strings.Split(identifier, ".")
	ident := icecat.ToIdentifier(parts...)

	tbl, err := cat.LoadTable(ctx, ident)
	if err != nil {
		return nil, fmt.Errorf("loading table %q: %w", identifier, err)
	}
	return tbl, nil
}

// LoadMetadataFromFile reads an Iceberg metadata.json from local disk and returns
// the parsed Metadata, without needing a running catalog. Handy for development,
// tests, and analyzing an exported metadata file directly.
//
// It returns table.Metadata (the interface) because that's all our analysis layer
// needs — and both load paths (catalog and file) converge on it: a live table
// gives Metadata via tbl.Metadata(), and here we parse it straight from bytes.
// Keeping the metrics dependent on Metadata, not on Table or Catalog, makes them
// trivially testable.
func LoadMetadataFromFile(path string) (table.Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading metadata file %q: %w", path, err)
	}

	meta, err := table.ParseMetadataBytes(data)
	if err != nil {
		return nil, fmt.Errorf("parsing iceberg metadata %q: %w", path, err)
	}
	return meta, nil
}

// Resolve loads a table from either a local metadata.json path or a catalog
// identifier (auto-detected: an existing file path uses the file loader).
//
// It always returns the Metadata. The *table.Table is non-nil only for the
// catalog path — manifest-level metrics like partition skew need the live table
// (its FileIO), which a metadata-only file load can't provide.
func Resolve(ctx context.Context, source, catalogURI string) (table.Metadata, *table.Table, error) {
	if info, statErr := os.Stat(source); statErr == nil && !info.IsDir() {
		meta, err := LoadMetadataFromFile(source)
		return meta, nil, err
	}

	// If it clearly looks like a file path (a separator or a .json suffix) but
	// doesn't exist, say so plainly instead of misfiring at the catalog.
	if strings.HasSuffix(source, ".json") || strings.ContainsAny(source, `/\`) {
		return nil, nil, fmt.Errorf("metadata file not found: %q", source)
	}

	cat, err := Connect(ctx, catalogURI)
	if err != nil {
		return nil, nil, err
	}
	tbl, err := LoadTable(ctx, cat, source)
	if err != nil {
		return nil, nil, err
	}
	return tbl.Metadata(), tbl, nil
}
