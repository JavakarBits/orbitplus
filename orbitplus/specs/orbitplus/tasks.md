# orbitplus Implementation Tasks

## Overview

Phase 1 delivers the Worker ingestion capability: receive the Worker envelope at `POST /api/tripdetails`, validate JSON syntax, log the payload, and return the acknowledgement response.

## Phase 1 Tasks

- [x] 1. Create the orbitplus service structure: entry point, configuration, application service, HTTP router.
- [x] 2. Expose `POST /api/tripdetails` as the Worker ingestion endpoint.
- [x] 3. Read the complete request body before JSON validation. On read failure, return HTTP 500 with `{"status":0,"message":"Internal server error"}`.
- [x] 4. Validate the request body as syntactically valid JSON. On failure, return HTTP 400 with `{"status":0,"message":"Invalid trip details JSON"}`.
- [x] 5. On valid JSON, log the raw payload to the terminal and return HTTP 200 with `{"status":1,"message":"Trip details received successfully"}`.
- [x] 6. Expose `GET /health` returning `{"status":"UP"}`.
- [x] 7. Configuration: support `APP_ENV` and `MASTER_API_PORT` environment variables.
- [x] 8. Add focused tests for: valid JSON acceptance (HTTP 200 + status:1), invalid JSON rejection (HTTP 400 + status:0), request body read failure (HTTP 500 + status:0), health endpoint (HTTP 200), and Content-Type headers.
- [ ] 9. Verify the Worker integration contract: confirm orbitplusworker receives `status:1` and ACKs successfully. This requires the separate Worker repository and a RabbitMQ integration environment.

## Scope guardrails (Phase 1)

Do not implement in Phase 1: action-specific processing, cache, storage, query APIs, duplicate/stale detection, authentication validation, or scheduler/Worker behavior.

These are future orbitplus responsibilities documented in [plan.md](./plan.md) and [spec.md](./spec.md). They are not permanent exclusions.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1"] },
    { "id": 1, "tasks": ["2", "3", "4", "5", "6", "7"] },
    { "id": 2, "tasks": ["8", "9"] }
  ]
}
```
