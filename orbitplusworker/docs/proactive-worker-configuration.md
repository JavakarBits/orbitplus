# Proactive TripDetails Worker configuration

This document covers the standalone Worker boundary only:

```text
RabbitMQ → orbitplusworker → Bits Service → external OrbitPlus destination → RabbitMQ ACK
```

## Configuration policy

The Worker configuration policy includes:

- `APP_ENV`
- RabbitMQ settings, including `RABBITMQ_PREFETCH`
- `BITS_BASE_URL`
- `ORBITPLUS_URL`
- `WORKER_CONCURRENCY`
- Optional `WORKER_HTTP_TIMEOUT`
- Optional Health API settings

Legacy `ORBIT_USERNAME`, `ORBIT_API_TOKEN`, `ORBIT_ZONE_URL`, and `WORKER_OPERATION_TIMEOUT` are not part of the Worker configuration.

`WORKER_CONCURRENCY` limits simultaneous local fetch/submit operations in one Worker process. `RABBITMQ_PREFETCH` is separate: it limits unacknowledged deliveries RabbitMQ may make available to a consumer channel.

## Delivery configuration behavior

Every message requires `operatorCode` and `actionType`. `search` and `searchbusmap` additionally require `fromCode`, `toCode`, and `tripDate`; `busmap` additionally requires `tripCode`, `fromStationCode`, `toStationCode`, and `travelDate`.

For a valid message, the Worker constructs temporary approved development username/token values directly in Worker code from `operatorCode`. No credential-service API, client, or contract is configured or specified.

The Worker safely escapes every dynamic Bits path segment and calls one action route:

| Action | GET route |
|---|---|
| `search` | `{BITS_BASE_URL}/busservices/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/{fromCode}/{toCode}/{tripDate}` |
| `busmap` | `{BITS_BASE_URL}/busservices/api/3.0/json/{operatorCode}/{username}/{apiToken}/busmap/{tripCode}/{fromStationCode}/{toStationCode}/{travelDate}` |
| `searchbusmap` | `{BITS_BASE_URL}/busservices/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/busmap/{fromCode}/{toCode}/{tripDate}` |

Only after a successful Bits fetch, the Worker logs the raw Bits JSON. It then submits that raw body unmodified through the existing external OrbitPlus client to `POST {ORBITPLUS_URL}/v1/trip-details/refreshes`, using the existing `orbitResponse` field.

The Worker never logs credentials, credential objects, request URLs containing credentials, passwords, headers, or secrets. It ACKs only `ACCEPTED`, `DUPLICATE`, or `STALE`. Invalid messages, Bits errors, OrbitPlus errors or retryable responses, and ACK errors remain unacknowledged for existing RabbitMQ redelivery/DLQ behavior.

## Scope boundary

This document does not configure or define scheduler/publisher behavior, Master implementation/storage/query APIs, Dragonfly, credential service, V1, Worker retry policy, retry queues, dead-letter configuration, distributed coordination, or external destination internals.
