# Feature Specification: Proactive TripDetails Refresh System

**Feature Branch**: `001-proactive-tripdetails-refresh`

**Created**: 2026-08-12

**Status**: Draft

**Input**: User description: "Create the specification for the Proactive TripDetails Refresh System that proactively maintains TripDetails for upcoming 40-day trips using tiered priority scheduling, RabbitMQ workers, and reliable delivery to orbitplusservice."

## User Scenarios & Testing

### User Story 1 - Priority-Based Periodic Scheduling (Priority: P1)

Every 10 minutes, the Scheduler identifies route + travel-date combinations that need TripDetails refresh within the upcoming 40-day window. Routes are classified as HOT, WARM, or COLD based on booking volume from top_route data. Travel dates are classified as HOT, WARM, or COLD based on proximity to today. The combined tier determines priority (P1–P6), and refresh tasks are published to RabbitMQ in priority order.

**Why this priority**: Without the scheduler, no proactive refresh happens. This is the core trigger for the entire system and directly enforces the constitution's "Proactive Freshness" principle.

**Independent Test**: Can be fully tested by running the scheduler against a known set of routes and dates, then verifying that correct-priority messages appear in RabbitMQ queues without any downstream processing.

**Acceptance Scenarios**:

1. **Given** routes exist with booking volume data and travel dates within the next 40 days, **When** the scheduler runs its 10-minute cycle, **Then** refresh tasks are published to RabbitMQ with priorities P1–P6 based on route tier × date tier.
2. **Given** a HOT route with a HOT travel date (departure within 3 days, high booking volume), **When** the scheduler evaluates priority, **Then** the task is assigned P1 (highest priority).
3. **Given** a COLD route with a COLD travel date, **When** the scheduler evaluates priority, **Then** the task is assigned P6 (lowest priority).
4. **Given** a refresh task already exists for a specific route + travel-date combination within the current cycle, **When** the scheduler attempts to create another task for the same combination, **Then** the duplicate is suppressed and no new message is published.
5. **Given** a travel date is more than 40 days from today, **When** the scheduler evaluates dates, **Then** that date is excluded from refresh consideration.

---

### User Story 2 - Worker Processing Pipeline (Priority: P1)

Dedicated Worker Services consume refresh tasks from RabbitMQ, obtain credentials from Dragonfly/Orbit, fetch the latest TripDetails from the Inventory/BusMap system, generate the TripDetails JSON, and push the result to orbitplusservice. Workers retry temporary failures and report permanent failures to the Master failure API.

**Why this priority**: Workers are the execution engine — without them, scheduled tasks accumulate but no TripDetails are actually refreshed. Co-equal with the scheduler for a minimum viable system.

**Independent Test**: Can be tested by placing a known refresh task message on RabbitMQ and verifying the worker fetches data, composes TripDetails JSON, and delivers it to orbitplusservice (or a mock endpoint).

**Acceptance Scenarios**:

1. **Given** a refresh task message is available in RabbitMQ, **When** a worker picks it up, **Then** the worker obtains credentials from Dragonfly/Orbit, fetches current Inventory/BusMap data, generates TripDetails JSON, and pushes it to orbitplusservice.
2. **Given** the Inventory/BusMap system returns a temporary error (timeout, 503), **When** the worker encounters this failure, **Then** the worker retries the operation according to the configured retry policy before giving up.
3. **Given** a permanent failure occurs (invalid route, 404 from Inventory), **When** the worker exhausts retries, **Then** the worker reports the failure to the orbitplusservice failure API and the message moves to the dead-letter queue.
4. **Given** a worker successfully generates TripDetails JSON, **When** it pushes to orbitplusservice, **Then** the worker acknowledges the RabbitMQ message only after receiving a success confirmation from orbitplusservice.
5. **Given** multiple worker instances are running, **When** tasks arrive in RabbitMQ, **Then** tasks are distributed across workers without duplication or conflicts.

---

### User Story 3 - Master Service TripDetails Ingestion (Priority: P2)

The orbitplusservice Master Service receives TripDetails from workers through a dedicated push API, validates the request, prevents duplicates and stale overwrites, applies rate limiting, and persists the TripDetails into the database.

**Why this priority**: The Master Service is the final persistence layer. Without it, workers have nowhere to deliver results. It is P2 because it can be partially stubbed during early development while scheduler and workers are built.

**Independent Test**: Can be tested by sending TripDetails JSON payloads directly to the push API and verifying correct persistence, duplicate rejection, and stale-data prevention without any scheduler or worker running.

**Acceptance Scenarios**:

1. **Given** a valid TripDetails JSON payload arrives at the push API, **When** orbitplusservice processes it, **Then** the TripDetails are persisted to the database and a success response is returned.
2. **Given** a TripDetails payload arrives for a route + date that already has newer data (based on timestamp/version), **When** orbitplusservice compares versions, **Then** the older payload is rejected and existing data is preserved.
3. **Given** a duplicate TripDetails payload arrives (same route + date + version), **When** orbitplusservice processes it, **Then** the duplicate is rejected without error and no redundant write occurs.
4. **Given** the push API is receiving requests above the configured rate limit, **When** additional requests arrive, **Then** excess requests are rejected with an appropriate rate-limit response and workers back off.
5. **Given** an invalid or malformed TripDetails payload arrives, **When** orbitplusservice validates it, **Then** the request is rejected with a descriptive error response.

---

### User Story 4 - End-to-End Traceability (Priority: P3)

Every refresh task carries a unique task/correlation ID that flows from Scheduler → RabbitMQ → Worker → Inventory/BusMap → orbitplusservice → Database, enabling operators to trace any TripDetails update across the entire pipeline.

**Why this priority**: Traceability is critical for operational support and debugging but does not affect core functional correctness. The system can function without it, though troubleshooting becomes significantly harder.

**Independent Test**: Can be tested by triggering a single refresh task and verifying the same correlation ID appears in scheduler logs, RabbitMQ message headers, worker processing logs, and orbitplusservice persistence logs.

**Acceptance Scenarios**:

1. **Given** the scheduler creates a refresh task, **When** the task is published to RabbitMQ, **Then** a unique correlation ID is embedded in the message metadata.
2. **Given** a worker processes a task with a correlation ID, **When** it logs processing steps and calls external services, **Then** the correlation ID is included in all log entries and outbound request headers.
3. **Given** orbitplusservice receives a TripDetails push, **When** it persists the data, **Then** the correlation ID is recorded alongside the database entry for audit purposes.

---

### User Story 5 - Dead-Letter and Failure Observability (Priority: P3)

Messages that exhaust their retry policy are moved to a dead-letter queue. Permanent processing failures are reported to orbitplusservice's failure API. Operators can monitor queue depth, processing latency, dead-letter volume, and refresh success rates.

**Why this priority**: Failure handling is essential for production reliability but the system can be developed and tested in happy-path scenarios first. This story ensures the system degrades gracefully rather than silently losing data.

**Independent Test**: Can be tested by injecting messages that will always fail (invalid route codes) and verifying they eventually land in the dead-letter queue and appear in the failure API.

**Acceptance Scenarios**:

1. **Given** a message has been retried the maximum configured number of times, **When** the final retry fails, **Then** the message is moved to the dead-letter queue.
2. **Given** a permanent failure is detected by a worker, **When** the worker reports it, **Then** the failure details (correlation ID, route, date, error reason) are sent to the orbitplusservice failure API.
3. **Given** the system is running in production, **When** an operator checks monitoring, **Then** queue depth, processing latency, dead-letter volume, and refresh success rate metrics are available.

---

### Edge Cases

- What happens when RabbitMQ is temporarily unavailable during scheduler publish? Tasks MUST be buffered or retried until the broker is reachable; no tasks are silently dropped.
- What happens when a worker crashes mid-processing? The unacknowledged message MUST be redelivered to another worker by RabbitMQ.
- What happens when Dragonfly is unavailable for deduplication checks? The scheduler MUST either wait/retry or proceed with the risk of duplicates (which workers and orbitplusservice handle idempotently).
- What happens when orbitplusservice is down when a worker tries to push? The worker MUST retry; if retries exhaust, the message goes to dead-letter for later reprocessing.
- What happens when the top_route data source is empty or unavailable? The scheduler MUST treat all routes as COLD tier rather than skipping them entirely.
- What happens when a route has no trips in the 40-day window? No tasks are generated for that route; it is simply not included in the current cycle.
- What happens when the clock drifts between scheduler instances? Travel date tier classification MUST be based on calendar-day boundaries, not precise timestamps, to minimize drift impact.

## Requirements

### Functional Requirements

- **FR-001**: System MUST run a periodic scheduler every 10 minutes that identifies route + travel-date combinations requiring TripDetails refresh within the upcoming 40 days.
- **FR-002**: System MUST classify routes as HOT, WARM, or COLD based on booking volume from top_route data.
- **FR-003**: System MUST classify travel dates as HOT, WARM, or COLD based on proximity to the current date.
- **FR-004**: System MUST combine route tier and date tier to determine refresh priority from P1 (HOT+HOT) to P6 (COLD+COLD).
- **FR-005**: System MUST prevent duplicate refresh tasks for the same route + travel-date combination within a scheduler cycle.
- **FR-006**: System MUST publish refresh tasks to RabbitMQ for asynchronous processing by worker services.
- **FR-007**: Workers MUST obtain required credentials from Dragonfly/Orbit before fetching data.
- **FR-008**: Workers MUST fetch the latest TripDetails from the Inventory/BusMap system.
- **FR-009**: Workers MUST generate TripDetails JSON and push the result to orbitplusservice.
- **FR-010**: Workers MUST retry temporary failures according to the configured retry policy.
- **FR-011**: Workers MUST report permanent failures to the orbitplusservice failure API.
- **FR-012**: Workers MUST NOT directly access the TripDetails database.
- **FR-013**: Workers MUST acknowledge RabbitMQ messages only after successful delivery to orbitplusservice.
- **FR-014**: orbitplusservice MUST receive TripDetails from workers through a dedicated push API.
- **FR-015**: orbitplusservice MUST validate all incoming TripDetails requests.
- **FR-016**: orbitplusservice MUST persist valid TripDetails into the database.
- **FR-017**: orbitplusservice MUST prevent duplicate TripDetails updates for the same route + date + version.
- **FR-018**: orbitplusservice MUST prevent older TripDetails from overwriting newer data (version/timestamp comparison).
- **FR-019**: orbitplusservice MUST rate-limit the TripDetails push API.
- **FR-020**: Messages that exhaust retry policy MUST be moved to a dead-letter queue.
- **FR-021**: Every refresh task MUST carry a unique correlation ID traceable across all pipeline stages.
- **FR-022**: System MUST support multiple concurrent worker instances and horizontal scaling.
- **FR-023**: Dragonfly MUST be used for distributed temporary state (task deduplication, locks, scheduler metadata).

### Key Entities

- **Route**: A travel route identified by origin and destination codes. Has a tier classification (HOT/WARM/COLD) derived from booking volume via top_route data.
- **Travel Date**: A specific date within the 40-day forward window. Has a tier classification (HOT/WARM/COLD) based on proximity to today.
- **Refresh Task**: A unit of work representing the need to refresh TripDetails for a specific route + travel-date combination. Carries a priority (P1–P6), correlation ID, and deduplication key.
- **TripDetails**: The complete trip information JSON containing inventory and bus map data for a specific route on a specific date. Versioned to prevent stale overwrites.
- **Priority Matrix**: The mapping of route tier × date tier to priority level (P1–P6). Determines processing order in the queue.
- **Dead-Letter Entry**: A failed refresh task that exhausted retries, containing the original task details and failure reason for operator inspection.

## Success Criteria

### Measurable Outcomes

- **SC-001**: TripDetails for all active routes within the 40-day window are refreshed at least once per scheduler cycle appropriate to their priority tier.
- **SC-002**: P1 tasks (HOT+HOT) are processed within 5 minutes of being scheduled under normal load.
- **SC-003**: No TripDetails refresh task is silently lost — every task either succeeds, is retried, or lands in the dead-letter queue.
- **SC-004**: Duplicate refresh tasks for the same route + date are suppressed with greater than 99% effectiveness per scheduler cycle.
- **SC-005**: The system handles a sustained throughput of all active routes × 40 days worth of refresh tasks per 10-minute cycle without queue buildup exceeding one cycle's worth of tasks.
- **SC-006**: End-to-end traceability: any TripDetails update can be traced from scheduler decision to database write using its correlation ID within 30 seconds of operator query.
- **SC-007**: Stale TripDetails overwrites are prevented — 100% of version conflicts are correctly rejected by orbitplusservice.
- **SC-008**: Worker horizontal scaling: adding a worker instance reduces average per-task processing time proportionally (linear scaling up to 10 instances).
- **SC-009**: Dead-letter queue volume remains below 1% of total daily task volume under normal operating conditions.
- **SC-010**: System recovers automatically after transient infrastructure failures (RabbitMQ restart, Dragonfly failover) without manual intervention.

## Assumptions

- The `top_route` data source is available and provides booking volume metrics that can be used to classify routes into HOT/WARM/COLD tiers. Tier boundaries (volume thresholds) will be configurable.
- Travel date tier boundaries are configurable (e.g., HOT = 0–3 days, WARM = 4–14 days, COLD = 15–40 days). Exact boundaries will be determined during planning.
- The Inventory/BusMap system provides a stable API for fetching trip data given route and date parameters. Authentication is handled via credentials stored in Dragonfly/Orbit.
- `orbitplusservice` already exists and will expose a new push API endpoint and failure API endpoint for this system. The worker service is the new component being built.
- RabbitMQ supports priority queues or multiple priority-specific queues to ensure higher-priority tasks are processed first.
- Dragonfly provides sub-millisecond key lookups suitable for high-frequency deduplication checks during scheduler cycles.
- The scheduler runs as a single-leader instance (not horizontally scaled) to avoid coordination complexity. Workers are the horizontally scaled component.
- Rate limiting on orbitplusservice is per-endpoint and will be configured to protect the database while allowing sustained worker throughput during peak processing.
- The system does not need to handle inventory-driven (event-triggered) refresh in this specification — that is a separate, future enhancement. This spec covers periodic refresh only.
