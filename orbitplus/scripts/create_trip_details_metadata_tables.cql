-- Run after selecting the target keyspace, for example: USE orbitplus;
-- These active tables store lookup metadata only; TripDetails JSON remains in Dragonfly.
-- Existing trip_details_metadata_by_stage tables are retained as legacy tables and are not used.

CREATE TABLE IF NOT EXISTS trip_details_metadata_by_route (
    operator_code text,
    travel_date text,
    from_station_code text,
    to_station_code text,
    trip_code text,
    trip_stage_code text,
    updated_at timestamp,
    PRIMARY KEY (
        (operator_code, travel_date, from_station_code, to_station_code),
        trip_code,
        trip_stage_code
    )
);

CREATE TABLE IF NOT EXISTS trip_details_metadata_by_schedule (
    operator_code text,
    schedule_code text,
    travel_date text,
    trip_code text,
    trip_stage_code text,
    updated_at timestamp,
    PRIMARY KEY (
        (operator_code, schedule_code, travel_date),
        trip_code,
        trip_stage_code
    )
);


-- One row per Orionmax data[] item. reference_id is Orionmax data[].refid and
-- is echoed by the Worker as referenceId in its TripDetails result envelope.
CREATE TABLE IF NOT EXISTS queue_metrix (
    reference_id text PRIMARY KEY,
    activity_type text,
    action_type text,
    operator_code text,
    schedule_code text,
    trip_code text,
    from_station text,
    to_station text,
    travel_date text,
    zone text,
    message text,
    trip_codes set<text>,
    queue_status text,
    queued_at timestamp,
    completed_at timestamp,
    dead_lettered_at timestamp,
    failure_message text,
    updated_at timestamp
);

