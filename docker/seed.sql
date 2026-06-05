-- Seed a synthetic Iceberg table whose *health* the tool can actually flag.
-- Produces: multiple snapshots, a real schema-evolution history, and partition skew.
--
-- Run it inside the spark-iceberg container (see README / the run command):
--   docker cp docker/seed.sql spark-iceberg:/tmp/seed.sql
--   docker exec spark-iceberg spark-sql -f /tmp/seed.sql
--
-- "demo" is the Spark catalog alias wired to the REST catalog. On the Iceberg
-- side the table identifier is just "nyc.taxis" — that's what icehealth loads.

DROP TABLE IF EXISTS demo.nyc.taxis;
CREATE NAMESPACE IF NOT EXISTS demo.nyc;

CREATE TABLE demo.nyc.taxis (
  trip_id         BIGINT,
  vendor_id       INT,
  passenger_count INT,
  trip_distance   DOUBLE,
  payment_type    STRING
)
USING iceberg
PARTITIONED BY (payment_type);

-- ── Several insert batches → several snapshots (snapshot-bloat + stale story) ──
INSERT INTO demo.nyc.taxis
SELECT id, CAST(rand()*3 AS INT)+1, CAST(rand()*4 AS INT)+1, rand()*20,
       CASE WHEN rand()<0.7 THEN 'card' WHEN rand()<0.9 THEN 'cash' ELSE 'wallet' END
FROM range(1000);

INSERT INTO demo.nyc.taxis
SELECT id, CAST(rand()*3 AS INT)+1, CAST(rand()*4 AS INT)+1, rand()*20,
       CASE WHEN rand()<0.7 THEN 'card' WHEN rand()<0.9 THEN 'cash' ELSE 'wallet' END
FROM range(1000, 2000);

INSERT INTO demo.nyc.taxis
SELECT id, CAST(rand()*3 AS INT)+1, CAST(rand()*4 AS INT)+1, rand()*20,
       CASE WHEN rand()<0.7 THEN 'card' WHEN rand()<0.9 THEN 'cash' ELSE 'wallet' END
FROM range(2000, 3000);

-- ── Schema evolution → entries in the `schemas` history (diff by column ID) ──
-- Column order after these changes:
--   trip_id, passenger_count, distance_miles, payment_type, tip_amount, fare_amount
ALTER TABLE demo.nyc.taxis ADD COLUMN tip_amount DOUBLE;
ALTER TABLE demo.nyc.taxis ADD COLUMN fare_amount DOUBLE;
ALTER TABLE demo.nyc.taxis RENAME COLUMN trip_distance TO distance_miles;
ALTER TABLE demo.nyc.taxis DROP COLUMN vendor_id;

-- Two more inserts under the evolved schema (new columns populated).
INSERT INTO demo.nyc.taxis
SELECT id, CAST(rand()*4 AS INT)+1, rand()*20,
       CASE WHEN rand()<0.7 THEN 'card' WHEN rand()<0.9 THEN 'cash' ELSE 'wallet' END,
       rand()*5, rand()*40
FROM range(3000, 4000);

-- ── Skewed batch: ~8000 rows all land in the 'card' partition (skew story) ──
INSERT INTO demo.nyc.taxis
SELECT id, CAST(rand()*4 AS INT)+1, rand()*20, 'card', rand()*5, rand()*40
FROM range(4000, 12000);

-- Sanity check (printed by spark-sql):
SELECT payment_type, count(*) AS rows FROM demo.nyc.taxis GROUP BY payment_type ORDER BY rows DESC;
