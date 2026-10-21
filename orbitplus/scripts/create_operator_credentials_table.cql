-- Run once after selecting the target keyspace, for example: USE orbitplus;
-- Do not run this from Master at runtime.
--
-- The Master resolves each operator's BITS credential from the Orbit credential
-- service at operator-registration time and stores it here, keyed by operator
-- code. Workers fetch credentials from the Master (not Orbit) and cache them
-- locally; this table is the Master-side source of truth.
--
-- Secrets at rest: bits_username and bits_api_token are credential material.
-- Protect this table with Cassandra authentication/authorization in production.
CREATE TABLE IF NOT EXISTS operator_credentials (
    operator_code  text PRIMARY KEY,
    zone_code      text,
    zone_url       text,
    bits_username  text,
    bits_api_token text,
    active_flag    int,
    updated_at     timestamp
);
