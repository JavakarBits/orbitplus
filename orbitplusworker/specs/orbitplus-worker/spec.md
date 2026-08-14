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

Given an eligible RabbitMQ message, the Worker validates its action-specific fields, constructs temporary approved development Bits credentials directly in Worker code from `operatorCode`, fetches raw Bits JSON, submits that exact body through the existing external OrbitPlus client, and acknowledges only an eligible terminal response.

**Acceptance scenarios**

1. Given `actionType` is `search` with valid `operatorCode`, `fromCode`, `toCode`, and `tripDate`, when processed, the Worker GETs the documented `search` route and submits the successful raw response unchanged.
2. Given `actionType` is `busmap` with valid `operatorCode`, `tripCode`, `fromStationCode`, `toStationCode`, and `travelDate`, when processed, the Worker GETs the documented `busmap` route and submits the successful raw response unchanged.
3. Given `actionType` is `searchbusmap` with valid `operatorCode`, `fromCode`, `toCode`, and `tripDate`, when processed, the Worker GETs the documented `searchbusmap` route and submits the successful raw response unchanged.
4. Given the external destination reports `ACCEPTED`, `DUPLICATE`, or `STALE`, when submission completes, the Worker ACKs the delivery.
5. Given any other result, when processing ends, the Worker leaves the delivery unacknowledged.

### User Story 2 — Preserve safe delivery behavior (Priority: P1)

The Worker validates messages before calling Bits; raw Bits JSON is logged only after a successful fetch. It never logs credentials, credential objects, credential-bearing request URLs, passwords, headers, or secrets.

**Acceptance scenarios**

1. Given a missing required field or unsupported action, the Worker makes no Bits or OrbitPlus request and leaves the delivery unacknowledged.
2. Given a Bits failure, an OrbitPlus error or retryable result, or an ACK error, the Worker leaves the delivery unacknowledged for the existing RabbitMQ redelivery/DLQ behavior.
3. Given dynamic message values are placed in a Bits route, every dynamic path segment is safely escaped.

## Requirements

- **FR-001**: A message MUST contain `operatorCode` and `actionType`.
- **FR-002**: `search` and `searchbusmap` messages MUST also contain `fromCode`, `toCode`, and `tripDate`.
- **FR-003**: `busmap` messages MUST also contain `tripCode`, `fromStationCode`, `toStationCode`, and `travelDate`.
- **FR-004**: The Worker MUST construct the temporary approved development username and token directly from the message operator code. No credential-service API, client, or contract is specified.
- **FR-005**: The Worker MUST call Bits using the action routes in the plan, with safe escaping for all dynamic path segments.
- **FR-006**: After a successful Bits fetch, the Worker MUST log the raw Bits JSON and submit it unmodified through the existing external OrbitPlus client in its existing `orbitResponse` field.
- **FR-007**: The client MUST POST to `{ORBITPLUS_URL}/v1/trip-details/refreshes`.
- **FR-008**: The Worker MUST ACK only after `ACCEPTED`, `DUPLICATE`, or `STALE`.
- **FR-009**: Invalid messages, Bits errors, OrbitPlus errors or retryable responses, and ACK errors MUST remain unacknowledged for existing RabbitMQ redelivery/DLQ behavior.
- **FR-010**: `WORKER_CONCURRENCY` MUST bound local fetch/submit operations in one Worker process; it is distinct from `RABBITMQ_PREFETCH`, which bounds unacknowledged deliveries available to a consumer channel.
- **FR-011**: Logs MUST exclude credentials, credential objects, credential-bearing request URLs, passwords, headers, and secrets.

## Configuration

The Worker configuration policy contains `APP_ENV`, RabbitMQ settings, `BITS_BASE_URL`, `ORBITPLUS_URL`, `WORKER_CONCURRENCY`, optional `WORKER_HTTP_TIMEOUT`, and optional Health API settings. It does not include `ORBIT_USERNAME`, `ORBIT_API_TOKEN`, `ORBIT_ZONE_URL`, or `WORKER_OPERATION_TIMEOUT`.

## Out of Scope

Scheduler and publishers; Master implementation, storage, and query APIs; Dragonfly; credential service; and V1 are out of scope. The Worker does not define retry policies, retry queues, dead-letter configuration, distributed coordination, or external destination internals.

## Success Criteria

- Every eligible action follows its documented Bits route and submits the successful raw Bits response unchanged.
- ACK occurs only for `ACCEPTED`, `DUPLICATE`, or `STALE` after successful submission.
- Invalid and failed processing paths are left unacknowledged.
- Worker documentation contains no scheduler, Master-internal, Dragonfly, V1, persistence, query, freshness, deduplication, rate-limit, or security requirements.
