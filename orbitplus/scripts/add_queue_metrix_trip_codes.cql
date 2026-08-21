-- Run once, after selecting the target keyspace (for example: USE orbitplus;).
-- Adds the unique trip codes whose entries completed persistence for one Worker job.
-- Do not run this from Master at runtime.
ALTER TABLE queue_metrix ADD trip_codes set<text>;
