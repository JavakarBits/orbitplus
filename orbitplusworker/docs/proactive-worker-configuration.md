# TripDetails Refresh Worker configuration

`cmd/orbitplusworker` calls `worker.LoadRuntimeConfig`. It reads optional, non-secret JSON from `TRIPDETAILS_REFRESH_WORKER_CONFIG_FILE`, then applies environment overrides. Unknown JSON fields fail startup. Values are not logged.

## Environment and transport policy

`APP_ENV` is required to be exactly `development` or `production`. If it is unset, the Worker defaults to `production`; an empty or other value fails startup without printing configuration values.

| APP_ENV | RabbitMQ URL | Bits base URL | OrbitPlus URL |
|---|---|---|---|
| `production` | `amqps://` only | `https://` only | `https://` only |
| `development` | `amqps://` or `amqp://` | `https://` or `http://` | `https://` or `http://` |

The runtime configuration and the RabbitMQ, Bits, and OrbitPlus constructors apply this policy. URL hostnames remain environment-provided; no hostname is embedded in Go source.

## Required settings

| Area | Settings |
|---|---|
| RabbitMQ | `RABBITMQ_URL`, `RABBITMQ_QUEUE`, `RABBITMQ_EXCHANGE`, `RABBITMQ_ROUTING_KEY`, and `RABBITMQ_PREFETCH` (default `10`), plus paired `RABBITMQ_USERNAME`/`RABBITMQ_PASSWORD` or their `*_FILE` forms. |
| Bits | `BITS_BASE_URL` and optional JSON TLS file paths. Temporary direct username/token construction remains in the Worker until the credential API exists. |
| OrbitPlus/Master destination | `ORBITPLUS_URL`, optional `ORBITPLUS_BEARER_TOKEN` or `_FILE`, JSON response size, and JSON TLS file paths. |
| Worker | `WORKER_CONCURRENCY` (default `10`) and `WORKER_HTTP_TIMEOUT`. |

`WORKER_CONCURRENCY` bounds simultaneous local Worker fetch/submit operations in one process only. `RABBITMQ_PREFETCH` independently bounds unacknowledged deliveries RabbitMQ may provide to one consumer channel.

## Direct flow and logging

Each delivery retains its action-specific fields, constructs temporary Bits credentials from the message operator code, and issues an escaped GET under `BITS_BASE_URL`:

- `search`: `/busservices/api/3.0/json/{operator}/{username}/{token}/search/{fromCode}/{toCode}/{tripDate}`
- `busmap`: `/busservices/api/3.0/json/{operator}/{username}/{token}/busmap/{tripCode}/{fromStationCode}/{toStationCode}/{travelDate}`
- `searchbusmap`: `/busservices/api/3.0/json/{operator}/{username}/{token}/search/busmap/{fromCode}/{toCode}/{tripDate}`

Every dynamic path segment is path-escaped. The worker logs the raw response body with `slog.Info` only after a successful Bits GET; request-completion logs include action, operator, and outcome only, never credentials or the credential-bearing request URL.

The raw Bits body is passed to the existing OrbitPlus-named destination contract. That client still posts to its established `/v1/trip-details/refreshes` endpoint and preserves its existing `orbitResponse` payload field; this is the active Master destination contract, not a new Master API. Only `ACCEPTED`, `DUPLICATE`, and `STALE` invoke `RabbitMQDelivery.Ack`. All source, destination, acknowledgement, retryable, or nonterminal failures remain unacknowledged for the existing RabbitMQ retry/redelivery/DLQ behavior.

## Startup

Prepare the required local environment values and running dependencies, then run from the module root:

```powershell
go run ./cmd/orbitplusworker
```

`go run .` intentionally fails because the module root has no `main` package.
