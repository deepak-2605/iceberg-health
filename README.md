# iceberg-health (`icehealth`)

A CLI **and** lightweight web dashboard that connects to an Apache Iceberg catalog
and surfaces **table-health** metrics — *snapshot bloat*, *stale snapshots*,
*schema-evolution history*, and *partition skew* — so you can spot tables that are
quietly slowing queries down or running up storage costs.

Built in Go on top of [`apache/iceberg-go`](https://github.com/apache/iceberg-go).

```text
Iceberg Table Health — nyc.taxis
UUID 7a3e1c44-… · format v2 · s3://warehouse/nyc/taxis

SNAPSHOTS
  total 8 · bloat 3 (keep newest 5) · stale 7 (> 7d)
  span 2026-01-10 → 2026-06-04 (145 days)

  WHEN        SNAPSHOT          FILES  RECORDS  AGE   STATUS
  2026-06-04  1000000000000008  28     18000    1d    ok
  2026-05-25  1000000000000007  20     10000    11d   STALE
  …

SCHEMA EVOLUTION
  current schema #3 · 6 columns · 4 versions in history
  TRANSITION  CHANGE   COLUMN                 DETAIL
  #0→#1       added    id 6 (tip_amount)      double
  #2→#3       dropped  id 2 (vendor_id)       was int
  #2→#3       renamed  id 4 (distance_miles)  trip_distance → distance_miles
```

The web dashboard renders the same data with summary cards, a snapshot-growth
chart (Chart.js), and the schema-evolution timeline:

> 📸 Run `./icehealth serve` and open <http://localhost:8080> to see it live. To add a
> screenshot to this README, save one as `docs/dashboard.png` and uncomment:
> <!-- ![dashboard](docs/dashboard.png) -->

## Why these four metrics

Iceberg tracks a table as a tree of metadata: `metadata.json` → snapshots →
manifest lists → manifests → data files. Each write makes a new **snapshot**, and
old snapshots/data files are never removed until you explicitly expire them.

| Metric | What it flags | Where it reads from |
|---|---|---|
| **Snapshot bloat** | Snapshots beyond a retention limit — excess metadata + pinned data files | `metadata.json` (cheap) |
| **Stale snapshots** | Snapshots older than `--max-age` that should be expired | `metadata.json` (cheap) |
| **Schema evolution** | Add/drop/rename/retype history, diffed by **stable column ID** (so a rename is a rename, not drop+add) | `metadata.json` (cheap) |
| **Partition skew** | One partition far larger than the rest → query stragglers | manifests (live tables only) |

## Try it in 30 seconds (no catalog, no Docker)

`icehealth` reads either a live catalog table **or** a local `metadata.json`, so
you can run it against the bundled sample immediately:

```bash
go build -o icehealth ./cmd/icehealth

./icehealth analyze ./testdata/nyc_taxis.metadata.json          # pretty table
./icehealth analyze ./testdata/nyc_taxis.metadata.json --json   # machine-readable
./icehealth serve                                               # http://localhost:8080
```

## Usage

```text
icehealth analyze <table|metadata.json> [more tables...] [flags]
  --retention int        keep this many newest snapshots before counting bloat (default 5)
  --max-age duration     snapshots older than this are stale (default 168h)
  --json                 emit JSON instead of a table
  --concurrency int      max tables analyzed in parallel (default 4)
  --catalog-uri string   REST catalog URI for table identifiers (default http://localhost:8181)

icehealth serve [flags]
  --port int             HTTP port (default 8080)
  --catalog-uri string   REST catalog URI (default http://localhost:8181)
```

Pass several tables and they're analyzed concurrently (goroutine fan-out, bounded
by `--concurrency`), with output kept in input order:

```bash
icehealth analyze ./testdata/nyc_taxis.metadata.json ./testdata/web_events.metadata.json
```

A `<table>` argument is either a catalog identifier (`nyc.taxis`) or a path to a
local `metadata.json`. Partition skew is computed only for live catalog tables
(it needs to read manifest files).

## How it works

```
internal/metrics   pure analysis → builds a HealthReport struct (no I/O, no printing)
        │
        ├── internal/render   CLI table  +  --json
        └── internal/server   HTTP API   +  dashboard (GET /api/report?table=…)
```

Both load paths — a live catalog (`tbl.Metadata()`) and a local file
(`ParseMetadataBytes`) — converge on a single `table.Metadata`, and the analysis
depends only on that. So the CLI and the web server render the *exact same*
`HealthReport`, and the metrics are unit-testable with no catalog at all.

## Live catalog (optional)

To point `icehealth` at a real REST catalog, bring up the local stack and seed a
table:

```bash
docker compose -f docker/docker-compose.yml up -d
docker cp docker/seed.sql spark-iceberg:/tmp/seed.sql
docker exec spark-iceberg spark-sql -f /tmp/seed.sql

./icehealth analyze nyc.taxis --catalog-uri http://localhost:8181
```

## Development

```bash
go test ./...     # table-driven tests for the metrics
go vet ./...
```

## Status

CLI, JSON output, and the web dashboard are complete (snapshot bloat, stale
snapshots, schema evolution). Partition-skew code is implemented for live
catalog tables. See [`PLAN.md`](./PLAN.md) for the roadmap.
