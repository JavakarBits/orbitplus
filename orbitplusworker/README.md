# orbitplusworker

`orbitplusworker` is the standalone proactive TripDetails Worker module.

## Execution boundary

```text
RabbitMQ → orbitplusworker → Bits Service → external OrbitPlus destination → RabbitMQ ACK
```

The Worker does not implement a scheduler or publisher, Master storage/query behavior, Dragonfly, a credential service, or V1 behavior.

Each RabbitMQ message requires `operatorCode` and `actionType`. `search` and `searchbusmap` require `fromCode`, `toCode`, and `tripDate`; `busmap` requires `tripCode`, `fromStationCode`, `toStationCode`, and `travelDate`.

The Worker builds temporary approved development Bits username/token values directly in Worker code from `operatorCode`, safely path-escapes every dynamic route segment, and fetches the matching action route under `BITS_BASE_URL`. After a successful fetch, it logs the raw Bits JSON and passes that raw body unchanged through the existing external OrbitPlus client. The client POSTs to `{ORBITPLUS_URL}/v1/trip-details/refreshes` in its existing `orbitResponse` field.

Credentials, credential objects, credential-bearing request URLs, passwords, headers, and secrets are never logged. Only destination outcomes `ACCEPTED`, `DUPLICATE`, and `STALE` cause `RabbitMQDelivery.Ack`. Invalid messages, Bits errors, OrbitPlus errors/retryable responses, and ACK errors remain unacknowledged for existing RabbitMQ redelivery/DLQ behavior.

`WORKER_CONCURRENCY` limits local fetch/submit operations in one Worker process. `RABBITMQ_PREFETCH` independently limits unacknowledged deliveries sent to a consumer channel.

## Configuration and startup

The configuration policy includes `APP_ENV`, RabbitMQ settings, `BITS_BASE_URL`, `ORBITPLUS_URL`, `WORKER_CONCURRENCY`, optional `WORKER_HTTP_TIMEOUT`, and optional Health API settings. It excludes legacy `ORBIT_USERNAME`, `ORBIT_API_TOKEN`, `ORBIT_ZONE_URL`, and `WORKER_OPERATION_TIMEOUT`.

Start the standalone Worker from the module root:

```powershell
go run ./cmd/orbitplusworker
```

`go run .` intentionally fails because the module root has no `main` package.

See [proactive-worker-configuration.md](docs/proactive-worker-configuration.md) for the Worker-only configuration and route contract.
