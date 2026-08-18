# orbitplus Implementation Plan

**Feature:** `orbitplus` | **Spec:** [spec.md](./spec.md) | **Requirements:** [requirements.md](./requirements.md)

## Summary

orbitplus is the TripDetails microservice. It receives data from orbitplusworker, and will eventually process, cache, store, and serve TripDetails to the Orbit application.

Phase 1 covers ingestion only: validate JSON, log the payload, return an acknowledgement response enabling the Worker to ACK.

## Phase 1 — current state

Phase 1 implementation and repository-side verification are **complete**. The following behavior is delivered and running:

- `POST /api/tripdetails` — accepts the Worker envelope, validates JSON syntax, logs the raw payload, returns `{"status":1,"message":"Trip details received successfully"}`.
- `GET /health` — returns `{"status":"UP"}`.
- Error responses: HTTP 400 for invalid JSON, HTTP 500 for read failure.
- Configuration: `APP_ENV`, `MASTER_API_PORT`.

### Verification status

- **Automated coverage:** Focused HTTP tests verify valid JSON acceptance, invalid JSON rejection, request-body read failures, the health endpoint, `Content-Type` headers, and that the success log preserves the raw request body unchanged.
- **Worker ACK integration:** Pending verification in the separate `orbitplusworker` repository with a RabbitMQ integration environment; that repository is not available in this workspace.

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

Implemented for cacheable `search` and `busmap` submissions:

- Parse Worker envelopes and persist Trip, Stage, BUSMAP, and route/stage metadata records.
- Cache raw TripDetails content in Dragonfly and Stage Metadata in Cassandra.

Remaining Phase 2 work: `searchbusmap` storage, duplicate/stale handling, and operational hardening.

### Phase 3 — Orbit-facing read APIs

Implemented:

- BusIQ-compatible persisted Search API.
- BusIQ-compatible persisted Busmap API with stage-specific seat layouts.

Future: combined TripDetails and station APIs.

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
| Phase 1 | Repository complete; Worker ACK verification pending | Ingestion, validation, raw logging, response contract |
| Phase 2 | Partially implemented | Search/Busmap persistence and metadata lookup; SEARCHBUSMAP, duplicate/stale pending |
| Phase 3 | Partially implemented | Persisted Search and Busmap APIs; TripDetails and station APIs pending |
| Cross-cutting | Future | Authentication, analytics, event-driven refresh |
