# Implementation Plan: Proactive TripDetails Refresh Worker

**Feature**: `orbitplus-worker` | **Spec**: [spec.md](./spec.md)

## Summary

This is a Worker-only plan. For each RabbitMQ delivery, `orbitplusworker` validates the message, fetches Bits data, forwards the unmodified successful Bits body through the existing external OrbitPlus client, and then acknowledges only eligible terminal destination outcomes.

```text
RabbitMQ → orbitplusworker → Bits Service → external OrbitPlus destination → RabbitMQ ACK
```

## Message Contract

All messages require `operatorCode` and `actionType`.

| `actionType` | Additional required fields |
|---|---|
| `search` | `fromCode`, `toCode`, `tripDate` |
| `searchbusmap` | `fromCode`, `toCode`, `tripDate` |
| `busmap` | `tripCode`, `fromStationCode`, `toStationCode`, `travelDate` |

Unsupported actions or messages missing required fields are invalid. They are not sent to Bits or the external destination and remain unacknowledged.

## Bits Fetch

The Worker constructs the temporary approved development username and token directly in Worker code from the delivery's `operatorCode`. This plan intentionally defines no credential-service API, client, or contract.

All dynamic route segments are safely path-escaped. The action routes are:

| Action | GET route |
|---|---|
| `search` | `{BITS_BASE_URL}/busservices/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/{fromCode}/{toCode}/{tripDate}` |
| `busmap` | `{BITS_BASE_URL}/busservices/api/3.0/json/{operatorCode}/{username}/{apiToken}/busmap/{tripCode}/{fromStationCode}/{toStationCode}/{travelDate}` |
| `searchbusmap` | `{BITS_BASE_URL}/busservices/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/busmap/{fromCode}/{toCode}/{tripDate}` |

After a successful fetch only, the Worker logs raw Bits JSON. It must not log credentials, credential objects, credential-bearing request URLs, passwords, headers, or secrets.

## External Submission and Acknowledgement

The Worker sends the successful raw Bits body unmodified through the existing external OrbitPlus client. The client POSTs to `{ORBITPLUS_URL}/v1/trip-details/refreshes`, using its existing `orbitResponse` field.

| Result | Delivery behavior |
|---|---|
| `ACCEPTED`, `DUPLICATE`, or `STALE` | ACK after that outcome is received |
| Invalid message; Bits error; OrbitPlus error or retryable response; ACK error | Leave unacknowledged for existing RabbitMQ redelivery/DLQ behavior |

No Worker retry policy, retry queue, dead-letter configuration, scheduler behavior, distributed coordination, or destination response internals are introduced by this plan.

## Runtime Configuration and Concurrency

Documented configuration is `APP_ENV`, RabbitMQ settings, `BITS_BASE_URL`, `ORBITPLUS_URL`, `WORKER_CONCURRENCY`, optional `WORKER_HTTP_TIMEOUT`, and optional Health API settings. Legacy `ORBIT_USERNAME`, `ORBIT_API_TOKEN`, `ORBIT_ZONE_URL`, and `WORKER_OPERATION_TIMEOUT` are not part of this configuration policy.

`WORKER_CONCURRENCY` limits simultaneous local fetch/submit operations in one Worker process. `RABBITMQ_PREFETCH` separately limits unacknowledged deliveries supplied to a RabbitMQ consumer channel; it is not a concurrency setting.

## Boundaries

Out of scope: scheduler and publishers; Master implementation, storage, and query APIs; Dragonfly; credential service; and V1. No persistence, query, freshness, deduplication, rate-limit, or security requirement is assigned to the Worker or the external destination.
