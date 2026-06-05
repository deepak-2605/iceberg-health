# Iceberg Table Health Dashboard — Build Plan

A CLI + lightweight web dashboard that connects to an Apache Iceberg catalog and surfaces
table-health metrics: **snapshot bloat, schema-evolution history, stale snapshots, and
partition skew**. Built in Go.

**Goal:** working, demoable MVP by **June 9**.

---

## Scope & priority

| Priority | Metric | Data source | Effort | Cuttable? |
|---|---|---|---|---|
| 1 | Snapshot bloat | Table metadata (snapshot list) | Low | No |
| 2 | Stale snapshots | Table metadata (timestamps) | Low | No |
| 3 | Schema-evolution history | Table metadata (`schemas` array) | Low–Med | No |
| 4 | Web dashboard (minimal) | Serves the above as JSON + HTML | Med | No |
| 5 | Partition skew | Manifest files (per-partition record counts) | High | **Yes — stretch** |

The "web dashboard" is a Go HTTP server serving a JSON API plus one static HTML page with
a couple of Chart.js charts (CDN). Partition skew is the one thing that may slip.

---

## Tech stack

- **Language:** Go (latest stable) — resolved to go1.26.3
- **Iceberg:** `github.com/apache/iceberg-go` v0.6.0
- **CLI framework:** `spf13/cobra`
- **CLI table output:** `olekukonko/tablewriter` or similar
- **Web:** Go stdlib `net/http` + `html/template`; Chart.js from CDN
- **Test catalog:** Iceberg REST-catalog docker-compose (Spark + REST + MinIO)

---

## Project structure

```
iceberg-health/
├── go.mod
├── cmd/icehealth/main.go        # cobra root + subcommand wiring
├── internal/
│   ├── catalog/connect.go       # load catalog + table by identifier
│   ├── metrics/
│   │   ├── snapshots.go         # snapshot bloat + stale-snapshot analysis
│   │   ├── schema.go            # schema-evolution history
│   │   ├── skew.go              # partition skew (stretch)
│   │   └── report.go            # combined HealthReport struct
│   ├── render/
│   │   ├── cli.go               # pretty table output
│   │   └── json.go              # --json output mode
│   └── server/
│       ├── server.go            # HTTP handlers serving the report as JSON
│       └── static/index.html    # dashboard page (Chart.js via CDN)
├── docker/docker-compose.yml    # local Iceberg REST catalog + spark + minio
└── README.md
```

Keep analysis logic in `internal/metrics` decoupled from rendering — both the CLI and the
web server call the *same* `HealthReport`.

---

## Day-by-day

### Day 0 — setup
- `git init`, `go mod init`. Stand up docker-compose REST catalog; confirm a sample table loads.
- `internal/catalog/connect.go`: connect, `LoadTable`, print UUID + snapshot count.
- **Milestone: it connects and reads.**
- *Go:* modules, packages, `error` convention, `context.Context`.

### Day 1 — snapshot bloat + stale
- `metrics/snapshots.go`: total count, snapshots/day, oldest/newest timestamps.
- Bloat: count beyond a configurable retention threshold + manifest/data-file growth trend.
- Stale: snapshots older than `--max-age` not yet expired.
- Wire `icehealth analyze <table>` to print a CLI table.
- *Go:* structs, methods, slices, `time`, cobra flags.

### Day 2 — schema evolution + JSON mode
- `metrics/schema.go`: read full `schemas` history; diff consecutive schemas by **column ID**.
- Build combined `HealthReport` in `metrics/report.go`.
- Add `--json` output (`render/json.go`).
- *Go:* interfaces, `encoding/json` struct tags, maps, sorting.

### Day 3 — web dashboard (RESUME-CRITICAL)
- `internal/server`: `GET /api/report?table=...` returns the `HealthReport` JSON.
- `static/index.html`: pick a table, fetch JSON, render timeline + schema history + stale list,
  Chart.js bar/line chart for snapshot growth.
- `icehealth serve --port 8080`.
- *Go:* `net/http` handlers, `html/template`, `embed`.

### Day 4 — partition skew (STRETCH)
- `metrics/skew.go`: walk current snapshot manifests, aggregate record counts + sizes per
  partition, compute skew (max/median ratio, coefficient of variation).
- **If hard, cut it — 1–4 already ship.**
- *Go:* manifest iterators, error wrapping (`%w`), goroutines + channels for multi-table fan-out.

### Day 5 — polish + ship
- README with screenshots/GIF, install + usage, architecture diagram.
- Table-driven unit tests on metrics functions.
- Tag `v0.1.0`.

---

## Acceptance criteria

- [ ] `icehealth analyze <table>` prints snapshot bloat, stale snapshots, schema history
- [ ] `icehealth analyze <table> --json` emits clean JSON
- [ ] `icehealth serve` runs a web dashboard with at least one chart
- [ ] README with a screenshot/GIF and clear setup instructions
- [ ] Public GitHub repo, tagged release
- [ ] (Stretch) partition-skew metric in both CLI and web

---

## Test-data recipe

In the Spark container: create a partitioned table, then
1. Insert data in several batches → multiple snapshots.
2. `ALTER TABLE ... ADD COLUMN`, drop a column, rename one → schema-evolution history.
3. Insert skewed data (one partition far larger) → partition skew.
