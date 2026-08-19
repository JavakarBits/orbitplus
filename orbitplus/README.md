# orbitplus

orbitplus is the TripDetails microservice in the OrbitPlus system. It receives TripDetails data from orbitplusworker and is architecturally responsible for processing, caching, storing, and serving TripDetails to the Orbit application.

## Current scope

Phase 1 provides Worker ingestion only:

- Accepts the Worker submission at `POST /api/tripdetails`, validates JSON syntax, logs the raw payload, and returns a success response.
- Exposes `GET /health` for liveness checks.
- Reconstructs persisted Search and Busmap responses from Cassandra metadata and Dragonfly; it never calls an upstream TripDetails API.

Phase 1 does **not** implement (these are future phases):
- Action-specific TripDetails processing
- Cache or storage
- Combined TripDetails and station query APIs for the Orbit application
- Duplicate or stale detection
- Worker → orbitplus authentication validation

## Worker → orbitplus contract

orbitplusworker submits TripDetails data as a JSON envelope:

```json
{
  "actionType": "search",
  "operatorCode": "OP1",
  "fromCode": "CITY_A",
  "toCode": "CITY_B",
  "tripDate": "2026-08-20",
  "orbitResponse": {
    "status": 1,
    "datetime": "2026-08-13 15:20:45",
    "data": [...]
  }
}
```

- `actionType`: `search`, `busmap`, or `searchbusmap` (lowercase).
- `operatorCode`: identifies the operator.
- Action-specific fields: `fromCode`/`toCode`/`tripDate` for search/searchbusmap; `tripCode`/`fromStationCode`/`toStationCode`/`travelDate` for busmap.
- `orbitResponse`: the raw Bits JSON response forwarded unchanged by the Worker.

**Success response (HTTP 200):**

```json
{"status":1,"message":"Trip details received successfully"}
```

The Worker maps `status:1` to ACCEPTED and ACKs the RabbitMQ delivery.

**Error responses:**

| Scenario | HTTP Status | Body |
|---|---|---|
| Invalid JSON | 400 | `{"status":0,"message":"Invalid trip details JSON"}` |
| Read failure | 500 | `{"status":0,"message":"Internal server error"}` |

## Running locally

```powershell
$env:APP_ENV="development"
$env:MASTER_API_PORT="8082"
go run ./cmd/orbitplusmaster
```

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `APP_ENV` | No | `development` | Deployment environment label |
| `MASTER_API_PORT` | No | `8082` | Port the HTTP server listens on |
| `PERIODIC_REFRESH_INTERVAL` | No | Disabled | Positive Go duration (for example `15m`) that enables periodic route refresh; requires Cassandra/storage plus `RABBITMQ_URL` and `RABBITMQ_EXCHANGE` |

## Endpoints

### `POST /api/tripdetails`

Accepts the Worker envelope. Validates that the body is syntactically valid JSON, logs the raw payload, and returns the success response.

### `GET /health`

Returns basic liveness status.

```json
{"status":"UP"}
```

## Web UI

The protected OrbitPlus UI is available at `GET /orbitplus/`. Set `ORBITPLUS_UI_ACCESS_TOKEN` to enable its access-token login; the token is never stored in the repository. The Queue Jobs and Tables APIs are available only through the authenticated UI session.

The Queue Jobs report loads up to 100 current `queue_metrix` records and provides client-side status tabs. It is read-only; server-side filtering and report-specific Cassandra indexes will be added only when those filters are required.

The Tables hub at `GET /orbitplus/tables` links to read-only periodic refresh routes, route metadata, schedule metadata, and the Queue Jobs report. Its authenticated API endpoints are:

| Endpoint | Required query parameters | Description |
|---|---|---|
| `GET /orbitplus/api/tables/periodic-refresh-routes` | None | Lists active and inactive periodic refresh routes by reading each Cassandra boolean partition. |
| `GET /orbitplus/api/tables/route-metadata` | `operator`, `travel`, `from`, `to` | Lists metadata for one complete route partition. |
| `GET /orbitplus/api/tables/schedule-metadata` | `operator`, `schedule`, `travel` | Lists metadata for one complete schedule partition. |

All Tables pages and APIs are read-only. If Cassandra persistence is not configured, they return a service-unavailable response.

## Docker

```powershell
docker compose up --build
```

Builds and runs orbitplus, listening on port 8082.

## Target architecture

```text
Orbit application
    ↓ (read: search / busmap / tripdetails / station)
orbitplus
    ↑ (write: Worker submission)
orbitplusworker → Bits Service → orbitplus
```

Future phases will add TripDetails processing, cache/storage, Orbit-facing query APIs, duplicate/stale detection, and Worker authentication. See [specs/orbitplus/](specs/orbitplus/) for details.

## Orionmax inventory event publishing

`POST /api/orionmax/inventory/events?activity_type=...` records every Orionmax `data[]` item in Cassandra table `queue_metrix`, then publishes one Worker job per item to the configured RabbitMQ exchange with routing key `tripdetails.refresh`. Set both `RABBITMQ_URL` and `RABBITMQ_EXCHANGE`; the exchange must already be bound to `orbitplus.tripdetails.refresh.worker` for that routing key.

Each Worker job includes `referenceId`, sourced from Orionmax `data[].refid`. The Worker must return the same `referenceId` at the root of its `POST /api/tripdetails` envelope. This transitions the matching metric row from `QUEUED` to `COMPLETED` after TripDetails storage succeeds. Failed job creation, broker publication, or TripDetails storage is recorded as `DEAD`.


## Periodic route refresh

Set `PERIODIC_REFRESH_INTERVAL` to a positive Go duration to enable a single-process ticker. On each tick it reads every `is_active=true` row from Cassandra `periodic_refresh_routes` (including routes with `ticket_count=0`), records the normal `queue_metrix` lifecycle, and publishes a `searchbusmap` Worker job with a new `referenceId`. Periodic RabbitMQ priorities are based on ticket count: 0 or less = 1, 1–5 = 2, 6–20 = 4, 21–50 = 6, 51–100 = 8, and 101+ = 9; Orionmax events remain priority 10.

This scheduler prevents overlapping runs only within one service process. In multi-instance deployments, configure the interval on exactly one designated scheduler leader; distributed leader election is not implemented.
