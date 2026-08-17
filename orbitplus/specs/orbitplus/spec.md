# orbitplus Design Specification

## Purpose

orbitplus is the TripDetails microservice in the OrbitPlus system. It receives TripDetails data from orbitplusworker, and is architecturally responsible for processing, caching, storing, and serving TripDetails-related data to the Orbit application.

### Current Phase 1 scope

Phase 1 is limited to TripDetails ingestion: receive the Worker submission, validate JSON syntax, log the payload, and return an acknowledgement response.

### Target architecture

The overall OrbitPlus architecture (from the OrbitPlus workflow) encompasses:

```text
Orbit application
    ↓ (read: search / busmap / tripdetails / station)
orbitplus
    ↑ (write: Worker submission)
orbitplusworker → Bits Service → orbitplus ingestion
```

Additional future flows:
- Periodic refresh: scheduler → orbitplusworker → Bits → orbitplus
- Inventory events: high-priority event → queue → refresh/processing
- Orbit app route sync and advance loading days (scheduler scope)

These are documented for architectural context. They are not implemented in Phase 1.

## Phase 1 — Worker ingestion

### Worker → orbitplus contract

orbitplusworker submits TripDetails data to orbitplus through an HTTP POST.

**Endpoint:** `POST /api/tripdetails`

**Request body (Worker envelope):**

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

- `actionType` and `operatorCode` are always present.
- `fromCode`, `toCode`, `tripDate` are present for `search` and `searchbusmap`.
- `tripCode`, `fromStationCode`, `toStationCode`, `travelDate` are present for `busmap`.
- `orbitResponse` contains the raw Bits JSON response forwarded unchanged by the Worker.

The outer envelope fields are Worker metadata describing what was fetched. The actual Bits TripDetails data is inside `orbitResponse`.

**Action types (lowercase):**

| actionType | Description |
|---|---|
| `search` | Trip search results including fares and availability |
| `busmap` | Seat layout and individual seat data |
| `searchbusmap` | Combined search and busmap data |

### HTTP response contract

| Scenario | HTTP Status | Response Body |
|---|---|---|
| Valid JSON accepted | 200 | `{"status":1,"message":"Trip details received successfully"}` |
| Invalid JSON | 400 | `{"status":0,"message":"Invalid trip details JSON"}` |
| Request body read failure | 500 | `{"status":0,"message":"Internal server error"}` |

The Worker maps HTTP 2xx + `status:1` to `ACCEPTED` and ACKs the RabbitMQ delivery. HTTP 408/429/5xx are treated as retryable (delivery stays unacknowledged). Other non-2xx responses are errors (delivery stays unacknowledged).

### Phase 1 processing behavior

1. Read the complete request body.
2. Validate that the body is syntactically valid JSON using `json.Unmarshal`.
3. If validation fails, return the invalid-JSON response.
4. If the body read fails, return the read-failure response.
5. If valid, log the raw JSON payload to the terminal.
6. Return the success response.

Phase 1 does not parse the Worker envelope, extract `actionType`, or perform action-specific processing. The entire request body is treated as an opaque JSON value for validation purposes.

### Health endpoint

`GET /health` returns HTTP 200 with `{"status":"UP"}`.

### Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `APP_ENV` | No | `development` | Deployment environment |
| `MASTER_API_PORT` | No | `8082` | HTTP server port |

### Logging

Phase 1 logs the raw JSON payload to the terminal after successful validation. Logs do not include pretty-printed JSON in the current implementation.

## Future phases

The following are future orbitplus responsibilities. They are not implemented in Phase 1 and should not be treated as permanently out of scope.

### Phase 2 — TripDetails processing and storage

- Parse the Worker envelope: extract `actionType`, `operatorCode`, action-specific fields, and `orbitResponse`.
- Process `orbitResponse` according to `actionType`:
  - `search`: trip data, `stageFare[]`, aggregate availability (`availableSeatCount`)
  - `busmap`: TripDetails with `bus.seatLayoutList[]` (individual seat layout data)
  - `searchbusmap`: combined search and busmap structure
- Cache processed TripDetails.
- Store TripDetails persistently.
- Detect duplicate submissions and return a distinguishable response (enabling Worker to ACK with `DUPLICATE`).
- Detect stale data and return a distinguishable response (enabling Worker to ACK with `STALE`).

### Phase 3 — Orbit-facing read APIs

- Search API: serve trip search results to the Orbit application.
- Busmap API: serve seat layout data to the Orbit application.
- TripDetails API: serve combined trip information.
- Station / station details: serve station-related data.

These APIs serve cached/stored data originating from the Worker → Bits → orbitplus pipeline.

### Cross-cutting (future)

- **Authentication:** Worker → orbitplus communication is intended to use a dedicated context token. The token name, HTTP header, format, validation mechanism, storage, and configuration variable names are unresolved. The current Worker implementation sends `Authorization: Bearer <token>` when configured; orbitplus does not validate it in Phase 1.
- **Failure/analytics tracking.**
- **Orbit app route sync** (scheduler scope).
- **Advance loading days** (scheduler scope).
- **Inventory high-priority event queue** (external publisher scope).

## Phase 1 — out of scope

The following are not part of Phase 1:

- Action-specific processing of `orbitResponse`
- Cache or storage
- Query/read APIs
- Duplicate or stale detection
- Authentication validation
- Scheduler, publisher, or Worker behavior
- RabbitMQ consumption (belongs to orbitplusworker)
- Bits Service integration (belongs to orbitplusworker)

## Domain model note

The current domain model (`TripDetailsResponse{Status, Datetime, Data}`) is used only as a JSON syntax validation target. Its fields do not correspond to the Worker envelope structure. This model will be redesigned when action-specific processing begins in Phase 2.
