-- Run once after selecting the target keyspace, for example: USE orbitplus;
--
-- Runtime-editable cache behaviour, set from the Master admin "Cache settings"
-- page (not from .env). A single config row identified by a fixed key.
--
--   hot_max_age_seconds / warm_max_age_seconds
--       Age thresholds that classify a cached trip stage as Hot / Warm / Cold
--       (age = now - updated_at). age <= hot => Hot; <= warm => Warm; else Cold.
--
--   window_enabled / window_start_minute / window_end_minute / window_timezone
--       Optional daily "cache hours" window (minutes-from-midnight in the given
--       IANA timezone, e.g. Asia/Kolkata). Inside the window reads serve from
--       cache; outside it ("non-cache hours") reads go live to BITS. When
--       window_enabled is false, caching behaves normally all day.
CREATE TABLE IF NOT EXISTS cache_settings (
    config_key           text PRIMARY KEY,
    hot_max_age_seconds  bigint,
    warm_max_age_seconds bigint,
    window_enabled       boolean,
    window_start_minute  int,
    window_end_minute    int,
    window_timezone      text,
    updated_at           timestamp
);
