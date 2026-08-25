-- Run once after selecting the target keyspace, for example: USE orbitplus;
-- Do not run this from Master at runtime.
-- New operators are inserted by Master with active_flag=1 and a supported zone_code.
-- To disable an operator, move its row from the active partition (1) to inactive (0).
CREATE TABLE IF NOT EXISTS operators (
    active_flag int,
    operator_code text,
    zone_code text,
    created_at timestamp,
    updated_at timestamp,
    PRIMARY KEY ((active_flag), operator_code)
);
