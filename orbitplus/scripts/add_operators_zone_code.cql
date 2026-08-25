-- Run once after selecting the target keyspace, for example: USE orbitplus;
-- Do not run this from Master at runtime.
-- Existing operators need a supported zone code added through the Operators UI
-- or a later Orionmax event before periodic refresh can queue for them.
ALTER TABLE operators ADD zone_code text;
