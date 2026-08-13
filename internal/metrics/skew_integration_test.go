//go:build integration

// This file validates AnalyzeSkew's manifest-reading I/O against a REAL Iceberg
// table — real Arrow-written data files, real manifests — with no Docker, no
// S3/MinIO, and no network. It uses iceberg-go's SQL catalog (backed by a local
// SQLite file) and the local filesystem for data, both fully in-process.
//
// It's build-tag gated (`integration`) and excluded from the default `go test
// ./...` / CI run, since it exercises the heavier Arrow write path. Run it with:
//
//	go test -tags=integration ./internal/metrics/... -run TestAnalyzeSkew_RealTable -v
package metrics

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	iceberg "github.com/apache/iceberg-go"
	icecat "github.com/apache/iceberg-go/catalog"
	sqlcat "github.com/apache/iceberg-go/catalog/sql"
	itable "github.com/apache/iceberg-go/table"

	_ "modernc.org/sqlite" // pure-Go sqlite driver, registers as "sqlite" — no cgo/C compiler needed
)

// TestAnalyzeSkew_RealTable creates a genuinely live Iceberg table (partitioned
// by "category"), writes real skewed data into it via the library's own Arrow
// write path, then runs OUR AnalyzeSkew against the resulting table and checks
// the numbers match what ComputeSkew's unit tests already proved for the same
// distribution ([1000, 100, 100] -> max/median ratio 10.0).
func TestAnalyzeSkew_RealTable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// 1. An in-process catalog backed by a local SQLite file — no server.
	db, err := sql.Open("sqlite", filepath.Join(dir, "catalog.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	cat, err := sqlcat.NewCatalog("test", db, sqlcat.SQLite, iceberg.Properties{
		"init_catalog_tables": "true",
	})
	if err != nil {
		t.Fatalf("new sql catalog: %v", err)
	}

	if err := cat.CreateNamespace(ctx, icecat.ToIdentifier("db"), nil); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	// 2. A real Iceberg schema + partition spec (identity on "category").
	schema := iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "id", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "category", Type: iceberg.PrimitiveTypes.String, Required: true},
	)
	spec := iceberg.NewPartitionSpec(iceberg.PartitionField{
		SourceIDs: []int{2}, FieldID: 1000, Name: "category", Transform: iceberg.IdentityTransform{},
	})

	loc := "file://" + filepath.ToSlash(filepath.Join(dir, "warehouse", "skewed"))
	tbl, err := cat.CreateTable(ctx, icecat.ToIdentifier("db", "skewed"), schema,
		icecat.WithPartitionSpec(&spec), icecat.WithLocation(loc))
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	// 3. Real Arrow data: 1000 rows in "huge", 100 each in "small1"/"small2" —
	// the exact distribution skew_test.go's "skewed" case already validated the
	// math for (max/median = 10.0x).
	arrSchema, err := itable.SchemaToArrowSchema(schema, nil, true, false)
	if err != nil {
		t.Fatalf("schema to arrow: %v", err)
	}

	mem := memory.DefaultAllocator
	bldr := array.NewRecordBuilder(mem, arrSchema)
	defer bldr.Release()
	idBldr := bldr.Field(0).(*array.Int64Builder)
	catBldr := bldr.Field(1).(*array.StringBuilder)

	rows := []struct {
		category string
		count    int
	}{
		{"huge", 1000},
		{"small1", 100},
		{"small2", 100},
	}
	var id int64
	for _, r := range rows {
		for i := 0; i < r.count; i++ {
			idBldr.Append(id)
			catBldr.Append(r.category)
			id++
		}
	}
	rec := bldr.NewRecordBatch()
	defer rec.Release()

	arrTbl := array.NewTableFromRecords(arrSchema, []arrow.RecordBatch{rec})
	defer arrTbl.Release()

	tbl, err = tbl.AppendTable(ctx, arrTbl, 1200, nil)
	if err != nil {
		t.Fatalf("append table: %v", err)
	}

	// 4. Run OUR code against this real table — the exact path a live REST
	// catalog would exercise, just via the SQL catalog for the connection.
	got, err := AnalyzeSkew(ctx, tbl)
	if err != nil {
		t.Fatalf("AnalyzeSkew: %v", err)
	}

	if got.PartitionCount != 3 {
		t.Errorf("PartitionCount = %d, want 3 (partitions: %+v)", got.PartitionCount, got.Partitions)
	}
	if got.TotalRecords != 1200 {
		t.Errorf("TotalRecords = %d, want 1200", got.TotalRecords)
	}
	if got.Largest == nil || got.Largest.RecordCount != 1000 {
		t.Errorf("Largest = %+v, want RecordCount 1000", got.Largest)
	}
	if got.MaxMedianRatio < 9.9 || got.MaxMedianRatio > 10.1 {
		t.Errorf("MaxMedianRatio = %v, want ~10.0", got.MaxMedianRatio)
	}
	t.Logf("real-table skew report: %+v", got)
}
