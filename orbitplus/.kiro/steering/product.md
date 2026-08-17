# Product

OrbitPlus Master is a Go HTTP ingestion service for raw Bits TripDetails payloads sent by a Worker. The current Phase 1 service exposes a liveness endpoint and accepts a TripDetails POST, validates JSON, logs the received payload, and returns a JSON outcome.

Keep the current phase deliberately small. Do not add persistence, caching, queues, authentication, retries, querying, duplicate/freshness logic, or schema-driven transformation unless the task explicitly expands the approved scope. Preserve incoming TripDetails data rather than inventing defaults or dropping unknown fields.
