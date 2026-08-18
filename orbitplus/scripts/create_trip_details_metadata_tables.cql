-- Run after selecting the target keyspace, for example: USE orbitplus;
-- These tables store lookup metadata only; TripDetails JSON remains in Dragonfly.

CREATE TABLE IF NOT EXISTS trip_details_metadata_by_stage (
    operator_code text,
    trip_code text,
    travel_date text,
    from_station_code text,
    to_station_code text,
    trip_stage_code text,
    updated_at timestamp,
    PRIMARY KEY (
        (operator_code, trip_code, travel_date, from_station_code, to_station_code),
        trip_stage_code
    )
);

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
