-- Run once, after selecting the target keyspace (for example: USE orbitplus;).
-- Adds the outgoing RabbitMQ message to the existing queue_metrix table.
-- Do not run this from Master at runtime.
ALTER TABLE queue_metrix ADD message text;
