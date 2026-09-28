# Feature Specification: Proactive TripDetails Refresh Worker

**Feature**: `orbitplus-worker`

**Status**: Draft

## Purpose

`orbitplusworker` processes one RabbitMQ delivery at a time through this boundary:

```text
RabbitMQ → orbitplusworker → Bits Service → external OrbitPlus destination → RabbitMQ ACK
```

The Worker does not schedule or publish work, implement Master behavior, or store/query TripDetails.

## Worker Processing

### User Story 1 — Process a valid refresh delivery (Priority: P1)

Given an eligible RabbitMQ message, the Worker validates its action-specific fields, constructs temporary hardcoded development Bits credentials directly in Worker code, fetches raw Bits JSON, submits that exact body through the existing external OrbitPlus client, and acknowledges only when the destination reports a successful outcome.

**Acceptance scenarios**

1. Given `actionType` is `search` with valid `operatorCode`, `fromCode`, `toCode`, and `tripDate`, when processed, the Worker GETs the documented `search` route and submits the successful raw response unchanged.
2. Given `actionType` is `busmap` with valid `operatorCode`, `tripCode`, `fromStationCode`, `toStationCode`, and `travelDate`, when processed, the Worker GETs the documented `busmap` route and submits the successful raw response unchanged.
3. Given `actionType` is `searchbusmap` with valid `operatorCode`, `fromCode`, `toCode`, and `tripDate`, when processed, the Worker GETs the documented `searchbusmap` route and submits the successful raw response unchanged.
4. Given the external destination reports `ACCEPTED`, when submission completes, the Worker ACKs the delivery.
5. Given any other result, when processing ends, the Worker leaves the delivery unacknowledged.

### User Story 2 — Preserve safe delivery behavior (Priority: P1)

The Worker validates messages before calling Bits; raw Bits JSON is logged only after a successful fetch. It never logs credentials, credential objects, credential-bearing request URLs, passwords, headers, or secrets.

**Acceptance scenarios**

1. Given a missing required field or unsupported action, the Worker makes no Bits or OrbitPlus request and leaves the delivery unacknowledged.
2. Given a Bits failure, an OrbitPlus error or retryable result, or an ACK error, the Worker leaves the delivery unacknowledged for the existing RabbitMQ redelivery/DLQ behavior.
3. Given dynamic message values are placed in a Bits route, every dynamic path segment is safely escaped.

### User Story 3 — Rate-limit Bits requests per zone (Priority: P2)

To protect Bits from overload, the Worker limits how frequently each zone is queried. A zone is identified by the delivery's Bits zone endpoint (zone URL). Each zone has a quota of a fixed number of hits per time window (for example, 10 hits per minute), with optional per-zone overrides. When a zone is within quota the Worker proceeds immediately; when a zone is over quota the Worker waits until that zone's window resets and then processes the same delivery — it does not requeue or drop it. The quota holds across all Worker goroutines, and across process instances when a shared distributed cache is configured.

**Acceptance scenarios**

1. Given a zone within its quota, when a delivery for that zone is processed, the Worker calls Bits immediately and counts the hit against the window.
2. Given a zone that has reached its quota within the current window, when a delivery for that zone is processed, the Worker waits until the window resets and then processes that same delivery without requeuing or dropping it.
3. Given one zone is over quota, when deliveries for other zones are processed, those other zones are unaffected and proceed within their own quotas.
4. Given a distributed cache is configured, when multiple goroutines or Worker instances process the same zone concurrently, the combined hits within one window do not exceed that zone's quota.
5. Given no quota is configured, when any delivery is processed, the Worker applies no zone limiting.

## Requirements

- **FR-001**: A message MUST contain `operatorCode` and `actionType`.
- **FR-002**: `search` and `searchbusmap` messages MUST also contain `fromCode`, `toCode`, and `tripDate`.
- **FR-003**: `busmap` messages MUST also contain `tripCode`, `fromStationCode`, `toStationCode`, and `travelDate`.
- **FR-004**: The Worker MUST use temporary hardcoded development constants for the Bits username and API token. The `operatorCode` from the message is used as the operator path segment and is validated, but it does not derive the username or API token. These credentials are intended to be replaced by a future approved credential mechanism.
- **FR-005**: The Worker MUST call Bits using the action routes in the plan, with safe escaping for all dynamic path segments.
- **FR-006**: After a successful Bits fetch, the Worker MUST log the raw Bits JSON and submit it unmodified through the existing external OrbitPlus client in its existing `orbitResponse` field.
- **FR-007**: The client MUST POST to `{ORBITPLUS_URL}/api/tripdetails`.
- **FR-008**: The Worker MUST ACK only after `ACCEPTED`. In the current phase, `ACCEPTED` is the only outcome the existing OrbitPlus destination produces for a successful submission.
- **FR-009**: Invalid messages, Bits errors, OrbitPlus errors or retryable responses, and ACK errors MUST remain unacknowledged for existing RabbitMQ redelivery/DLQ behavior.
- **FR-010**: `WORKER_CONCURRENCY` MUST bound local fetch/submit operations in one Worker process; it is distinct from `RABBITMQ_PREFETCH`, which bounds unacknowledged deliveries available to a consumer channel.
- **FR-011**: Logs MUST exclude credentials, credential objects, credential-bearing request URLs, passwords, headers, and secrets.
- **FR-012**: A zone MUST be identified by the delivery's Bits zone endpoint (zone URL).
- **FR-013**: Each zone MUST be limited to a configured number of hits per fixed time window (a `hits/window` quota such as `10/1m`), with optional per-zone overrides. The window starts on a zone's first hit and resets when it expires; unused hits do not carry over.
- **FR-014**: When a zone is over quota, the Worker MUST wait until that zone's window resets and then process the same delivery. It MUST NOT requeue or drop a delivery because of rate limiting.
- **FR-015**: The per-zone quota MUST hold across all goroutines in one Worker process, and across process instances when a distributed cache (Dragonfly) is configured. Enforcement MUST be atomic so concurrent workers cannot together exceed a zone's quota.
- **FR-016**: When no quota is configured, rate limiting MUST be disabled. When a distributed cache is configured but unreachable at startup, the Worker MUST fail fast; if the cache becomes unreachable during processing, the Worker MAY fail open (allow the request) so refreshes are not halted.

## Configuration

The Worker configuration policy contains `APP_ENV`, RabbitMQ settings, `BITS_BASE_URL`, `ORBITPLUS_URL`, `WORKER_CONCURRENCY`, optional `WORKER_HTTP_TIMEOUT`, and optional Health API settings. It does not include `ORBIT_USERNAME`, `ORBIT_API_TOKEN`, `ORBIT_ZONE_URL`, or `WORKER_OPERATION_TIMEOUT`.

Zone rate limiting adds these optional settings:

- `WORKER_BITS_RATE_LIMIT` — default quota for every zone in `hits/window` form (for example `10/1m`). Unset disables rate limiting.
- `WORKER_BITS_RATE_LIMIT_OVERRIDES` — optional comma-separated `zoneURL=hits/window` per-zone overrides (for example `http://app.ezeebits.com=20/1m,http://app.r2.ezeebits.com=5/30s`).
- `DRAGONFLY_ADDRESS`, `DRAGONFLY_PASSWORD`, `DRAGONFLY_DATABASE`, `DRAGONFLY_CONNECTION_TIMEOUT` — the shared distributed cache used to enforce the quota across instances. When `DRAGONFLY_ADDRESS` is unset, the Worker uses an equivalent in-memory limiter that is correct within a single process only.

Worker → OrbitPlus authentication exists in the current implementation as a bearer-token mechanism. The intended architectural direction is a dedicated context token specifically for Worker → OrbitPlus communication; the token name, HTTP header, format, validation, storage, and configuration variable names are unresolved. The current bearer-token implementation is a temporary detail, not the final authentication contract.

## Future Outcomes

In a future phase, when the OrbitPlus destination implements duplicate and stale detection, the Worker is expected to also ACK on `DUPLICATE` and `STALE` outcomes. These outcomes are not currently producible by the existing OrbitPlus destination and are outside the scope of the current phase.

## Out of Scope

Scheduler and publishers; Master implementation, storage, and query APIs; credential service; V1; duplicate detection; freshness tracking; and version comparison are out of scope. The Worker does not define retry policies, retry queues, dead-letter configuration, or external destination internals. The only cross-instance coordination the Worker performs is the per-zone rate-limit counter (User Story 3); Dragonfly is used solely as that counter's backing store and for no other Worker concern. The dedicated Worker → OrbitPlus context-token contract details are unresolved and out of scope for this phase.

## Success Criteria

- Every eligible action follows its documented Bits route and submits the successful raw Bits response unchanged.
- ACK occurs only for `ACCEPTED` after successful submission.
- Invalid and failed processing paths are left unacknowledged.
- Each zone stays within its configured `hits/window` quota; an over-quota zone causes the Worker to wait and then process the same delivery, without requeue or drop, and without affecting other zones.
- Worker documentation contains no scheduler, Master-internal, V1, persistence, query, freshness, deduplication, or security requirements.
