# orbitplusmaster

Phase 1 of the OrbitPlus Master service.

## Scope (Phase 1)

This is the initial, minimal version of orbitplusmaster. It only:

- Accepts the raw Bits TripDetails JSON payload sent by the Worker at
  `POST /api/tripdetails`, validates it is well-formed JSON, logs the full
  received JSON to the terminal, and returns a success response.
- Exposes `GET /health` for basic liveness checks.

It intentionally does **not** implement (these are future phases):
Dragonfly caching, database persistence, TripDetails splitting/metadata/fare/
seat storage, a query API, freshness/duplicate/stale detection, RabbitMQ,
a credential service, Worker-Master authentication, retry logic, or other
business rules.

## Running locally

```powershell
$env:APP_ENV="development"
$env:MASTER_API_PORT="8082"
go run ./cmd/orbitplusmaster
```

## Environment variables

| Variable          | Required | Default       | Description                          |
|--------------------|----------|---------------|---------------------------------------|
| `APP_ENV`          | no       | `development` | Deployment environment label.         |
| `MASTER_API_PORT`  | no       | `8082`         | Port the HTTP server listens on.      |

## Endpoints

### `POST /api/tripdetails`

Accepts the complete raw Bits TripDetails JSON response as-is (no fields are
dropped or reshaped). Logs the full received JSON, then returns success.

**Example request:**

```bash
curl -X POST http://localhost:8082/api/tripdetails \
  -H "Content-Type: application/json" \
  -d '{
    "status": 1,
    "datetime": "2026-08-13 15:20:45",
    "data": [
      {
        "tripCode": "2N38731S260820D",
        "tripStageCode": "2N38731S260820D2T1",
        "travelDate": "2026-08-20",
        "tripDate": "2026-08-20",
        "displayName": "NA",
        "stageFare": [
          { "fare": 2999, "seatType": "LSL", "seatName": "Lower Sleeper", "availableSeatCount": 17 },
          { "fare": 2999, "seatType": "USL", "seatName": "Upper Sleeper", "availableSeatCount": 17 }
        ],
        "travelTime": "11 : 15",
        "closeTime": "2026-08-20 22:00:00",
        "bus": { "code": "BUS288D137E", "busType": "2+1 A/C Sleeper", "categoryCode": "LT03|CC01|CS99|MK99|ST03", "displayName": "2+1 SLEEPER AC", "name": "2+1 SLEEPER AC", "totalSeatCount": 36 },
        "schedule": { "code": "SCH4B8E31H", "serviceNumber": "Mad TO Chen" },
        "fromStation": { "name": "Madurai", "code": "STF17D52" },
        "toStation": { "name": "Chennai", "code": "STF17D51" },
        "tripStatus": { "code": "TPO", "name": "Trip Open" },
        "operator": { "code": "bits", "name": "BITS Admin" }
      }
    ]
  }'
```

**Example success response (`200 OK`):**

```json
{"status":1,"message":"Trip details received successfully"}
```

**Invalid JSON response (`400 Bad Request`):**

```json
{"status":0,"message":"Invalid trip details JSON"}
```

### `GET /health`

Returns basic liveness status.

```bash
curl http://localhost:8082/health
```

```json
{"status":"UP"}
```

## Docker

```powershell
docker compose up --build
```

This builds and runs only `orbitplusmaster`, listening on port 8082.
