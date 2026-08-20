# orbitplus

orbitplus is the TripDetails microservice in the OrbitPlus system. It receives TripDetails data from orbitplusworker and is responsible for processing, caching, storing, and serving TripDetails to the Orbit application.

## Current scope

Phase 1 provides Worker ingestion only:

- Accepts the Worker submission at `POST /api/tripdetails`, validates JSON syntax, logs the raw payload, and returns a success response.
- Exposes `GET /health` for liveness checks.
- Reconstructs persisted Search and BusMap responses from Cassandra metadata and Dragonfly; it does not call an upstream TripDetails API.

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
    "data": []
  }
}
```

- `actionType`: `search`, `busmap`, or `searchbusmap`.
- `operatorCode`: identifies the operator.
- `orbitResponse`: the raw Bits JSON response forwarded by the Worker.

**Success response (HTTP 200):**

```json
{"status":1,"message":"Trip details received successfully"}
```

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

## Endpoints

### `POST /api/tripdetails`

Accepts the Worker envelope, validates that the body is syntactically valid JSON, logs the raw payload, and returns the success response.

### `GET /health`

Returns basic liveness status:

```json
{"status":"UP"}
```

## Web UI

The protected OrbitPlus UI is available at `GET /orbitplus/`. Set `ORBITPLUS_UI_ACCESS_TOKEN` to enable its access-token login; the token is never stored in the repository.

The Queue Jobs report is one read-only page with All Jobs, Received, Queued, Completed, and Dead filters. It queries status server-side and provides Previous/Next pagination through `GET /orbitplus/api/reports/queue-jobs`. The endpoint accepts optional `status`, `limit` (default `25`, maximum `100`), and opaque `page` parameters.

The Tables hub provides read-only route and schedule metadata lookups:

| Endpoint | Required query parameters | Description |
|---|---|---|
| `GET /orbitplus/api/tables/route-metadata` | `operator`, `travel`, `from`, `to` | Lists metadata for one complete route partition. |
| `GET /orbitplus/api/tables/schedule-metadata` | `operator`, `schedule`, `travel` | Lists metadata for one complete schedule partition. |

## Docker

```powershell
docker compose up --build
```

Builds and runs orbitplus, listening on port 8082.

## Orionmax inventory event publishing

`POST /api/orionmax/inventory/events?activity_type=...` records every Orionmax `data[]` item in Cassandra table `queue_metrix`, then publishes one Worker job per item to the configured RabbitMQ exchange with routing key `tripdetails.refresh`.

Each Worker job includes `referenceId`, sourced from Orionmax `data[].refid`. The Worker must return the same `referenceId` at the root of its `POST /api/tripdetails` envelope. This transitions the matching metric row from `QUEUED` to `COMPLETED` after TripDetails storage succeeds. Failed job creation, broker publication, or TripDetails storage is recorded as `DEAD`.

## Route refresh

The Cassandra-backed `periodic_refresh_routes` flow is removed. Periodic route retrieval and queueing are disabled until the Orbit route API response contract is available. The future implementation will obtain route details directly from that API rather than storing them in `periodic_refresh_routes`.
