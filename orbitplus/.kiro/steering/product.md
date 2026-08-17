# Product

orbitplus is a Go HTTP microservice that receives TripDetails data from orbitplusworker. Its architectural responsibility spans ingestion, processing, caching, storage, and serving TripDetails to the Orbit application.

Phase 1 scope is limited to ingestion: accept the Worker envelope at `POST /api/tripdetails`, validate JSON syntax, log the payload, and return the acknowledgement response. Do not add action-specific processing, cache, storage, query APIs, duplicate/stale detection, or authentication validation in Phase 1 unless the task explicitly expands the approved scope.

Future phases will introduce TripDetails processing, cache/storage, Orbit-facing read APIs, duplicate/stale detection, and Worker → orbitplus authentication. These are future orbitplus responsibilities, not permanent exclusions. Preserve incoming data rather than inventing defaults or dropping unknown fields.
