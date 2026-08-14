# Implementation Plan: Proactive TripDetails Refresh System

**Branch**: `001-proactive-tripdetails-refresh` | **Date**: 2026-08-12 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/001-proactive-tripdetails-refresh/spec.md`

**V1 is legacy/out-of-scope and was not used as an implementation or architectural reference.**

## Summary

Design and implement a proactive TripDetails refresh system that maintains fresh TripDetails for all trips within a 40-day forward window. The system uses two RabbitMQ queues: an Inventory Change Queue for event-driven refresh (higher urgency) and a Periodic Refresh Queue for scheduler-driven refresh (priority P1–P6). A periodic scheduler classifies routes and dates into tiers, computes priority, and publishes to the Periodic Refresh Queue. Inventory-change events flow through the Inventory Change Queue for immediate processing. Dedicated worker services consume from both queues, fetch data from Inventory/BusMap, compose TripDetails JSON, and deliver results to the orbitplusservice Master Service for persistence. The system uses Dragonfly for distributed coordination state and provides end-to-end traceability via correlation IDs.

## Technical Context

**Language/Version**: Go 1.25.0

**Primary Dependencies**: Gin 1.10.1 (HTTP framework), RabbitMQ (message broker), Dragonfly (distributed cache/state)

**Storage**: TripDetails database (accessed exclusively via orbitplusservice)

**Testing**: Go standard testing (`go test`), table-driven tests, integration tests with test containers

**Target Platform**: Linux server (containerized via Docker)

**Project Type**: Distributed service system (scheduler + worker + master service extensions)

**Performance Goals**: Process all active routes × 40 days of refresh tasks within each 10-minute scheduler cycle; P1 tasks processed within 5 minutes of scheduling; inventory-change tasks processed within 2 minutes of event receipt

**Constraints**: Workers MUST NOT access TripDetails DB directly; single-leader scheduler; horizontal worker scaling up to 10+ instances

**Scale/Scope**: Hundreds of routes × 40 travel dates = thousands of refresh tasks per cycle; multiple concurrent workers; inventory-change events arrive at variable rate

## Constitution Check

| Principle | Compliance | Notes |
|-----------|------------|-------|
| I. Proactive TripDetails Freshness | ✅ PASS | 40-day window with 10-min periodic scheduling + inventory-driven refresh; no reactive fetching |
| II. Worker-Based Processing | ✅ PASS | Dedicated RabbitMQ workers; horizontally scalable; idempotent; ack after delivery |
| III. Reliable Message Delivery | ✅ PASS | RabbitMQ with manual ack; dead-letter on exhaust; confirm before ack; two queues for isolation |
| IV. Minimal Dependencies | ✅ PASS | Only Go stdlib + Gin + RabbitMQ client + Dragonfly client; no extras |
| V. Observability & Traceability | ✅ PASS | Correlation IDs end-to-end; structured logging; metrics for all stages and both queues |

---

## 1. Overall Architecture

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│                            OrbitPlus System                                    │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                               │
│  Inventory Events                    Periodic Scheduler                       │
│       │                                     │                                 │
│       ▼                                     ▼                                 │
│  ┌───────────────────┐          ┌─────────────────────────┐                  │
│  │ Inventory Change   │          │  Periodic Refresh Queue  │                  │
│  │ Queue              │          │  (x-max-priority: 6)     │                  │
│  │ (higher urgency)   │          │  P1-P6 priority ordering │                  │
│  └─────────┬─────────┘          └────────────┬────────────┘                  │
│            │                                  │                               │
│            └──────────────┬───────────────────┘                               │
│                           │                                                   │
│                           ▼                                                   │
│                  ┌──────────────────┐                                         │
│                  │  Worker Service   │                                         │
│                  │  (N instances)    │                                         │
│                  │  consumes both    │                                         │
│                  └────────┬─────────┘                                         │
│                           │                                                   │
│              ┌────────────┼────────────┐                                      │
│              │            │            │                                       │
│              ▼            ▼            ▼                                       │
│    ┌──────────────┐ ┌──────────┐ ┌──────────────────┐                        │
│    │  Dragonfly   │ │Inventory/│ │ orbitplusservice  │                        │
│    │  (state,     │ │ BusMap   │ │ (Master Service)  │                        │
│    │   dedup,     │ │(ext API) │ │  - Push API       │                        │
│    │   locks,     │ │          │ │  - Failure API    │                        │
│    │   creds)     │ │          │ │  - Rate Limiter   │                        │
│    └──────────────┘ └──────────┘ └────────┬─────────┘                        │
│                                           │ persists                          │
│                                           ▼                                   │
│                                  ┌──────────────────┐                         │
│                                  │  TripDetails DB   │                         │
│                                  └──────────────────┘                         │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Two-Queue Architecture Rationale

The system uses two separate RabbitMQ queues to provide **isolation** between inventory-driven and periodic refresh work:

1. **Inventory Change Queue** — Event-driven, higher urgency. Ensures inventory changes trigger immediate TripDetails refresh regardless of periodic queue backlog.
2. **Periodic Refresh Queue** — Scheduler-driven, priority-ordered (P1–P6). Handles the bulk 40-day window maintenance work.

This isolation guarantees that periodic task volume (potentially thousands per cycle) cannot delay or block inventory-change processing.

### Responsibility Separation

| Layer | Responsibility | Components |
|-------|---------------|------------|
| **Domain** | Priority calculation, tier classification, task models, validation rules, version comparison logic | Pure business logic, no I/O |
| **Application** | Orchestration of scheduler cycles, worker pipelines, retry policies, correlation ID propagation, dual-queue consumption | Coordinates domain + infrastructure |
| **Infrastructure** | RabbitMQ publishing/consuming (both queues), Dragonfly operations, HTTP calls to Inventory/BusMap, HTTP calls to orbitplusservice, structured logging output | All external I/O |

### Data Flow

**Periodic Path**:
1. **Scheduler** reads top_route data and generates travel dates (today + 40 days)
2. **Scheduler** classifies routes (HOT/WARM/COLD) and dates (HOT/WARM/COLD), computes priority matrix
3. **Scheduler** checks Dragonfly for existing tasks (deduplication), publishes new tasks to **Periodic Refresh Queue**
4. **Worker** consumes task from Periodic Refresh Queue

**Inventory-Change Path**:
1. **Inventory System** publishes change event to **Inventory Change Queue**
2. **Worker** consumes task from Inventory Change Queue (higher urgency, no P1–P6 priority)

**Common Path** (after consumption from either queue):
3. **Worker** obtains credentials from Dragonfly/Orbit
4. **Worker** calls Inventory/BusMap external API to fetch latest data
5. **Worker** composes TripDetails JSON payload
6. **Worker** pushes TripDetails to orbitplusservice Push API
7. **Worker** acknowledges RabbitMQ message only after successful push
8. **orbitplusservice** validates, checks version/freshness, persists to TripDetails DB

---

## 2. Periodic Scheduler

### Execution Cycle

The scheduler runs every 10 minutes as a single-leader process. Each cycle:

1. **Load route data**: Fetch active routes with booking volume from the `top_route` data source
2. **Generate date range**: Produce all calendar dates from today to today+40
3. **Classify routes**: Assign each route a tier (HOT/WARM/COLD) based on configurable volume thresholds
4. **Classify dates**: Assign each date a tier (HOT/WARM/COLD) based on configurable day-proximity thresholds
5. **Compute priority matrix**: For each route × date combination, determine P1–P6
6. **Deduplicate**: For each candidate task, check Dragonfly for an existing active task with the same route+date key
7. **Publish**: Publish non-duplicate tasks to the **Periodic Refresh Queue** with the computed priority
8. **Record metadata**: Update scheduler cycle metadata in Dragonfly (last run time, tasks published count)

### Route Tier Classification

| Tier | Criteria (configurable) | Default Threshold |
|------|------------------------|-------------------|
| HOT | Booking volume ≥ high threshold | Top 20% by volume |
| WARM | Booking volume between low and high | Middle 50% |
| COLD | Booking volume < low threshold | Bottom 30% |

### Date Tier Classification

| Tier | Criteria (configurable) | Default Range |
|------|------------------------|---------------|
| HOT | Days from today ≤ hot_days | 0–3 days |
| WARM | Days from today between hot_days and warm_days | 4–14 days |
| COLD | Days from today > warm_days | 15–40 days |

### Priority Matrix

| | HOT Date | WARM Date | COLD Date |
|---|----------|-----------|-----------|
| **HOT Route** | P1 | P2 | P4 |
| **WARM Route** | P2 | P3 | P5 |
| **COLD Route** | P4 | P5 | P6 |

This maps directly to the specification:
- HOT + HOT → P1
- HOT + WARM → P2
- WARM + HOT → P2
- WARM + WARM → P3
- HOT + COLD → P4
- COLD + HOT → P4
- WARM + COLD → P5
- COLD + WARM → P5
- COLD + COLD → P6

### Deduplication Strategy

- **Key format**: `dedup:{routeID}:{travelDate}` stored in Dragonfly
- **TTL**: Set to slightly longer than one scheduler cycle (12 minutes) to prevent duplicate tasks within overlapping cycles
- **Check**: Before publishing, SET NX (set-if-not-exists) the dedup key. If the key already exists, skip publishing.
- **Clearing**: Keys auto-expire via TTL; no manual cleanup needed

### Scheduler State and Recovery

- **State stored in Dragonfly**:
  - `scheduler:last_cycle_start` — timestamp of last cycle start
  - `scheduler:last_cycle_complete` — timestamp of last successful cycle completion
  - `scheduler:tasks_published` — count of tasks published in last cycle
- **Restart recovery**: On startup, the scheduler reads `scheduler:last_cycle_complete`. If more than 10 minutes have elapsed, it immediately starts a new cycle. Otherwise, it waits for the remaining interval.
- **Incomplete cycle**: If `scheduler:last_cycle_start` exists but `scheduler:last_cycle_complete` is absent or older, the scheduler logs a warning (previous cycle may have been interrupted) and starts a fresh cycle. Deduplication keys prevent double-publishing for tasks that were already published before the crash.

### Top Route Data Source

- The scheduler fetches route popularity data from a configured endpoint (orbitplusservice provides this data or it is loaded from a known data source)
- If the top_route source is unavailable, all routes default to COLD tier (per spec edge case)
- The scheduler caches the route classification in Dragonfly for the duration of the cycle to avoid repeated fetches

---

## 3. Priority and Queue Design

### Queue Architecture

```text
Exchange: tripdetails.inventory (direct exchange)
Queue:    tripdetails.inventory.tasks (standard queue, no priority)
DLX:      tripdetails.inventory.dlx (dead-letter exchange)
DLQ:      tripdetails.inventory.dead-letter (dead-letter queue)

Exchange: tripdetails.periodic (direct exchange)
Queue:    tripdetails.periodic.tasks (x-max-priority: 6)
DLX:      tripdetails.periodic.dlx (dead-letter exchange)
DLQ:      tripdetails.periodic.dead-letter (dead-letter queue)

Shared:
Retry Exchange: tripdetails.retry (with per-queue routing)
Retry Queue:    tripdetails.inventory.retry (TTL-based delay → tripdetails.inventory.tasks)
Retry Queue:    tripdetails.periodic.retry (TTL-based delay → tripdetails.periodic.tasks)
```

### Queue 1: Inventory Change Queue

| Property | Value |
|----------|-------|
| Name | `tripdetails.inventory.tasks` |
| Priority support | No (all inventory-change tasks are treated as equally urgent) |
| Purpose | Event-driven refresh triggered by inventory changes |
| Publisher | Inventory System / orbitplusservice event publisher |
| Urgency | Higher than any periodic task — isolation guarantees timely processing |

### Queue 2: Periodic Refresh Queue

| Property | Value |
|----------|-------|
| Name | `tripdetails.periodic.tasks` |
| Priority support | Yes (`x-max-priority: 6`) |
| Purpose | Scheduler-driven 40-day window maintenance |
| Publisher | Periodic Scheduler |
| Priority mapping | P1=6, P2=5, P3=4, P4=3, P5=2, P6=1 (higher RabbitMQ value = processed first) |

### Priority Mapping (Periodic Queue Only)

| Spec Priority | RabbitMQ Priority Value | Route+Date Combination |
|--------------|------------------------|------------------------|
| P1 | 6 (highest) | HOT + HOT |
| P2 | 5 | HOT+WARM, WARM+HOT |
| P3 | 4 | WARM + WARM |
| P4 | 3 | HOT+COLD, COLD+HOT |
| P5 | 2 | WARM+COLD, COLD+WARM |
| P6 | 1 (lowest) | COLD + COLD |

### Isolation Guarantee

- Inventory-change tasks are processed from a **separate queue** with its own consumer allocation
- Even if the Periodic Refresh Queue has thousands of messages queued, inventory-change messages are consumed independently and immediately
- Workers allocate dedicated consumer goroutines to the Inventory Change Queue, ensuring inventory events are never blocked by periodic backlog

### Priority Starvation Prevention (Periodic Queue)

- **Natural ordering**: RabbitMQ priority queues deliver highest-priority messages first. Under normal load (queue drains within cycle), all priorities are processed.
- **Backpressure signal**: If queue depth exceeds a configurable threshold (e.g., 2× expected cycle volume), the scheduler logs a warning and the monitoring system alerts.
- **P6 TTL safeguard**: P6 messages carry a message TTL. If a P6 message has not been consumed within a configurable window (e.g., 30 minutes), it expires and is dead-lettered rather than blocking indefinitely. This prevents unbounded accumulation of low-priority work during sustained high load.
- **Minimum processing guarantee**: Workers consume whatever RabbitMQ delivers next from the periodic queue. Since RabbitMQ serves highest priority first, starvation only occurs under sustained overload, which the monitoring system detects.

### Retry Behavior (Both Queues)

- **Retry count**: Tracked via a custom header `x-retry-count` on the message
- **Max retries**: Configurable (default: 3)
- **Retry mechanism**: On temporary failure, the worker increments `x-retry-count` and republishes the message to the appropriate retry queue with a delay
- **Retry queue routing**: Messages from each queue route back to their origin queue after TTL expiry:
  - `tripdetails.inventory.retry` → `tripdetails.inventory.tasks`
  - `tripdetails.periodic.retry` → `tripdetails.periodic.tasks`
- **Retry preserves priority**: Periodic messages retain their original RabbitMQ priority on republish

### Dead-Letter Behavior (Both Queues)

- Messages exceeding max retry count are rejected (basic.nack with requeue=false)
- Each queue has its own dead-letter exchange and dead-letter queue:
  - `tripdetails.inventory.dead-letter` — failed inventory-change tasks
  - `tripdetails.periodic.dead-letter` — failed periodic refresh tasks
- Dead-lettered messages retain all original headers including correlation ID, retry count, and failure reason
- Separate DLQs enable independent monitoring and alerting thresholds per queue type
- DLQ messages are available for manual inspection and replay

---

## 4. Worker Service

### Worker Lifecycle

```text
Start
  │
  ├─▶ Connect to RabbitMQ
  ├─▶ Connect to Dragonfly
  ├─▶ Register consumer on Inventory Change Queue (dedicated goroutines)
  ├─▶ Register consumer on Periodic Refresh Queue (dedicated goroutines)
  │
  ▼
Dual Consumer Loop
  │
  ├─▶ [Inventory Consumer] Receive message from Inventory Change Queue
  │     └─▶ Process (same pipeline as periodic, refresh_type = "inventory_change")
  │
  ├─▶ [Periodic Consumer] Receive message from Periodic Refresh Queue
  │     └─▶ Process (refresh_type = "periodic")
  │
  ├─▶ Processing Pipeline (shared):
  │     ├─▶ Parse RefreshTask
  │     ├─▶ Obtain credentials (Dragonfly/Orbit)
  │     ├─▶ Fetch Inventory/BusMap data
  │     ├─▶ Compose TripDetails JSON
  │     ├─▶ Push to orbitplusservice
  │     ├─▶ On success: ACK message
  │     ├─▶ On temp failure: retry or republish to retry queue
  │     └─▶ On permanent failure: report to failure API, NACK (no requeue)
  │
  ▼
Shutdown (SIGTERM/SIGINT)
  │
  ├─▶ Stop accepting new messages (cancel both consumers)
  ├─▶ Wait for in-flight messages to complete (graceful timeout)
  ├─▶ Close connections
  └─▶ Exit
```

### Dual-Queue Consumption Model

Each worker instance runs two independent consumer groups:

| Consumer Group | Queue | Goroutines | Prefetch | Purpose |
|---------------|-------|------------|----------|---------|
| Inventory consumer | `tripdetails.inventory.tasks` | Configurable (default: 5) | 5 | Ensures inventory-change tasks are always promptly processed |
| Periodic consumer | `tripdetails.periodic.tasks` | Configurable (default: 10) | 10 | Handles bulk periodic refresh work |

**Key design decisions**:
- Inventory consumer goroutines are **reserved** — they only process inventory-change messages, guaranteeing capacity even when periodic load is high
- Periodic consumer goroutines handle the higher-volume scheduled work
- The ratio is configurable per deployment (e.g., increase inventory consumers if event volume grows)
- Both consumer groups share the same processing pipeline code; only the message source and `refresh_type` field differ

### Concurrency Model

- Each worker instance runs a configurable total number of concurrent goroutines split across both queues
- **Prefetch count**: Set per-consumer-group to limit in-flight messages
- **No shared mutable state**: Each goroutine operates independently with its own HTTP client and context
- **Channel isolation**: Each consumer group operates on its own RabbitMQ channel for independent flow control

### Credential Retrieval

- Workers obtain operator credentials from Dragonfly using a well-known key pattern: `creds:{operatorCode}`
- Credentials are cached locally in-memory for a configurable TTL (e.g., 5 minutes) to reduce Dragonfly round-trips
- If credentials are missing or expired in Dragonfly, the worker fetches fresh credentials from the Orbit credential service and populates Dragonfly

### Inventory/BusMap Fetch

- Worker constructs the API request from the refresh task's route (from/to station codes) and travel date
- HTTP call to Inventory/BusMap API with:
  - Configurable timeout (default: 10 seconds)
  - Correlation ID passed as request header
  - Operator credentials in the request (as required by the API)
- Response parsed into internal domain model

### TripDetails Composition

- Worker composes the TripDetails JSON from the fetched Inventory/BusMap data
- Includes metadata: correlation ID, refresh timestamp, route info, travel date, version identifier, refresh_type (periodic|inventory_change)
- Version identifier is derived from the refresh timestamp (epoch milliseconds) to enable stale-data detection at orbitplusservice

### Push to orbitplusservice

- HTTP POST to orbitplusservice Push API endpoint
- Request includes: TripDetails JSON body, correlation ID header, version header
- Timeout: configurable (default: 5 seconds)
- Worker interprets response:
  - `200/201`: Success → ACK the RabbitMQ message
  - `409` (Conflict/Stale): Data is older than existing → ACK the message (no error, work is unnecessary)
  - `429` (Rate Limited): Back off and retry after the indicated delay
  - `400` (Validation Error): Permanent failure → report to failure API, NACK without requeue
  - `5xx` (Server Error): Temporary failure → retry

### Retry Strategy (Worker-Level)

- **Temporary failures**: Inventory/BusMap timeout, orbitplusservice 5xx, network errors
- **Backoff**: Exponential with jitter: `base_delay * 2^attempt + random_jitter`
  - Base delay: 1 second
  - Max delay: 30 seconds
  - Max attempts within single message processing: 3 (for immediate retries)
- **Message-level retry**: If all immediate retries fail, republish to the appropriate retry queue (`tripdetails.inventory.retry` or `tripdetails.periodic.retry`) with incremented `x-retry-count`
- **Permanent failures**: 404 from Inventory, 400 from orbitplusservice, credential errors after refresh attempt

### Failure Reporting

- On permanent failure, worker sends a POST to orbitplusservice Failure API:
  - Correlation ID
  - Route ID (from + to station codes)
  - Travel date
  - Error category (inventory_not_found, validation_error, credential_error)
  - Error detail message
  - Timestamp
  - Refresh type (periodic | inventory_change)
  - Source queue identifier
- Worker then NACKs the message without requeue (message goes to appropriate DLQ)

### Database Access Prohibition

Workers have no database connection string, no database driver dependency, and no database-related code. All persistence flows exclusively through orbitplusservice HTTP API.

---

## 5. Master Service (orbitplusservice Extensions)

### TripDetails Push API

**Endpoint**: `POST /orbitplusservice/api/tripdetails/push`

**Request Processing Flow**:

1. **Rate limit check**: If request exceeds rate limit → respond 429
2. **Validate payload**: Check required fields, JSON structure, data types → 400 on failure
3. **Extract version**: Read version/timestamp from payload
4. **Check existing record**: Query TripDetails DB for existing entry with same route+date
5. **Version comparison**:
   - If no existing record → insert (new)
   - If existing record has older version → update (replace)
   - If existing record has same version → respond 200 (idempotent, no-op)
   - If existing record has newer version → respond 409 (stale, rejected)
6. **Persist**: Write to TripDetails DB
7. **Respond**: 201 Created (new) or 200 OK (updated/no-op)

### Idempotency Design

- **Idempotency key**: Combination of route ID (from+to) + travel date + version timestamp
- **Behavior**: Repeated pushes with the same key produce the same result without side effects
- **No separate idempotency store needed**: The version comparison at persistence time inherently provides idempotency

### Stale Data Prevention

- Each TripDetails payload carries a `refreshed_at` timestamp (epoch milliseconds, set by worker at fetch time)
- orbitplusservice compares incoming `refreshed_at` against the stored record's `refreshed_at`
- Strictly newer wins; equal is treated as duplicate (no-op); older is rejected (409)
- This handles out-of-order delivery from both queues, duplicate processing, and concurrent worker results

### Failure API

**Endpoint**: `POST /orbitplusservice/api/tripdetails/failures`

**Purpose**: Record permanent processing failures for operational visibility

**Request Processing**:
1. Validate failure report payload
2. Persist failure record (correlation ID, route, date, error category, error detail, timestamp, refresh_type)
3. Respond 201 Created

Failure records are queryable by operators for troubleshooting and can trigger alerts when volume exceeds thresholds. Records include `refresh_type` to distinguish inventory-change failures from periodic failures.

### Concurrent Worker Support

- orbitplusservice is stateless (no in-memory session state between requests)
- Database writes use optimistic concurrency: `UPDATE ... WHERE refreshed_at < :new_refreshed_at`
- If the UPDATE affects 0 rows (record already newer), respond 409 without error
- Multiple orbitplusservice instances can run behind a load balancer; each instance processes independently
- The Push API is agnostic to the source queue — it processes identically whether the data came from an inventory-change task or a periodic task

---

## 6. Rate Limiting

### Design

- **Scope**: Per-endpoint rate limiting on the TripDetails Push API
- **Algorithm**: Token bucket (requests-per-second with burst allowance)
- **Configuration**:
  - `rate_limit_rps`: Sustained requests per second (configurable, e.g., 100)
  - `rate_limit_burst`: Maximum burst size (configurable, e.g., 150)
- **Implementation**: In-process token bucket per orbitplusservice instance

### Behavior When Limit Exceeded

- Response: `429 Too Many Requests`
- Response header: `Retry-After: <seconds>` indicating when the worker should retry
- Workers honor the `Retry-After` header and back off accordingly

### Horizontal Scaling Consideration

- Each orbitplusservice instance maintains its own local rate limiter
- **Per-instance limiting** is sufficient when:
  - Instances are behind a load balancer distributing evenly
  - The per-instance limit is set to `total_desired_rps / instance_count`
  - Configuration is updated when scaling events occur
- **Distributed rate limiting** (via Dragonfly) is NOT required unless:
  - Traffic distribution is highly uneven
  - Strict global rate enforcement is needed regardless of instance count
- **Design decision**: Start with per-instance local rate limiting. Document Dragonfly-based distributed limiting as an optional future enhancement if operational experience shows per-instance is insufficient.

### Database Protection

- Rate limit values are chosen based on TripDetails DB write capacity
- The rate limiter acts as a safety valve: even if all workers push simultaneously from both queues, the DB is protected
- If DB becomes slow, orbitplusservice can dynamically reduce the rate limit via configuration reload

---

## 7. Dragonfly Usage

### Key Patterns and Purposes

| Purpose | Key Pattern | Value | TTL |
|---------|-------------|-------|-----|
| Task deduplication (periodic) | `dedup:periodic:{fromCode}:{toCode}:{travelDate}` | `1` | 12 minutes |
| Task deduplication (inventory) | `dedup:inventory:{fromCode}:{toCode}:{travelDate}` | `1` | 2 minutes |
| Scheduler metadata | `scheduler:last_cycle_start` | ISO timestamp | None (persistent) |
| Scheduler metadata | `scheduler:last_cycle_complete` | ISO timestamp | None (persistent) |
| Scheduler metadata | `scheduler:tasks_published` | Integer count | 12 minutes |
| Route tier cache | `route_tier:{routeID}` | `HOT\|WARM\|COLD` | 12 minutes |
| Operator credentials | `creds:{operatorCode}` | JSON credential blob | Configurable (e.g., 1 hour) |
| Worker processing lock | `lock:task:{correlationID}` | Worker instance ID | 5 minutes (safety timeout) |
| Rate limit state (optional) | `ratelimit:push_api:{instanceID}` | Token count | Rolling window |

### Design Principles

- Dragonfly is used exclusively for **ephemeral/temporary state**
- No durable data is stored in Dragonfly — loss of Dragonfly data causes at most duplicate processing (handled idempotently)
- All keys have explicit TTLs (except scheduler metadata which is overwritten each cycle)
- Operations use atomic commands (SET NX, INCR, EXPIRE) to avoid race conditions
- Deduplication keys are namespaced by source (`periodic:` vs `inventory:`) because the two paths have different TTL requirements and a periodic dedup should not suppress an inventory-change task for the same route+date

### Failure Mode

If Dragonfly is unavailable:
- **Scheduler**: Deduplication is degraded — may publish duplicate tasks. Workers and orbitplusservice handle duplicates idempotently, so correctness is maintained.
- **Workers**: Credential cache miss — workers fall through to direct credential fetch from Orbit. Processing continues with higher latency.
- **Processing locks**: Not enforced — at-most-once processing degrades to at-least-once. Idempotency at orbitplusservice prevents data corruption.

---

## 8. Reliability

### RabbitMQ Acknowledgement Strategy (Both Queues)

- **Consumer**: Manual acknowledgement mode (autoAck=false) on both queues
- **ACK**: Sent only after successful push to orbitplusservice (or successful 409 stale response)
- **NACK without requeue**: Sent when max retries exhausted (message goes to respective DLQ via dead-letter exchange)
- **NACK with requeue**: NOT used (risks infinite redelivery loops). Instead, messages are republished to the appropriate retry queue with incremented counter.
- **Publisher confirms**: Scheduler and inventory event publishers enable publisher confirms to ensure messages reach RabbitMQ

### Retry Policy

| Parameter | Inventory Change Queue | Periodic Refresh Queue | Notes |
|-----------|----------------------|----------------------|-------|
| Max immediate retries (within worker) | 3 | 3 | For transient network blips |
| Max message-level retries | 3 | 3 | Via retry queue republish |
| Total max attempts | 4 (1 initial + 3 retries) | 4 (1 initial + 3 retries) | Before dead-lettering |
| Immediate retry backoff | 1s, 2s, 4s | 1s, 2s, 4s | Exponential + jitter |
| Message retry delay | 10s, 20s, 40s | 30s, 60s, 120s | Inventory retries faster due to urgency |

### Dead Letter Queues

| DLQ | Source | Monitoring Threshold |
|-----|--------|---------------------|
| `tripdetails.inventory.dead-letter` | Failed inventory-change tasks | Alert if depth > 5 (urgent failures) |
| `tripdetails.periodic.dead-letter` | Failed periodic refresh tasks | Alert if depth > 50 (bulk failures) |

- Messages retain: original body, all headers (correlation ID, retry count, last error, source queue)
- Separate DLQs enable different alerting sensitivity (inventory failures are more critical)
- Operators can inspect and manually replay DLQ messages back to their origin queue

### Component Failure Recovery

| Component | Failure Mode | Recovery |
|-----------|-------------|----------|
| **Worker crash** | Unacked messages on both queues | RabbitMQ redelivers to another worker (messages return to ready state on respective queues) |
| **RabbitMQ unavailable** | Scheduler/publishers cannot publish | Scheduler retries connection with backoff; inventory event publisher buffers or retries; cycle skipped if broker is down for full cycle |
| **orbitplusservice down** | Worker push fails | Worker retries with backoff; message eventually dead-lettered if all retries fail |
| **Inventory/BusMap down** | Fetch fails | Worker retries; temporary failures back off; if persistent, message dead-lettered |
| **Dragonfly down** | Dedup/lock unavailable | System continues with degraded dedup; idempotency at Master prevents corruption |
| **Scheduler restart** | Interrupted cycle | Reads last_cycle_complete from Dragonfly; starts fresh cycle; dedup keys prevent duplicate publishing |
| **Database slow** | orbitplusservice responds slowly | Rate limiter protects DB; workers back off on timeouts; both queues absorb backpressure |

### Graceful Shutdown

1. Receive SIGTERM/SIGINT
2. Stop accepting new messages from both queues (cancel both consumer groups)
3. Wait for in-flight messages to complete (configurable timeout, e.g., 30 seconds)
4. If timeout reached, NACK remaining in-flight messages (they return to respective queues for other workers)
5. Close RabbitMQ connection
6. Close Dragonfly connection
7. Exit cleanly

---

## 9. Data Consistency

### Concurrent Refresh Handling

| Scenario | How Handled |
|----------|-------------|
| Inventory-change refresh and periodic refresh for same route+date arrive simultaneously | Both workers process independently; version timestamp comparison at orbitplusservice — the one with the later `refreshed_at` wins; the earlier one receives 409 |
| Newer TripDetails from inventory-change queue completes before an older periodic worker result | Older periodic result arrives at orbitplusservice, `refreshed_at` is older than stored value → 409 rejected, newer inventory-change data preserved |
| Same task delivered more than once (redelivery) | Worker processes it again; orbitplusservice compares version — if same, returns 200 (idempotent no-op); if older (because a fresh refresh happened between), returns 409 |
| Worker times out after Master has already persisted | Worker's RabbitMQ message is not ACKed → message redelivered → next attempt pushes same data → orbitplusservice detects duplicate version → 200 no-op |
| Inventory-change event arrives for a route that periodic scheduler also just scheduled | Both are processed; the one that fetches more recent data from Inventory/BusMap will have a later `refreshed_at` and win at orbitplusservice. The other is harmlessly rejected. |

### Version/Timestamp Contract

- **Source of truth**: `refreshed_at` field — epoch milliseconds when the worker fetched data from Inventory/BusMap
- **Comparison**: Strictly numeric. Higher `refreshed_at` = newer data.
- **Source-agnostic**: orbitplusservice does NOT differentiate between inventory-change and periodic sources for version comparison. The only thing that matters is which data is newer.
- **Database operation**: Conditional update: `UPDATE tripdetails SET ... WHERE route_key = :key AND travel_date = :date AND refreshed_at < :new_refreshed_at`
- **Zero-row update**: Indicates stale data → respond 409 to worker
- **Insert (new record)**: No existing record → INSERT directly

### Idempotency Guarantee

The combination of:
1. Deduplication at scheduler level (prevents most duplicate periodic task generation)
2. Deduplication at inventory event level (prevents rapid-fire duplicate events)
3. Version comparison at orbitplusservice (prevents duplicate/stale writes regardless of source)
4. Worker ACK-after-success (prevents premature message removal)

…ensures that the system is safe under message redelivery, worker crashes, concurrent processing, and cross-queue race conditions.

---

## 10. Observability

### Structured Logging

All log entries include:
- `timestamp` (ISO 8601)
- `level` (DEBUG, INFO, WARN, ERROR)
- `component` (scheduler, worker, master)
- `correlation_id` (when available)
- `refresh_type` (periodic | inventory_change)
- `source_queue` (inventory | periodic)
- `message`
- Additional contextual fields per event

### Correlation ID Flow

```text
Scheduler / Inventory Event (generates ID)
    → RabbitMQ message header: X-Correlation-ID
        → Worker (extracts, propagates)
            → Inventory/BusMap request header: X-Correlation-ID
            → orbitplusservice push request header: X-Correlation-ID
                → Database record field: correlation_id
```

### Metrics

| Metric | Component | Type | Description |
|--------|-----------|------|-------------|
| `scheduler.cycle.duration_ms` | Scheduler | Histogram | Time to complete one scheduler cycle |
| `scheduler.tasks.published` | Scheduler | Counter | Tasks published per cycle |
| `scheduler.tasks.deduplicated` | Scheduler | Counter | Tasks skipped due to dedup |
| `scheduler.routes.total` | Scheduler | Gauge | Total routes evaluated |
| `queue.inventory.depth` | RabbitMQ | Gauge | Messages waiting in inventory change queue |
| `queue.periodic.depth` | RabbitMQ | Gauge | Messages waiting in periodic refresh queue |
| `queue.inventory.dlq.depth` | RabbitMQ | Gauge | Messages in inventory DLQ |
| `queue.periodic.dlq.depth` | RabbitMQ | Gauge | Messages in periodic DLQ |
| `worker.task.duration_ms` | Worker | Histogram | End-to-end task processing time (labeled by refresh_type) |
| `worker.task.success` | Worker | Counter | Successfully processed tasks (labeled by source_queue) |
| `worker.task.failure` | Worker | Counter | Failed tasks (labeled by source_queue) |
| `worker.task.retry` | Worker | Counter | Retry attempts (labeled by source_queue) |
| `worker.inventory_api.latency_ms` | Worker | Histogram | Inventory/BusMap API call latency |
| `worker.master_api.latency_ms` | Worker | Histogram | orbitplusservice push call latency |
| `master.push.received` | Master | Counter | Push requests received |
| `master.push.persisted` | Master | Counter | Successfully persisted |
| `master.push.rejected_stale` | Master | Counter | Rejected (stale version) |
| `master.push.rejected_duplicate` | Master | Counter | Rejected (duplicate) |
| `master.push.rejected_invalid` | Master | Counter | Rejected (validation) |
| `master.push.rate_limited` | Master | Counter | Rejected (rate limit) |
| `master.failures.reported` | Master | Counter | Failure reports received (labeled by refresh_type) |

### Health Checks

- Scheduler: `/health` endpoint reporting last cycle status, Dragonfly connectivity, RabbitMQ connectivity
- Worker: `/health` endpoint reporting consumer status for both queues, Dragonfly connectivity, active goroutine count per consumer group
- orbitplusservice: Existing health infrastructure extended with push API readiness

### Alerting Rules

| Alert | Condition | Severity |
|-------|-----------|----------|
| Inventory queue backing up | `queue.inventory.depth` > 10 for 2 minutes | Critical |
| Periodic queue not draining | `queue.periodic.depth` > 2× cycle volume for 15 minutes | Warning |
| Inventory DLQ growing | `queue.inventory.dlq.depth` > 5 | Critical |
| Periodic DLQ growing | `queue.periodic.dlq.depth` > 50 | Warning |
| Worker processing latency high | `worker.task.duration_ms` p95 > 30s | Warning |
| Scheduler cycle missed | No `scheduler:last_cycle_complete` update for 20 minutes | Critical |

---

## 11. API and Data Contracts

### Refresh Task — Periodic (Scheduler → Periodic Refresh Queue → Worker)

```json
{
  "task_id": "uuid-v4",
  "correlation_id": "uuid-v4",
  "route_id": "BLR_CHN",
  "from_station_code": "BLR",
  "to_station_code": "CHN",
  "travel_date": "2026-08-15",
  "priority": 1,
  "route_tier": "HOT",
  "date_tier": "HOT",
  "refresh_type": "periodic",
  "scheduled_at": "2026-08-12T11:30:00Z",
  "operator_code": "OP123",
  "retry_count": 0
}
```

**RabbitMQ message properties (Periodic Queue)**:
- `priority`: Integer 1–6 (mapped: P6=1, P5=2, P4=3, P3=4, P2=5, P1=6)
- `message_id`: Same as `task_id`
- `correlation_id`: Same as `correlation_id`
- `content_type`: `application/json`
- `headers.x-retry-count`: Integer
- `headers.x-source-queue`: `periodic`

### Refresh Task — Inventory Change (Inventory System → Inventory Change Queue → Worker)

```json
{
  "task_id": "uuid-v4",
  "correlation_id": "uuid-v4",
  "route_id": "BLR_CHN",
  "from_station_code": "BLR",
  "to_station_code": "CHN",
  "travel_date": "2026-08-15",
  "refresh_type": "inventory_change",
  "triggered_at": "2026-08-12T11:25:00Z",
  "operator_code": "OP123",
  "change_type": "inventory_updated",
  "retry_count": 0
}
```

**RabbitMQ message properties (Inventory Change Queue)**:
- `priority`: Not set (queue has no priority support)
- `message_id`: Same as `task_id`
- `correlation_id`: Same as `correlation_id`
- `content_type`: `application/json`
- `headers.x-retry-count`: Integer
- `headers.x-source-queue`: `inventory`

**Note**: Inventory-change tasks do NOT carry `priority`, `route_tier`, or `date_tier` fields — those concepts apply only to periodic scheduling.

### TripDetails Push (Worker → orbitplusservice)

**Request**: `POST /orbitplusservice/api/tripdetails/push`

```json
{
  "correlation_id": "uuid-v4",
  "route_id": "BLR_CHN",
  "from_station_code": "BLR",
  "to_station_code": "CHN",
  "travel_date": "2026-08-15",
  "operator_code": "OP123",
  "refreshed_at": 1723456200000,
  "refresh_type": "periodic",
  "trip_details": {
    "trips": [
      {
        "trip_code": "TC001",
        "trip_stage_code": "TSC001",
        "display_name": "Express Service",
        "travel_time": "06:00",
        "bus": {
          "type": "AC Sleeper",
          "total_seats": 36
        },
        "fares": [
          {
            "seat_type": "sleeper",
            "fare": 850.00,
            "available_seats": 12
          }
        ],
        "operator": {
          "code": "OP123",
          "name": "Operator Name"
        },
        "amenities": ["wifi", "charging"],
        "schedule": {
          "departure": "22:00",
          "arrival": "06:00",
          "duration_minutes": 480
        }
      }
    ]
  }
}
```

**Response (Success)**:
```json
{
  "status": "created",
  "correlation_id": "uuid-v4",
  "persisted_at": "2026-08-12T11:35:00Z"
}
```

**Response (Stale/Conflict)**:
```json
{
  "status": "rejected",
  "reason": "stale_version",
  "correlation_id": "uuid-v4",
  "existing_refreshed_at": 1723456300000,
  "incoming_refreshed_at": 1723456200000
}
```

**Response (Rate Limited)**:
```json
{
  "status": "rate_limited",
  "retry_after_seconds": 2
}
```

### Failure Report (Worker → orbitplusservice)

**Request**: `POST /orbitplusservice/api/tripdetails/failures`

```json
{
  "correlation_id": "uuid-v4",
  "route_id": "BLR_CHN",
  "from_station_code": "BLR",
  "to_station_code": "CHN",
  "travel_date": "2026-08-15",
  "operator_code": "OP123",
  "error_category": "inventory_not_found",
  "error_detail": "Inventory API returned 404 for route BLR_CHN on 2026-08-15",
  "failed_at": "2026-08-12T11:32:00Z",
  "refresh_type": "inventory_change",
  "source_queue": "inventory",
  "retry_count": 3
}
```

**Response**:
```json
{
  "status": "recorded",
  "failure_id": "uuid-v4"
}
```

### Inventory/BusMap Request (Worker → External API)

- **Endpoint**: Configured per-operator (base URL from credentials)
- **Method**: GET
- **Path pattern**: `/{operator_code}/{username}/{api_token}/search/{fromCode}/{toCode}/{travelDate}`
- **Headers**: `X-Correlation-ID: {correlation_id}`
- **Response**: Operator-specific format parsed by the worker into the internal TripDetails model

---

## 12. Project Structure

```text
orbitplus/
├── build/                          # Build scripts, CI/CD configs
├── cmd/
│   ├── scheduler/
│   │   └── main.go                 # Scheduler entry point
│   └── worker/
│       └── main.go                 # Worker entry point
├── configs/
│   ├── scheduler.yaml              # Scheduler configuration
│   ├── worker.yaml                 # Worker configuration (both queues)
│   └── master.yaml                 # orbitplusservice config (if separate)
├── data/                           # Data files (migrations, seeds)
├── docs/                           # Documentation
├── internal/
│   ├── application/
│   │   ├── scheduler/
│   │   │   ├── cycle.go            # Scheduler cycle orchestration
│   │   │   └── recovery.go         # Restart recovery logic
│   │   ├── worker/
│   │   │   ├── pipeline.go         # Shared processing pipeline
│   │   │   ├── consumer_inventory.go  # Inventory queue consumer setup
│   │   │   ├── consumer_periodic.go   # Periodic queue consumer setup
│   │   │   ├── retry.go            # Retry orchestration (queue-aware)
│   │   │   └── shutdown.go         # Graceful shutdown (both consumers)
│   │   └── master/
│   │       ├── push_handler.go     # Push API handler orchestration
│   │       └── failure_handler.go  # Failure API handler orchestration
│   ├── common/
│   │   ├── config/
│   │   │   └── config.go           # Configuration loading
│   │   ├── correlation/
│   │   │   └── context.go          # Correlation ID context propagation
│   │   ├── logging/
│   │   │   └── logger.go           # Structured logger setup
│   │   └── metrics/
│   │       └── metrics.go          # Metrics registry and helpers
│   ├── domain/
│   │   ├── models/
│   │   │   ├── refresh_task.go     # RefreshTask model (both types)
│   │   │   ├── trip_details.go     # TripDetails model
│   │   │   ├── route.go            # Route model with tier
│   │   │   ├── priority.go         # Priority matrix and calculation
│   │   │   └── failure_report.go   # Failure report model
│   │   ├── services/
│   │   │   ├── tier_classifier.go  # Route/date tier classification
│   │   │   ├── priority_calculator.go  # Priority matrix logic
│   │   │   ├── version_comparator.go   # Version/timestamp comparison
│   │   │   └── validator.go        # TripDetails validation rules
│   │   └── ports/
│   │       ├── queue_publisher.go  # Interface: publish to queue
│   │       ├── queue_consumer.go   # Interface: consume from queue
│   │       ├── cache_store.go      # Interface: Dragonfly operations
│   │       ├── inventory_client.go # Interface: Inventory/BusMap API
│   │       ├── master_client.go    # Interface: orbitplusservice push
│   │       ├── route_repository.go # Interface: route data access
│   │       └── tripdetails_repository.go  # Interface: TripDetails persistence
│   └── infrastructure/
│       ├── rabbitmq/
│       │   ├── publisher.go        # RabbitMQ publisher (both exchanges)
│       │   ├── consumer.go         # RabbitMQ consumer (dual-queue)
│       │   ├── connection.go       # Connection management, reconnection
│       │   └── topology.go         # Exchange/queue/binding declarations
│       ├── dragonfly/
│       │   ├── client.go           # Dragonfly client wrapper
│       │   ├── dedup.go            # Deduplication operations (namespaced)
│       │   ├── locks.go            # Distributed lock operations
│       │   └── credentials.go     # Credential cache operations
│       ├── http/
│       │   ├── inventory_client.go # Inventory/BusMap HTTP client
│       │   ├── master_client.go    # orbitplusservice HTTP client
│       │   └── middleware/
│       │       ├── rate_limiter.go # Rate limiting middleware
│       │       ├── correlation.go  # Correlation ID middleware
│       │       └── logging.go      # Request logging middleware
│       └── database/
│           └── tripdetails_repo.go # TripDetails DB implementation
├── logs/                           # Log output directory
├── postman/                        # Postman collections for API testing
├── scripts/                        # Operational scripts
├── specs/                          # Feature specifications
│   └── 001-proactive-tripdetails-refresh/
├── test/
│   ├── integration/
│   │   ├── scheduler_test.go
│   │   ├── worker_inventory_test.go   # Inventory queue consumer tests
│   │   ├── worker_periodic_test.go    # Periodic queue consumer tests
│   │   ├── master_push_test.go
│   │   └── end_to_end_test.go
│   ├── mocks/
│   │   ├── queue_mock.go
│   │   ├── cache_mock.go
│   │   ├── inventory_mock.go
│   │   └── master_mock.go
│   └── testdata/
│       ├── routes.json
│       ├── trip_details_sample.json
│       ├── inventory_response.json
│       └── inventory_change_event.json
├── ui/                             # UI assets (if any)
├── .dockerignore
├── .env
├── .gitignore
├── docker-compose.yml              # RabbitMQ, Dragonfly, services
├── Dockerfile
├── go.mod
├── go.sum
└── README.md
```

### Structure Decisions

- **cmd/scheduler** and **cmd/worker** are separate binaries — independently deployable
- **internal/domain** contains zero external dependencies — pure Go, no imports from infrastructure
- **internal/domain/ports** defines interfaces that infrastructure implements (dependency inversion)
- **internal/application/worker** has separate files for inventory and periodic consumer setup, sharing a common pipeline
- **internal/infrastructure/rabbitmq/topology.go** declares both queues, exchanges, bindings, and DLX configuration
- **internal/infrastructure/dragonfly/dedup.go** handles namespaced deduplication (periodic vs inventory)
- **orbitplusservice extensions** (push API, failure API, rate limiter) live within the existing orbitplusservice codebase but follow the same layering pattern

---

## 13. Testing Strategy

### Unit Tests

| Component | What to Test | Approach |
|-----------|-------------|----------|
| Priority calculation | All 9 tier combinations produce correct P1–P6 | Table-driven tests with exact matrix values |
| Tier classifier | Route classification at boundaries | Boundary value tests (exactly at threshold, ±1) |
| Tier classifier | Date classification at boundaries | Tests for day 0, 3, 4, 14, 15, 40 |
| Version comparator | Newer/older/equal timestamp comparison | Edge cases: equal, off-by-one ms, zero values |
| Validator | Valid/invalid TripDetails payloads | Missing fields, wrong types, empty arrays |
| Retry policy | Backoff calculation and jitter bounds | Verify exponential growth, max cap, jitter range |
| Rate limiter | Token bucket behavior | Fill, drain, burst, refill timing |
| Task model | Periodic vs inventory-change task parsing | Ensure correct fields present/absent per type |

### Integration Tests

| Component | What to Test | Infrastructure Needed |
|-----------|-------------|----------------------|
| Scheduler → Periodic Queue | Tasks published with correct priority and format | RabbitMQ test container |
| Scheduler → Dragonfly | Deduplication prevents double-publish | Dragonfly test container |
| Worker ← Inventory Queue | Message consumption from inventory queue, ACK/NACK | RabbitMQ test container |
| Worker ← Periodic Queue | Message consumption respects priority ordering | RabbitMQ test container |
| Worker dual-consumer | Both consumer groups operate independently | RabbitMQ test container |
| Worker → Dragonfly | Credential retrieval, lock acquisition | Dragonfly test container |
| Worker → Inventory mock | HTTP call with correct params, timeout handling | HTTP test server |
| Worker → Master mock | Push succeeds/fails, retry to correct retry queue | HTTP test server |
| Master Push API | Version comparison, persistence, idempotency | Test database |
| Master Rate Limiter | Requests accepted/rejected at threshold | In-process test |
| Inventory DLQ flow | Message exhausts retries → appears in inventory DLQ | RabbitMQ test container |
| Periodic DLQ flow | Message exhausts retries → appears in periodic DLQ | RabbitMQ test container |
| Queue isolation | Periodic queue backlog does not block inventory processing | RabbitMQ test container |

### End-to-End Tests

| Scenario | Verification |
|----------|-------------|
| Periodic happy path: scheduler → periodic queue → worker → master → DB | TripDetails appear in DB with correct data, correlation ID, refresh_type=periodic |
| Inventory-change happy path: event → inventory queue → worker → master → DB | TripDetails appear in DB with correct data, refresh_type=inventory_change |
| Queue isolation: flood periodic queue, send inventory-change event | Inventory-change task completes within 2 minutes despite periodic backlog |
| Cross-queue race: inventory-change and periodic for same route+date | Only newer data persists; both workers complete without error |
| Stale rejection: two workers for same route, one finishes later | Only newer data persists; older data rejected with 409 |
| Retry and periodic DLQ: inventory permanently down | Message eventually dead-lettered in periodic DLQ; failure reported |
| Retry and inventory DLQ: inventory permanently down for change event | Message dead-lettered in inventory DLQ (faster retry cycle) |
| Rate limiting: burst of worker pushes from both queues | Excess requests receive 429; workers retry; all eventually persist |
| Scheduler restart: crash mid-cycle | After restart, no duplicate tasks (dedup keys still valid) |
| Worker crash: mid-processing | Message redelivered from respective queue to another worker |

### Test Infrastructure

- **docker-compose.test.yml**: RabbitMQ and Dragonfly containers for integration tests
- **Mocks**: Generated or hand-written implementations of domain port interfaces
- **Test data**: JSON fixtures for routes, inventory responses, TripDetails payloads, inventory-change events
- **Idempotency tests**: Run the same push request 3× sequentially; verify DB state unchanged after first
- **Isolation tests**: Publish 1000 periodic messages, then 1 inventory-change message; verify inventory message is processed within expected latency

---

## 14. Performance and Scalability

### Scale Estimates

| Dimension | Estimate | Notes |
|-----------|----------|-------|
| Active routes | ~500 | Routes with any booking activity |
| Travel dates per cycle | 40 | Fixed window |
| Total route×date combinations | ~20,000 | Upper bound per cycle |
| Periodic tasks after deduplication | ~2,000–5,000 | Only stale/unrefreshed combinations |
| Periodic tasks per second (sustained) | ~8–10 | 5,000 tasks / 600 seconds (10 min) |
| Inventory-change events per hour | Variable (10–500) | Depends on booking activity |

### Scheduler Efficiency

- **Avoid scanning all 20,000 combinations every cycle**: The scheduler maintains a refresh schedule in Dragonfly:
  - Track `last_refreshed:{routeID}:{date}` with the timestamp of last successful refresh
  - On each cycle, only generate tasks for combinations where `last_refreshed` is older than the required refresh interval for their priority tier:
    - P1: refresh every cycle (10 min)
    - P2: refresh every 20 min
    - P3: refresh every 30 min
    - P4: refresh every 1 hour
    - P5: refresh every 2 hours
    - P6: refresh every 4 hours
  - This dramatically reduces per-cycle task volume

- **Batch operations**: Scheduler reads route data in bulk, classifies in-memory, and publishes messages in batches to RabbitMQ (not one-by-one)

### Worker Scaling

| Workers | Inventory Goroutines/Worker | Periodic Goroutines/Worker | Total Concurrent | Notes |
|---------|---------------------------|---------------------------|------------------|-------|
| 1 | 5 | 10 | 15 | Minimum viable deployment |
| 3 | 5 | 10 | 45 | Standard deployment |
| 5 | 5 | 10 | 75 | High-volume deployment |
| 10 | 5 | 10 | 150 | Peak/scaling deployment |

- Workers scale linearly because they are stateless and share no local state
- Inventory consumer goroutines scale with worker count, maintaining isolation even under load
- Bottleneck shifts to: Inventory/BusMap API capacity → orbitplusservice rate limit → DB write capacity

### Queue Backpressure

- **Periodic queue depth monitoring**: Alert if depth exceeds 2× expected cycle volume
- **Inventory queue depth monitoring**: Alert if depth exceeds 10 (should drain quickly)
- **Prefetch as flow control**: Each consumer group only pulls `prefetch_count` messages; if processing slows, queue naturally buffers
- **No unbounded growth**: Deduplication prevents duplicate tasks; TTL on P6 messages prevents infinite queueing
- **Scaling trigger**: If average periodic queue drain time exceeds one scheduler cycle (10 min), add worker instances
- **Inventory queue never starved**: Dedicated goroutines ensure inventory-change messages are always being consumed regardless of periodic load

### orbitplusservice and Database Capacity

- Rate limiter protects DB from burst writes (from both queue paths combined)
- Conditional UPDATE (version comparison) is a single indexed query — highly efficient
- Database index: composite index on `(route_key, travel_date)` for fast lookups
- Connection pooling: orbitplusservice maintains a connection pool sized to the rate limit (e.g., 100 connections for 100 RPS)

### Resource Efficiency

- Scheduler is lightweight: runs every 10 min, mostly Dragonfly reads + RabbitMQ publishes
- Workers are I/O-bound (waiting on HTTP): goroutines are cheap; high concurrency with low memory
- Dragonfly operations are sub-millisecond: dedup checks add negligible overhead
- TripDetails JSON is composed in-memory: no disk I/O in the worker
- Dual-queue consumption adds minimal overhead: two RabbitMQ channels per worker, independent goroutine pools

---

## Constitution Compliance (Post-Design Verification)

| Principle | Verification |
|-----------|-------------|
| I. Proactive Freshness | ✅ 40-day window, 10-min cycles, priority-based refresh intervals + inventory-change events for immediate freshness |
| II. Worker-Based Processing | ✅ Dedicated workers consuming from both queues, horizontal scaling, ACK-after-delivery |
| III. Reliable Message Delivery | ✅ Publisher confirms, manual ACK, per-queue retry queues, per-queue dead-letter flows, no silent loss |
| IV. Minimal Dependencies | ✅ Only Go stdlib + Gin + amqp091-go (RabbitMQ client) + go-redis/redis (Dragonfly client compatible) |
| V. Observability & Traceability | ✅ Correlation IDs end-to-end, per-queue metrics, structured logging with source_queue label, alerting rules |

---

## Complexity Tracking

No constitution violations detected. All design decisions align with the 5 principles and technology standards.

| Design Decision | Justification |
|----------------|---------------|
| Two queues instead of one | Required for isolation — inventory-change urgency must not be blocked by periodic backlog |
| Separate DLQs per queue | Enables different monitoring thresholds and failure response for urgent vs bulk tasks |
| Separate retry queues per queue | Messages must route back to their origin queue to maintain isolation |
| Dedicated goroutines per queue | Guarantees inventory-change processing capacity regardless of periodic load |

---

## Next Steps

This plan is ready for `/speckit.tasks` to decompose into implementation tasks.
