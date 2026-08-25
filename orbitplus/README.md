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

A local `.env` file in the repository root is loaded automatically by `go run ./cmd/orbitplusmaster`. It is ignored by Git. Process environment variables take precedence over `.env`, so production services should use their platform's secret manager or environment configuration.

```powershell
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

Periodic route refresh is disabled unless persistence, RabbitMQ, the Orbit route settings below, and a stale duration are configured. It never creates or uses a `periodic_refresh_routes` table and never runs schema changes at runtime.

The scheduler does not run immediately on startup and permits only one run at a time. On each interval it reads active operators and their supported `zone_code` values from Cassandra, then fetches each operator's Orbit route response. It reads only persisted route pairs whose latest metadata update is at least the configured stale duration old, considers trip dates from today through seven days ahead inclusive, and deduplicates `(operatorCode, tripDate, fromCode, toCode)` pairs by latest `updated_at`.

Only a stale persisted pair that appears in the operator's current Orbit route response is queued. A destination in `topRoute` queues at priority 9; a destination in the normal `route` queues at priority 8. The Worker job is `searchbusmap` and contains `referenceId`, `operatorCode`, `zoneURL`, `fromCode`, `toCode`, and `tripDate`; no trip code is invented. Orionmax inventory jobs remain priority 10. The deterministic periodic reference ID includes the metadata update time, preventing re-publication of QUEUED or COMPLETED jobs for the same metadata version while allowing a newly updated route to become eligible again after it becomes stale.

| Variable | Required to enable | Description |
|---|---|---|
| `ORBIT_ROUTE_BASE_URL` | Yes | HTTPS/HTTP Orbit host. Master adds the operator-specific `/orbitservices/ezeeinfo/{operatorCode}/{accessToken}/top/route` path without logging it. |
| `ORBIT_ROUTE_ACCESS_TOKEN` | Yes | Shared Orbit access token used for every active operator. The operator's supported `zone_code` is stored in Cassandra and resolved to its Worker URL at runtime. |
| `ORBIT_ROUTE_TIMEOUT` | Yes | Positive Go duration for the standard-library HTTP client, for example `5s`. |
| `ORBIT_ROUTE_REFRESH_INTERVAL` | Yes | Positive scheduler interval, for example `1m` for local testing or `15m` in production. |
| `ORBIT_ROUTE_STALE_DURATION` | Yes | How long persisted route metadata can remain unchanged before it is eligible, for example `1m` for testing or `1h` in production. |

Keep `ORBIT_ROUTE_ACCESS_TOKEN` in a secret manager or deployment environment. It is neither logged nor stored in Cassandra, RabbitMQ payloads, CQL, or this repository.
