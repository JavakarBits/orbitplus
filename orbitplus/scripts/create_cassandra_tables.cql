-- orbitplusmaster Cassandra schema, authoritative for all five tables.
--
-- Master does NOT create these at startup. Its Cassandra repositories connect
-- to existing tables, so this script is the only source of the schema and must
-- be kept in sync with:
--   internal/infrastructure/cassandra/queue_metrix.go
--   internal/infrastructure/cassandra/tripdetails_metadata.go
--   internal/infrastructure/cassandra/periodic_refresh_routes.go
--
-- Column naming note: queue_metrix and the two metadata tables use trip_date.
-- periodic_refresh_routes still uses travel_date. That inconsistency is
-- deliberate here only because it matches the Go code today.

-- Create the keyspace first. Adjust replication for your environment;
-- SimpleStrategy with factor 1 is single-node only.
-- CREATE KEYSPACE IF NOT EXISTS orbitplus
--     WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

-- USE orbitplus;


-- ---------------------------------------------------------------------------
-- 1. Queue job lifecycle for Orionmax inventory events and periodic refresh.
-- RECEIVED -> QUEUED -> COMPLETED, or -> DEAD on failure.
-- queue_id is the Orionmax data[].refid, or a generated periodic-refresh-<hex>.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS queue_metrix (
    queue_id text PRIMARY KEY,
    zone text,
    action_type text,
    activity_type text,
    completed_at timestamp,
    dead_lettered_at timestamp,
    failure_message text,
    from_station text,
    operator_code text,
    queue_status text,
    queued_at timestamp,
    schedule_code text,
    to_station text,
    trip_date text,
    trip_code text,
    updated_at timestamp
);


-- ---------------------------------------------------------------------------
-- 2. Route lookup metadata. Written by the SEARCH ingestion path only.
-- Read by FindStagesByRoute to serve persisted Search reads.
-- TripDetails JSON itself lives in Dragonfly, not here.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS trip_details_metadata_by_route (
    operator_code text,
    trip_date text,
    from_station_code text,
    to_station_code text,
    trip_code text,
    trip_stage_code text,
    updated_at timestamp,
    PRIMARY KEY (
        (operator_code, trip_date, from_station_code, to_station_code),
        trip_code,
        trip_stage_code
    )
);


-- ---------------------------------------------------------------------------
-- 3. Schedule lookup metadata. Written only when a SEARCH entry carries a
-- schedule code. Read by FindStagesBySchedule when building busmap refresh
-- jobs, so busmap events cannot resolve a trip code until this is populated.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS trip_details_metadata_by_schedule (
    operator_code text,
    schedule_code text,
    trip_date text,
    trip_code text,
    trip_stage_code text,
    updated_at timestamp,
    PRIMARY KEY (
        (operator_code, schedule_code, trip_date),
        trip_code,
        trip_stage_code
    )
);


-- ---------------------------------------------------------------------------
-- 4. Routes the periodic refresh scheduler queues on each tick.
--
-- WARNING: is_active is the sole partition key, so this table has exactly two
-- partitions (true and false). Every active route lives in one partition on one
-- replica set, which cannot distribute and grows without bound. A partition key
-- also cannot be updated, so moving a route between active and inactive needs a
-- DELETE from the old partition as well as the INSERT. SavePeriodicRefreshRoute
-- only inserts today, so a state change would leave two rows for one route.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS periodic_refresh_routes (
    is_active boolean,
    operator_code text,
    travel_date text,
    from_station text,
    to_station text,
    ticket_count int,
    updated_at timestamp,
    PRIMARY KEY (is_active, operator_code, travel_date, from_station, to_station)
);


-- ---------------------------------------------------------------------------
-- 5. Cache freshness differences.
--
-- One row per non-matching live verification, so the table grows with problems
-- rather than with traffic. MATCH outcomes are not stored.
--
-- verification_outcome is one of DIFFERENT, CACHE_MISSING, SOURCE_UNAVAILABLE.
--
-- detected_on is the UTC calendar date and is part of the partition key so
-- partitions stay bounded and every read restricts a partition key. Never add a
-- query that omits operator_code or detected_on: that is what forces the
-- queue_metrix report into a full cluster scan.
--
-- Clustering by detected_at DESC means the report reads newest first without
-- sorting in application code.
--
-- NOT YET WRITTEN TO. Creating it now is harmless and settles the column set,
-- which matters because only primary key columns can be renamed later.
-- ---------------------------------------------------------------------------

-- The row is a summary, not an archive. Full before-and-after payloads are
-- deliberately not stored: they are large, Bits can be asked again, and the
-- cache is repaired immediately, so a stored snapshot would describe a state
-- that no longer exists. difference_paths is what makes a row actionable.
CREATE TABLE IF NOT EXISTS cache_freshness_difference (
    operator_code text,
    detected_on text,
    detected_at timestamp,
    difference_id uuid,
    action_type text,
    trip_code text,
    trip_stage_code text,
    from_code text,
    to_code text,
    trip_date text,
    verification_outcome text,
    difference_count int,
    difference_paths list<text>,
    cache_repaired boolean,
    PRIMARY KEY ((operator_code, detected_on), detected_at, difference_id)
) WITH CLUSTERING ORDER BY (detected_at DESC, difference_id ASC);


-- ---------------------------------------------------------------------------
-- Migrating an existing deployment
--
-- Only primary key columns can be renamed in Cassandra. travel_date is a
-- regular column in queue_metrix, so it must be added and dropped:
--
--   ALTER TABLE queue_metrix ADD trip_date text;
--   -- optionally backfill trip_date from travel_date, then:
--   ALTER TABLE queue_metrix DROP travel_date;
--   ALTER TABLE queue_metrix RENAME reference_id TO queue_id;   -- if still present
--
-- In the metadata tables trip_date IS part of the partition key, so:
--
--   ALTER TABLE trip_details_metadata_by_route RENAME travel_date TO trip_date;
--   ALTER TABLE trip_details_metadata_by_schedule RENAME travel_date TO trip_date;
--
-- Removed schema: trip_details_metadata_by_stage was replaced by the by_route
-- and by_schedule tables. Nothing reads or writes it. Dropping is irreversible,
-- so confirm no external consumer depends on it first:
--
--   DROP TABLE IF EXISTS trip_details_metadata_by_stage;
-- ---------------------------------------------------------------------------
