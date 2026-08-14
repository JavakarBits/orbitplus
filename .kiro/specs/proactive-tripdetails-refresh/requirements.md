# Requirements Document

## Introduction

The Proactive TripDetails Refresh feature maintains current TripDetails for eligible trips whose departure dates fall within a configurable forward-looking calendar window. The feature accepts both inventory-change events and scheduled refresh decisions, coalesces overlapping work, executes refreshes through horizontally scalable Workers, and persists accepted results through the orbitplusservice Master Service. The requirements cover functional behavior, reliability, freshness ordering, downstream-load protection, APIs, security, observability, and implementation readiness for a production-grade Go service.

The default refresh window is 40 calendar days. The window is configurable without a code change. The current calendar date and UTC timestamps are used consistently for eligibility and ordering decisions.

## Glossary

- **Refresh_System**: The complete feature comprising the Scheduler, Event_Ingestor, Durable_Message_Broker, Worker, and Master_Service interactions.
- **Scheduler**: The component that periodically evaluates eligible routes and departure dates and creates Scheduled_Refresh requests.
- **Event_Ingestor**: The component that authenticates and accepts inventory-change notifications and creates Event_Driven_Refresh requests.
- **Worker**: A horizontally scalable execution component that consumes Refresh_Requests, obtains authorized source data, builds TripDetails, and submits results to the Master_Service.
- **Master_Service**: The orbitplusservice component that authenticates, validates, deduplicates, orders, rate-limits, and persists Worker results.
- **Durable_Message_Broker**: The durable work-delivery service that stores Refresh_Requests until acknowledgement or dead-lettering.
- **Inventory_Source**: The authoritative service that provides inventory and bus-map data and emits inventory-change notifications.
- **TripDetails**: The complete persisted trip representation for one route and departure date.
- **Route**: An ordered origin and destination pair identifying a travel path.
- **Departure_Date**: A calendar date on which a trip departs.
- **Refresh_Window**: The inclusive interval from the current calendar date through the date obtained by adding `refresh_window_days`; the default `refresh_window_days` value is 40.
- **Refresh_Key**: The normalized identity formed from Route and Departure_Date; all requests for the same Refresh_Key address the same TripDetails record.
- **Refresh_Request**: A durable unit of work containing a Refresh_Key, trigger type, priority, correlation ID, attempt metadata, and freshness context.
- **Scheduled_Refresh**: A Refresh_Request created by the Scheduler.
- **Event_Driven_Refresh**: A Refresh_Request created from an Inventory_Source change notification.
- **Coalescing**: Combining multiple eligible Refresh_Requests for one Refresh_Key into one active execution while retaining the most urgent trigger and required freshness context.
- **Freshness_Version**: A totally ordered value supplied by the Inventory_Source or derived from a UTC observation timestamp with a deterministic tie-breaker; it determines whether one TripDetails result is newer than another.
- **At_Least_Once_Delivery**: A delivery guarantee under which a Refresh_Request is retried or redelivered until it is accepted, classified as an idempotent no-op, or placed in a dead-letter state.
- **Transient_Failure**: A failure likely to succeed after delay, including timeouts, temporary unavailability, connection failures, and explicit rate limiting.
- **Permanent_Failure**: A failure that cannot succeed without changing the request or configuration, including invalid identity, malformed data, and confirmed non-existent source data.
- **Dead_Letter**: A durable terminal record for a request that exhausted configured recovery attempts or was rejected as permanently unprocessable.
- **Refresh_Submission_API**: The authenticated Master_Service API through which a Worker submits TripDetails results.
- **Failure_Reporting_API**: The authenticated Master_Service API through which a Worker reports a Permanent_Failure.
- **Observability_Data**: Structured logs, metrics, traces, and audit records emitted by the Refresh_System.
- **Load_Control_Policy**: Configured limits for admission rate, concurrency, batching, queue depth, and retry behavior that protect downstream services.
- **Architecture_Decision_Record**: A document recording an architectural choice, alternatives considered, decision rationale, consequences, and rejected options.
- **Go_Implementation_Plan**: The production implementation plan describing Go package boundaries, interfaces, operational behavior, test strategy, and delivery sequencing.

## Requirements

### Requirement 1: Refresh window and eligibility

**User Story:** As a trip consumer, I want upcoming TripDetails to be maintained within a configurable horizon, so that the system provides useful data before departure.

#### Acceptance Criteria

1. THE Refresh_System SHALL expose `refresh_window_days` as a positive configuration value with a default of 40.
2. WHEN the Scheduler evaluates a Departure_Date, THE Scheduler SHALL consider the Departure_Date eligible only when the Departure_Date is on or after the current calendar date and on or before the current calendar date plus `refresh_window_days`.
3. WHEN the Scheduler evaluates an inactive Route or a Route without an eligible Departure_Date, THE Scheduler SHALL create no Scheduled_Refresh for that Route.
4. IF `refresh_window_days` is zero, negative, malformed, or unavailable at startup, THEN THE Refresh_System SHALL reject the configuration and report a startup error before accepting work.
5. WHEN the Refresh_System evaluates calendar eligibility across components, THE Refresh_System SHALL use one configured time zone and one injected clock source for the decision.

### Requirement 2: Scheduled refresh production

**User Story:** As an operator, I want scheduled refresh decisions to run continuously, so that eligible TripDetails remain fresh even when no inventory event occurs.

#### Acceptance Criteria

1. THE Scheduler SHALL expose a configurable schedule with a default interval of 10 minutes.
2. WHEN a scheduled cycle starts, THE Scheduler SHALL evaluate every active Route and every eligible Departure_Date against the configured freshness policy, and scheduled work already identified SHALL remain eligible if the Route later becomes inactive.
3. WHEN a scheduled cycle completes, THE Scheduler SHALL record its start time, completion time, candidate count, coalesced count, published count, and failure count, including a failed cycle with an empty candidate set.
4. IF a scheduled cycle cannot complete for any dependency, internal, or timeout failure, THEN THE Scheduler SHALL record the failed cycle outcome and retain its candidate set, including an empty set, for later retry.
5. WHILE a previous scheduled cycle is still running, THE Scheduler SHALL prevent a second active cycle from publishing duplicate work for the same Refresh_Key, regardless of whether the previous cycle has zero, one, or more candidates.

### Requirement 3: Inventory-driven refresh production

**User Story:** As a trip consumer, I want inventory changes to trigger a refresh, so that material changes are reflected before the next scheduled cycle.

#### Acceptance Criteria

1. WHEN the Inventory_Source emits an authenticated inventory-change notification for a Route and Departure_Date, THE Event_Ingestor SHALL create or update an Event_Driven_Refresh for the corresponding Refresh_Key.
2. WHEN an inventory-change notification identifies a Departure_Date outside the Refresh_Window, THE Event_Ingestor SHALL accept the notification for audit and create no executable Event_Driven_Refresh.
3. WHEN an inventory-change notification identifies malformed or unknown Route or Departure_Date data, THE Event_Ingestor SHALL return a validation error and create no executable Event_Driven_Refresh.
4. WHEN an Event_Driven_Refresh and a Scheduled_Refresh address the same Refresh_Key, THE Refresh_System SHALL preserve the event trigger and its freshness context while coalescing the work into one active execution.
5. WHEN an Event_Driven_Refresh is accepted, THE Refresh_System SHALL make the request eligible for worker consumption independently of the age of the scheduled queue.

### Requirement 4: Tiering and priority

**User Story:** As an operator, I want the system to prioritize urgent and high-demand trips, so that constrained capacity is used where it has the greatest freshness value.

#### Acceptance Criteria

1. THE Scheduler SHALL classify each eligible Route into exactly one of HOT, WARM, or COLD using configured booking-volume rules.
2. THE Scheduler SHALL classify each eligible Departure_Date into exactly one of HOT, WARM, or COLD using configured calendar-distance rules.
3. WHEN a Route tier and Departure_Date tier are available, THE Scheduler SHALL assign a priority from P1 through P6 using a configured priority matrix.
4. WHEN a Route tier and Departure_Date tier are available, THE Scheduler SHALL assign one configured priority from P1 through P6 for every tier combination, including mixed combinations.
5. THE Scheduler SHALL permit configuration to assign any priority from P1 through P6 to any Route-tier and Departure_Date-tier combination.
6. IF route-volume data is unavailable for a scheduled cycle, THEN THE Scheduler SHALL apply the configured fallback tier and record the fallback in Observability_Data, and a failure to record the fallback SHALL not prevent scheduling.
7. WHEN a request is coalesced, THE Refresh_System SHALL retain the highest urgency among its contributing triggers and priority values.

### Requirement 5: Master and Worker responsibilities

**User Story:** As a platform owner, I want clear Master_Service and Worker boundaries, so that persistence remains controlled and execution can scale independently.

#### Acceptance Criteria

1. THE Worker SHALL consume Refresh_Requests, retrieve authorized Inventory_Source data, construct TripDetails, and submit the result through the Refresh_Submission_API.
2. THE Master_Service SHALL authenticate and validate every Refresh_Submission_API request before evaluating persistence.
3. THE Master_Service SHALL persist valid accepted TripDetails and the associated Freshness_Version for the Refresh_Key.
4. THE Worker SHALL access TripDetails persistence only through the Refresh_Submission_API.
5. WHEN the Worker reports a Permanent_Failure, THE Worker SHALL actively submit exactly one Failure_Reporting_API record containing the original correlation ID, Refresh_Key, failure category, retry count, trigger type, and actionable detail before terminal handling.
6. WHEN the Master_Service receives a duplicate or stale result, THE Master_Service SHALL return an idempotent outcome that permits the Worker to complete the corresponding durable request.

### Requirement 6: Deduplication and coalescing

**User Story:** As a downstream service owner, I want duplicate triggers to be coalesced, so that one trip refresh does not cause redundant source calls or writes.

#### Acceptance Criteria

1. THE Refresh_System SHALL use the normalized Refresh_Key as the coalescing identity across scheduled and event-driven triggers.
2. WHEN multiple accepted triggers for one Refresh_Key are pending or in progress, THE Refresh_System SHALL enforce a maximum of one active refresh execution for new requests while allowing existing executions to complete naturally.
3. WHEN a trigger is coalesced into an existing request, THE Refresh_System SHALL retain the latest known Freshness_Version and the highest urgency metadata from all contributing triggers.
4. WHEN a coalescing record expires or is lost because of a system failure, THE Refresh_System SHALL block a new request for that Refresh_Key until expiration or loss processing completes, then permit a later request and rely on idempotent Master_Service ordering to preserve correctness.
5. IF the Refresh_System cannot preserve the highest urgency metadata during coalescing, THEN THE Refresh_System SHALL reject coalescing for the affected triggers and process the triggers individually.
6. IF a coalescing operation cannot determine whether a request already exists, THEN THE Refresh_System SHALL fail closed for publication, record the ambiguity, and retry the decision without silently discarding the trigger.

### Requirement 7: At-least-once delivery and acknowledgement

**User Story:** As an operator, I want recoverable delivery, so that infrastructure failures do not silently lose refresh work.

#### Acceptance Criteria

1. THE Durable_Message_Broker SHALL retain each accepted Refresh_Request until the Refresh_System acknowledges successful processing, an idempotent no-op, or terminal dead-letter handling.
2. WHEN the Refresh_System has completed successful processing, THE Worker SHALL retry the broker acknowledgement indefinitely until the broker confirms it.
3. WHEN a Worker loses its process or connection before acknowledgement, THE Durable_Message_Broker SHALL make the unacknowledged Refresh_Request available for redelivery.
4. WHEN a Worker submits a result successfully or receives an idempotent stale or duplicate outcome from the Master_Service, THE Worker SHALL acknowledge the corresponding Refresh_Request.
5. IF processing ends with a non-success failure outcome, THEN THE Worker SHALL not acknowledge the Refresh_Request and SHALL not retry its broker acknowledgement.
6. IF a Worker cannot confirm successful publication of a retry or acknowledgement after processing has completed, THEN THE Worker SHALL treat the Refresh_Request as logically acknowledged, SHALL not intentionally republish the request, and SHALL rely on Master_Service idempotency for any broker redelivery.
7. THE Refresh_System SHALL provide At_Least_Once_Delivery for both Scheduled_Refresh and Event_Driven_Refresh requests.

### Requirement 8: Freshness and version ordering

**User Story:** As a data consumer, I want newer TripDetails to win over delayed results, so that out-of-order workers cannot overwrite current information with stale data.

#### Acceptance Criteria

1. THE Worker SHALL include a Freshness_Version, source observation time, fetch time, Refresh_Key, and correlation ID in every Refresh_Submission_API request.
2. WHEN the Master_Service compares an incoming Freshness_Version with the stored value for the same Refresh_Key, THE Master_Service SHALL persist the incoming result only when the incoming value is greater and SHALL route an equal value to duplicate handling.
3. WHEN an incoming Freshness_Version equals the stored value, THE Master_Service SHALL return an idempotent duplicate outcome without changing the stored TripDetails.
4. WHEN an incoming Freshness_Version is less than the stored value, THE Master_Service SHALL reject the result as stale without changing the stored TripDetails.
5. THE Master_Service SHALL perform freshness comparison and persistence as one atomic operation for each Refresh_Key.
6. IF the Inventory_Source supplies no monotonic revision, THEN THE Refresh_System SHALL reject operation until both a deterministic UTC timestamp rule and a deterministic tie-breaker ordering rule are configured, and THE Go_Implementation_Plan SHALL document their clock-skew behavior.

### Requirement 9: Controlled downstream load

**User Story:** As a downstream service owner, I want refresh traffic bounded and fairly admitted, so that proactive work does not impair interactive or inventory operations.

#### Acceptance Criteria

1. THE Master_Service SHALL enforce a configurable Load_Control_Policy for Refresh_Submission_API requests.
2. WHEN the configured admission or concurrency limit is exhausted, THE Master_Service SHALL return a retryable rate-limit outcome and SHALL not return SUCCESS for the request.
3. WHEN a Worker receives a retryable rate-limit outcome, THE Worker SHALL delay and retry the request without increasing concurrency beyond its configured limit.
4. THE Scheduler SHALL bound the number of Refresh_Requests published in one cycle according to a configurable batch or rate limit.
5. THE Refresh_System SHALL reserve processing capacity for Event_Driven_Refresh requests so that scheduled backlog cannot consume all available refresh capacity.
6. THE Refresh_System SHALL expose queue age, queue depth, in-flight work, retry volume, and downstream rejection metrics for load-control decisions.

### Requirement 10: Horizontal scaling and coordination

**User Story:** As a platform owner, I want to add Worker and Master_Service instances without changing refresh correctness, so that capacity can grow with demand.

#### Acceptance Criteria

1. THE Worker SHALL process Refresh_Requests without relying on process-local state for correctness.
2. WHEN multiple Worker instances consume the same request stream, THE Durable_Message_Broker SHALL distribute work and the Master_Service SHALL preserve idempotent outcomes under concurrent submissions.
3. WHEN Worker capacity is increased within configured resource limits, THE Refresh_System SHALL increase end-to-end processing capacity without requiring a change to Refresh_Key semantics.
4. THE Master_Service SHALL prevent stale overwrites for every Refresh_Key, regardless of whether API requests for the Refresh_Key are concurrent.
5. THE Scheduler SHALL coordinate active-cycle ownership so that at most one scheduler leader publishes a given scheduled decision at a time.

### Requirement 11: API contracts

**User Story:** As an integrating service owner, I want stable and explicit APIs, so that producers, Workers, and operators can integrate safely.

#### Acceptance Criteria

1. THE Event_Ingestor SHALL expose an authenticated inventory-change API whose request schema includes Route, Departure_Date, event identity, source event time, and correlation ID.
2. THE Refresh_Submission_API SHALL require Route, Departure_Date, TripDetails, Freshness_Version, source metadata, and correlation ID in its request schema.
3. THE Failure_Reporting_API SHALL require Refresh_Key, correlation ID, failure category, retry count, source queue or trigger type, and an operator-actionable detail.
4. WHEN an API request is accepted, THE applicable API SHALL return a SUCCESS outcome that identifies the request and its correlation ID.
5. IF an API request fails any validation rule or authentication, THEN THE applicable API SHALL return a documented client error without invoking source retrieval or persistence.
6. WHEN an API request succeeds at authentication and validation, THE applicable API SHALL invoke only the operations defined by its contract.
6. THE API contracts SHALL define timeout behavior, retryable status categories, idempotency behavior, size limits, schema versioning, and backwards-compatibility rules.

### Requirement 12: Failure handling and recovery

**User Story:** As an operator, I want failures classified and recoverable, so that transient outages recover automatically and permanent failures remain actionable.

#### Acceptance Criteria

1. THE Worker SHALL classify each failure as exactly one of Transient_Failure or Permanent_Failure using documented rules.
2. WHEN a Transient_Failure occurs, THE Worker SHALL retry with bounded exponential backoff and jitter up to the configured attempt limit.
3. WHEN all configured retry attempts are truly exhausted, THE Refresh_System SHALL create a Dead_Letter that preserves all available failure context, including Refresh_Key, trigger type, attempts, failure reason, and correlation ID when available, and SHALL not dead-letter the request earlier.
4. WHEN a Permanent_Failure occurs, THE Worker SHALL immediately attempt to submit one Failure_Reporting_API record before terminally acknowledging or dead-lettering the request.
5. IF the Failure_Reporting_API is unavailable, THEN THE Worker SHALL preserve the failure report for retry and SHALL NOT silently discard the original failure context.
6. THE Refresh_System SHALL provide an operator-controlled replay path that preserves the original correlation ID and SHALL reject a replay when that correlation ID cannot be preserved.
7. WHEN the Scheduler, Durable_Message_Broker, Inventory_Source, or Master_Service recovers after an outage, THE Refresh_System SHALL resume pending work without requiring deletion of durable requests.

### Requirement 13: Observability and auditability

**User Story:** As an operator, I want end-to-end visibility, so that refresh correctness and latency can be diagnosed from trigger to persistence.

#### Acceptance Criteria

1. THE Refresh_System SHALL assign or propagate one correlation ID across Scheduler or Event_Ingestor, Durable_Message_Broker, Worker, Inventory_Source calls, Master_Service APIs, and persistence records.
2. THE Refresh_System SHALL emit structured Observability_Data for request accepted, coalesced, published, consumed, retried, submitted, persisted, stale, duplicate, rate-limited, failed, and dead-lettered outcomes.
3. THE Refresh_System SHALL expose metrics for eligible candidates, trigger counts, coalescing ratio, queue depth, oldest queue age, processing latency, source latency, API latency, success rate, stale rate, retry rate, dead-letter rate, and downstream rejections.
4. THE Refresh_System SHALL expose distributed trace context for a refresh execution and correlate trace spans with the correlation ID.
5. WHEN an operator actually queries a correlation ID, THE Refresh_System SHALL return the request path and terminal outcome within 30 seconds under normal observability operation and SHALL reject that query when the bound cannot be met.
6. THE Refresh_System SHALL redact credentials, access tokens, and configured sensitive fields from logs, traces, metrics, and failure details.

### Requirement 14: Security and data protection

**User Story:** As a security owner, I want refresh interfaces and credentials protected, so that proactive processing does not introduce unauthorized access or data disclosure.

#### Acceptance Criteria

1. THE Refresh_System SHALL authenticate every producer, Worker, operator, and service-to-service API request before accepting protected operations.
2. THE Refresh_System SHALL authorize every protected operation for every caller against the minimum permission required for that operation.
3. THE Worker SHALL retrieve source credentials through an approved secret-management interface and SHALL not persist plaintext credentials in durable messages, logs, or TripDetails.
4. THE Refresh_System SHALL maintain encrypted service-to-service transport and certificate or token validation as an active baseline security control at all times.
5. THE Master_Service SHALL reject any payload that fails size, field-format, route-identity, Departure_Date, Freshness_Version, or schema-version validation and SHALL not persist that payload.
6. THE Refresh_System SHALL always record security-relevant authentication, authorization, replay, and credential-access failures and SHALL exclude secret material from each record.

### Requirement 15: Production-grade Go implementation readiness

**User Story:** As an engineering team, I want an implementation-ready Go plan, so that the feature can be delivered consistently and operated safely in production.

#### Acceptance Criteria

1. THE Go_Implementation_Plan SHALL define domain, application, and infrastructure boundaries for Scheduler, Event_Ingestor, Worker, Master_Service adapters, message handling, and persistence interfaces.
2. THE Go_Implementation_Plan SHALL define context cancellation, request deadlines, graceful shutdown, bounded goroutine lifetimes, and connection-draining behavior.
3. THE Go_Implementation_Plan SHALL define typed error categories, retry eligibility, backoff limits, idempotency handling, and metrics labels without exposing sensitive values.
4. THE Go_Implementation_Plan SHALL define unit, property, integration, contract, load, race-detection, and failure-recovery tests for every externally observable requirement.
5. THE Go_Implementation_Plan SHALL define configuration validation, versioned API schemas, dependency health checks, rollout controls, and backward-compatible migration steps.
6. THE Go_Implementation_Plan SHALL define resource limits for memory, network connections, worker concurrency, message prefetch, payload size, and queue backlog.
7. THE Go_Implementation_Plan SHALL define deterministic clock and dependency interfaces so that date-window, retry, coalescing, and freshness-order tests do not depend on wall-clock timing or live services.

### Requirement 16: Architectural trade-offs and decisions

**User Story:** As an architecture reviewer, I want explicit trade-off records, so that reliability and operational choices remain understandable and revisable.

#### Acceptance Criteria

1. THE Architecture_Decision_Record SHALL compare isolated event-driven and scheduled work streams with a single shared stream and document the selected approach.
2. THE Architecture_Decision_Record SHALL compare broker-priority ordering with separate priority queues and document starvation, fairness, and operational consequences.
3. THE Architecture_Decision_Record SHALL compare centralized, distributed, and broker-backed coalescing state and document behavior when coordination state is unavailable.
4. THE Architecture_Decision_Record SHALL compare local and distributed Load_Control_Policy enforcement and document scaling and failure consequences.
5. THE Architecture_Decision_Record SHALL compare source revision ordering with timestamp ordering and document clock-skew, replay, and out-of-order delivery behavior.
6. THE Architecture_Decision_Record SHALL identify durable state, ephemeral state, recovery guarantees, cost implications, and rejected alternatives for each selected decision.
7. THE Architecture_Decision_Record SHALL permit recording the decision date, owner, reason, migration impact, and compatibility assessment at any time, including when `decision_changed` is false.

## Measurable Success Criteria

- **SC-001:** With the default configuration, 100% of active Route and Departure_Date combinations inside the inclusive 40-day Refresh_Window are evaluated during every completed scheduled cycle.
- **SC-002:** Under a declared normal-load profile, at least 99% of Event_Driven_Refresh requests are available for Worker consumption within 60 seconds of accepted ingestion.
- **SC-003:** Under a declared normal-load profile, at least 99% of P1 Scheduled_Refresh requests are submitted to the Master_Service within 5 minutes of publication.
- **SC-004:** Across a 24-hour normal-load run, no accepted Refresh_Request is silently lost; every request reaches accepted, idempotent no-op, retry-pending, or Dead_Letter state.
- **SC-005:** In generated duplicate-trigger tests, coalescing limits active execution to one per Refresh_Key and reduces duplicate source fetches by at least 99%.
- **SC-006:** In concurrent ordering tests, 100% of lower or equal Freshness_Version submissions leave the greatest stored version unchanged.
- **SC-007:** Under the configured downstream limit, the Master_Service produces no more successful submissions than the Load_Control_Policy permits in any measurement interval.
- **SC-008:** A scale test with 1, 2, 5, and 10 Worker instances demonstrates increasing throughput without an increase in stale-overwrite, duplicate-persistence, or lost-request rates; the target throughput and saturation point are recorded.
- **SC-009:** For a representative failure-injection run covering broker, source, credential, and Master_Service outages, 100% of requests are recovered, retried, or dead-lettered with required context.
- **SC-010:** For a sampled refresh execution, an operator can retrieve the complete correlation-linked path from trigger through persistence or terminal failure within 30 seconds.
- **SC-011:** Security tests show that unauthenticated, unauthorized, oversized, malformed, and secret-bearing requests are rejected or redacted according to the documented API and security contracts.
- **SC-012:** The Go_Implementation_Plan is complete when each acceptance criterion maps to at least one automated test, one operational check, or an explicitly documented architecture decision.

## Assumptions and Constraints

- The Inventory_Source provides active routes, departure dates, inventory data, and either a monotonic source revision or sufficient observation metadata for deterministic freshness ordering.
- The Master_Service owns TripDetails persistence and is the only component allowed to write the TripDetails store.
- A durable message-delivery capability and a coordination/state capability are available in the deployment; their concrete products are selected during design and recorded in Architecture_Decision_Record.
- The default 40-day window, 10-minute schedule, tier thresholds, retry limits, rate limits, batch limits, and retention periods are configuration values with validated deployment-specific overrides.
- Event-driven refresh is in scope and is not excluded by the scheduled-refresh design.
- The feature does not define the internal schema of existing Inventory_Source data beyond the fields required by the API and Freshness_Version contracts.
- The design and implementation must preserve the stated responsibilities, delivery guarantees, security controls, and measurable outcomes even when concrete infrastructure choices change.
