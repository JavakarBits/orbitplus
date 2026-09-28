# Implementation Plan: Proactive TripDetails Refresh Worker

**Feature**: `orbitplus-worker` | **Spec**: [spec.md](./spec.md)

## Summary

This is a Worker-only plan. For each RabbitMQ delivery, `orbitplusworker` validates the message, fetches Bits data, forwards the unmodified successful Bits body through the existing external OrbitPlus client, and then acknowledges only when the destination reports a successful outcome.

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

The Worker uses temporary hardcoded development constants for the Bits username and API token. The `operatorCode` from the delivery message is used as the operator path segment and is validated, but it does not derive the username or API token. These credentials are intended to be replaced by a future approved credential mechanism. This plan intentionally defines no credential-service API, client, or contract.

All dynamic route segments are safely path-escaped. The action routes are:

| Action | GET route |
|---|---|
| `search` | `{BITS_BASE_URL}/busservices/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/{fromCode}/{toCode}/{tripDate}` |
| `busmap` | `{BITS_BASE_URL}/busservices/api/3.0/json/{operatorCode}/{username}/{apiToken}/busmap/{tripCode}/{fromStationCode}/{toStationCode}/{travelDate}` |
| `searchbusmap` | `{BITS_BASE_URL}/busservices/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/busmap/{fromCode}/{toCode}/{tripDate}` |

After a successful fetch only, the Worker logs raw Bits JSON. It must not log credentials, credential objects, credential-bearing request URLs, passwords, headers, or secrets.

## External Submission and Acknowledgement

The Worker sends the successful raw Bits body unmodified through the existing external OrbitPlus client. The client POSTs to `{ORBITPLUS_URL}/api/tripdetails`, using its existing `orbitResponse` field.

| Result | Delivery behavior |
|---|---|
| `ACCEPTED` | ACK after that outcome is received |
| Invalid message; Bits error; OrbitPlus error or retryable response; ACK error | Leave unacknowledged for existing RabbitMQ redelivery/DLQ behavior |

In the current phase, `ACCEPTED` is the only outcome the existing OrbitPlus destination produces for a successful submission (`status: 1` in the response). In a future phase, when the destination implements duplicate and stale detection, `DUPLICATE` and `STALE` are expected to also become ACK-eligible outcomes. Those outcomes are not currently producible and are outside the scope of this plan.

No Worker retry policy, retry queue, dead-letter configuration, scheduler behavior, distributed coordination, or destination response internals are introduced by this plan.

## Zone Rate Limiting

Before any Bits/credential work for a delivery, the Worker enforces a per-zone quota. A zone is the delivery's Bits zone endpoint (zone URL). The quota is a fixed window: `hits/window` (for example `10/1m`), with optional per-zone overrides. The window starts on a zone's first hit and resets when it expires; unused hits do not carry over.

Behavior when a zone is over quota: the Worker waits in place until that zone's window resets, then processes the same delivery. It does not requeue or drop the delivery. Other zones are processed independently by other goroutines, each against its own quota.

Enforcement is atomic so concurrent workers cannot together exceed a zone's quota:

- Distributed (when `DRAGONFLY_ADDRESS` is set): a single Lua script does `INCR` on the zone key and, on the first hit of a window, `PEXPIRE` for the window duration, returning the count and remaining TTL. Because the counter lives in Dragonfly, the quota holds across all goroutines and process instances.
- In-memory (when `DRAGONFLY_ADDRESS` is unset): an equivalent mutex-guarded per-zone window counter, correct within a single process only.

Failure policy: a configured cache that is unreachable at startup is a fatal, visible failure. If the limiter errors during processing, the Worker fails open (logs and allows) so a cache outage does not halt all refreshes. No quota configured means limiting is disabled.

## Runtime Configuration and Concurrency

Documented configuration is `APP_ENV`, RabbitMQ settings, `BITS_BASE_URL`, `ORBITPLUS_URL`, `WORKER_CONCURRENCY`, optional `WORKER_HTTP_TIMEOUT`, optional Health API settings, and the zone rate-limit settings (`WORKER_BITS_RATE_LIMIT`, optional `WORKER_BITS_RATE_LIMIT_OVERRIDES`, and the `DRAGONFLY_*` cache settings). Legacy `ORBIT_USERNAME`, `ORBIT_API_TOKEN`, `ORBIT_ZONE_URL`, and `WORKER_OPERATION_TIMEOUT` are not part of this configuration policy.

Worker → OrbitPlus authentication exists in the current implementation as a bearer-token mechanism. The intended architectural direction is a dedicated context token specifically for Worker → OrbitPlus communication; the token name, HTTP header, format, validation, storage, and configuration variable names are unresolved. The current bearer-token implementation is a temporary detail, not the final authentication contract.

`WORKER_CONCURRENCY` limits simultaneous local fetch/submit operations in one Worker process. `RABBITMQ_PREFETCH` separately limits unacknowledged deliveries supplied to a RabbitMQ consumer channel; it is not a concurrency setting.

## Boundaries

Out of scope: scheduler and publishers; Master implementation, storage, and query APIs; credential service; V1; duplicate detection; freshness tracking; and version comparison. No persistence, query, freshness, deduplication, or security requirement is assigned to the Worker or the external destination. Dragonfly is in scope only as the backing store for the per-zone rate-limit counter and for no other Worker concern. The dedicated Worker → OrbitPlus context-token contract details are unresolved and out of scope for this phase.
