# orbitplusworker

`orbitplusworker` is the standalone proactive TripDetails Worker module.

## Layout

- `cmd/orbitplusworker`: production composition root.
- `internal/application/worker`: direct Worker flow, configuration, result types, and dependency contracts.
- `internal/infrastructure/bits`: authorized `BitsTripDetailsClient` source adapter.
- `internal/infrastructure/rabbitmq`: durable manual-ack consumer and delivery adapter.
- `internal/infrastructure/orbitplus`: existing OrbitPlus/Master destination submission adapter.

## Execution boundary

The worker preserves the RabbitMQ message's existing action-specific fields, builds temporary direct Bits credentials from its operator code, and calls `BitsTripDetailsClient.FetchTripDetails` using the environment-provided `BITS_BASE_URL`. `search`, `busmap`, and `searchbusmap` GET routes retain safe escaping for every dynamic path segment. The worker passes the raw Bits response to the existing OrbitPlus-named destination contract; it does not define or fabricate a Master API.

After a successful Bits response is received, the worker writes the raw response body through the existing structured `slog` approach. Source completion logs do not include credentials or the complete credential-bearing request URL. Temporary direct credential construction remains marked with a TODO for the future credential API.

Only existing destination statuses `ACCEPTED`, `DUPLICATE`, and `STALE` invoke `RabbitMQDelivery.Ack`. `RETRYABLE`, source failures, destination failures, acknowledgement failures, and all other nonterminal outcomes remain unacknowledged for existing RabbitMQ retry, redelivery, and DLQ behavior. Manual acknowledgement, prefetch, readiness, shutdown, and context propagation are unchanged.

`WORKER_CONCURRENCY` (default `10`) caps simultaneous local fetch/submit operations in one Worker process. `RABBITMQ_PREFETCH` (default `10`) independently caps unacknowledged deliveries sent to one RabbitMQ consumer channel.

## Environment and startup

`APP_ENV` must be exactly `development` or `production`; it defaults to `production` when unset. Production accepts only `amqps://` RabbitMQ, `https://` Bits, and `https://` OrbitPlus URLs. Development additionally permits `amqp://` RabbitMQ and `http://` Bits and OrbitPlus URLs. URLs are supplied through configuration or environment variables—no hostname is hardcoded in Go source.

Start the standalone Worker from the module root with:

```powershell
go run ./cmd/orbitplusworker
```

`go run .` intentionally fails because the module root has no `main` package.

## Verification

```powershell
gofmt -w cmd/orbitplusworker/main.go internal/application/worker/bits.go internal/application/worker/config.go internal/application/worker/orbitplus.go internal/application/worker/worker.go internal/infrastructure/bits/tripdetails_client.go internal/infrastructure/orbitplus/client.go
go build ./...
go vet ./...
```
