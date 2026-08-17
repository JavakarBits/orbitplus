# orbitplus Implementation Plan

**Feature:** `orbitplus` | **Spec:** [spec.md](./spec.md) | **Requirements:** [requirements.md](./requirements.md)

## Summary

orbitplus is the TripDetails microservice. It receives data from orbitplusworker, and will eventually process, cache, store, and serve TripDetails to the Orbit application.

Phase 1 covers ingestion only: validate JSON, log the payload, return an acknowledgement response enabling the Worker to ACK.

## Phase 1 — current state

Phase 1 implementation is **complete**. The following behavior is delivered and running:

- `POST /api/tripdetails` — accepts the Worker envelope, validates JSON syntax, logs the raw payload, returns `{"status":1,"message":"Trip details received successfully"}`.
- `GET /health` — returns `{"status":"UP"}`.
- Error responses: HTTP 400 for invalid JSON, HTTP 500 for read failure.
- Configuration: `APP_ENV`, `MASTER_API_PORT`.

### Outstanding Phase 1 work

- **Tests:** Phase 1 has no test coverage. Focused tests should verify:
  - Valid JSON → HTTP 200 + status:1
  - Invalid JSON → HTTP 400 + status:0
  - Request body read failure → HTTP 500 + status:0
  - Health endpoint → HTTP 200 + status:UP
  - Content-Type headers

## Worker → orbitplus integration

orbitplusworker submits a JSON envelope to `POST /api/tripdetails`:

```json
{
  "actionType": "search|busmap|searchbusmap",
  "operatorCode": "...",
  "fromCode": "...",
  "toCode": "...",
  "tripDate": "...",
  "tripCode": "...",
  "fromStationCode": "...",
  "toStationCode": "...",
  "travelDate": "...",
  "orbitResponse": <raw Bits JSON>
}
```

The Worker maps HTTP 200 + `status:1` to ACCEPTED and ACKs the RabbitMQ delivery. HTTP 408/429/5xx are retryable (no ACK). Other errors leave the delivery unacknowledged.

## Future phases

### Phase 2 — TripDetails processing and storage

- Parse Worker envelope: extract `actionType`, `operatorCode`, action-specific fields, `orbitResponse`.
- Process `orbitResponse` according to action type:
  - `search`: trip data, `stageFare[]`, aggregate availability
  - `busmap`: TripDetails with `bus.seatLayoutList[]` (seat layout)
  - `searchbusmap`: combined structure
- Cache processed TripDetails.
- Persist TripDetails to storage.
- Implement duplicate detection → return distinguishable response to Worker.
- Implement stale detection → return distinguishable response to Worker.

### Phase 3 — Orbit-facing read APIs

- Search API for the Orbit application.
- Busmap API for the Orbit application.
- TripDetails API for the Orbit application.
- Station / station details.
- All served from cached/stored data originating from the Worker pipeline.

### Cross-cutting (future)

- **Authentication:** Dedicated Worker → orbitplus context token (details unresolved).
- **Inventory event support:** High-priority refresh triggered by inventory events through a queue (external publisher scope).
- **Periodic refresh:** Scheduler dispatches work to orbitplusworker; data flows through to orbitplus (scheduler scope).
- **Advance loading days:** Scheduling logic (scheduler scope).
- **Orbit app route sync** (scheduler scope).
- **Failure/analytics tracking.**

## Target architecture

```text
                    ┌─────────────────────┐
                    │  Orbit application  │
                    └──────────┬──────────┘
                               │
                    Search / Busmap /
                    TripDetails / Station
                               │
                               ▼
                    ┌─────────────────────┐
                    │     orbitplus        │
                    │                     │
                    │ Ingestion (Phase 1) │
                    │ Processing (Phase 2)│
                    │ Cache/Storage (P2)  │
                    │ Query APIs (Phase 3)│
                    └──────────┬──────────┘
                               ▲
                               │
                       Worker submission
                               │
                    ┌──────────┴──────────┐
                    │   orbitplusworker    │
                    └──────────┬──────────┘
                               │
                               ▼
                         Bits Service
```

Additional write-side triggers (future):
- Inventory events → high-priority queue → refresh
- Scheduler → periodic refresh → orbitplusworker → Bits → orbitplus

## Delivery approach

| Phase | Status | Scope |
|---|---|---|
| Phase 1 | Complete (tests pending) | Ingestion, validation, logging, response contract |
| Phase 2 | Future | Envelope parsing, action processing, cache, storage, duplicate/stale |
| Phase 3 | Future | Orbit-facing read APIs |
| Cross-cutting | Future | Authentication, analytics, event-driven refresh |
