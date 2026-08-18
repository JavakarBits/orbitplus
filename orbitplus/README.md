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

## Endpoints

### `POST /api/tripdetails`

Accepts the Worker envelope. Validates that the body is syntactically valid JSON, logs the raw payload, and returns the success response.

### `GET /health`

Returns basic liveness status.

```json
{"status":"UP"}
```

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
