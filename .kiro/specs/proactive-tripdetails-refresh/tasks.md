# Implementation Plan: Proactive TripDetails Refresh

## Overview

Implement the approved Go design under `V1/proactive`. Dragonfly is the sole authoritative store for every TripDetails record and all proactive-refresh feature state: only Dragonfly-backed TripDetails, claims, coalescing, triggers/intents, scheduler/candidate state, failure state, audit state, and admission state may be authoritative. Do not introduce SQL/relational databases, Cassandra, Redis, or process-local correctness/recovery state. `Master_Service` is the only TripDetails writer and applies atomic freshness ordering. RabbitMQ is used only for durable delivery, retry, and contextual DLQ handling. The work is deliberately sequenced from foundations through recovery: no task changes the approved requirements or design.

All property tasks use Go's `testing` package and an exactly pinned, approved `pgregory.net/rapid` version. Each runs at least 100 generated cases, supports deterministic seed replay and shrinking, uses injected clocks/dependencies, and tags failures with `Feature: proactive-tripdetails-refresh, Property N: <title>`.

## Tasks

- [ ] 1. Establish the Go feature foundation and deterministic application boundaries
  - [x] 1.1 Create the Go module, package layout, configuration schema, and dependency interfaces
    - Create `V1/go.mod` with exact approved versions and create `V1/proactive/{domain,application,infrastructure,transport,testutil}` boundaries.
    - Define typed configuration/defaults and startup validation for windows, schedule, tier matrix, queue/pool settings, priority weights, age thresholds, version fallback/skew, resource/payload limits, retries, retention, TLS, Dragonfly durability/primary/backup readiness, and RabbitMQ topology.
    - Define deterministic `Clock`, source, secret, broker, Dragonfly state, global-admission, telemetry, trace, and lifecycle ports with typed outcomes/errors. Encode Dragonfly-only authoritative state: Dragonfly-backed TripDetails, claims, coalescing, triggers/intents, scheduler/candidate state, failure state, audit state, and admission state; prohibit SQL/relational databases, Cassandra, Redis, and process-local correctness/recovery state. Encode Master-only TripDetails writes and publisher confirms.
    - _Requirements: 1.1, 1.4, 1.5, 2.1, 5.4, 8.6, 9.1, 11.6, 14.3, 15.1, 15.2, 15.3, 15.5, 15.6, 15.7; selected decisions in 16.1–16.6_

  - [ ]* 1.2 Write foundation configuration and dependency-contract tests
    - Cover defaults, invalid startup configuration, exact dependency interface errors, no plaintext secret configuration exposure, and readiness refusal before accepting work.
    - _Requirements: 1.1, 1.4, 8.6, 14.3, 15.3, 15.5_

- [ ] 2. Implement immutable domain identity, ordering, and scheduling primitives
  - [ ] 2.1 Implement `RefreshKey`, `FreshnessVersion`, immutable request envelopes, and pure eligibility services
    - Add canonical URL-safe normalized route/date `RefreshKey`; immutable request ID, correlation ID, trigger type, source queue, periodic priority, attempt, and reconciliation metadata; configured-zone inclusive date-window evaluation from an injected clock; and total version comparison.
    - Keep fetch time out of freshness ordering. Validate source-revision or timestamp/event-ID fallback with skew bounds before work acceptance.
    - _Requirements: 1.2, 1.3, 1.5, 6.1, 7.7, 8.1, 8.6, 11.5, 14.5, 15.3, 15.7_

  - [ ]* 2.2 Write property test for inclusive calendar eligibility
    - **Property 1: Inclusive calendar eligibility** — generated instants, configured zones, positive windows, and dates are eligible iff they are in `[today, today + window]`.
    - **Validates: Requirements 1.2, 1.5**

  - [ ]* 2.3 Write property test for Refresh_Key canonicalization
    - **Property 8: Refresh_Key canonicalization preserves identity** — equivalent valid route/date representations normalize identically and distinct canonical identities never collide.
    - **Validates: Requirements 6.1**

  - [ ]* 2.4 Write focused freshness-version and immutable-envelope tests
    - Cover deterministic timestamp tie-breaking, future-skew rejection, immutability of request/correlation identity, and preservation of source queue, trigger, and periodic priority fields.
    - _Requirements: 7.7, 8.1, 8.6, 11.2, 12.3, 12.6_

- [ ] 3. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 4. Implement Dragonfly-authoritative state, coalescing, and atomic freshness writes
  - [ ] 4.1 Implement Dragonfly namespaces, fenced claims, publish intents, and atomic coalescing
    - Create versioned, encoded, retention-controlled Dragonfly state for triggers, cycles, candidates, immutable intents, claims, failures, audit, and admission; do not create any relational/Cassandra or Worker-local correctness store.
    - Coalesce exclusively by immutable `Refresh_Key`: inventory-triggered work takes precedence over periodic work; preserve the greatest comparable version, highest urgency, all contributing correlations, and exactly one active fenced claim. Periodic work must never suppress or overwrite inventory-triggered state.
    - Fail closed into durable `COALESCE_UNCERTAIN` on ambiguity; reject unsafe merges into individually auditable intents; allow liveness recovery only after expiry/loss processing.
    - _Requirements: 3.4, 4.7, 6.1–6.6, 10.1, 13.1, 15.1, 15.7_

  - [ ] 4.2 Implement the Dragonfly writable-primary atomic TripDetails compare-and-write primitive
    - Implement the one Master-owned atomic Dragonfly operation/script that validates the ordering scheme, compares `FreshnessVersion`, and writes complete TripDetails/version/audit/expiry only for a greater version; this must not be an application read-then-write sequence and must return non-mutating `DUPLICATE`/`STALE` for equal/lower versions.
    - Require writable-primary role, fencing, durability, replica/lag, backup, and no-eviction health gates; deny Worker credentials from TripDetails write namespaces.
    - _Requirements: 5.3, 8.2–8.5, 10.2, 10.4, 14.4, 15.5, 15.6_

  - [ ]* 4.3 Write property test for single active coalesced execution
    - **Property 4: Coalescing permits at most one active execution** — generated trigger, claim, lease-expiry, and release interleavings for one key never admit more than one active claim.
    - **Validates: Requirements 2.5, 6.2**

  - [ ]* 4.4 Write property test for dominant coalesced metadata
    - **Property 6: Coalescing preserves dominant metadata** — generated comparable trigger sets preserve inventory trigger precedence, greatest known version, highest urgency/priority, and contributing correlations.
    - **Validates: Requirements 3.4, 4.7, 6.3**

  - [ ]* 4.5 Write property test for atomic monotonic TripDetails state
    - **Property 10: Dragonfly TripDetails state is monotonic by version** — generated sequential and concurrent submissions leave the greatest version stored; greater accepts, equal duplicates without payload mutation, and lower is stale without payload mutation.
    - **Validates: Requirements 5.3, 8.2, 8.3, 8.4, 8.5, 10.4**

  - [ ]* 4.6 Write Dragonfly state and ACL integration tests
    - Use a disposable writable primary to verify direct-key retention, coalescing ambiguity recovery, one claim, atomic compare-and-write under concurrency, Master-only TripDetails ACLs, and health-gated failures.
    - _Requirements: 5.3, 6.4–6.6, 8.5, 10.4, 14.4, 15.5, 15.6_

- [ ] 5. Implement Master_Service protected APIs and Dragonfly-coordinated global admission
  - [ ] 5.1 Implement RefreshSubmission, FailureReporting, and authorized TripDetails API contracts
    - Authenticate/authorize every caller; validate schema versions, sizes, formats, normalized route/date, versions, metadata, and correlation before state or source operations.
    - Route all valid TripDetails submissions to the Master-owned atomic primitive, return documented `ACCEPTED`, `DUPLICATE`, `STALE`, retryable, and client/security outcomes, and expose authorized direct Dragonfly lookup without source fetches.
    - Persist idempotent failure reports using original correlation plus failure identity.
    - _Requirements: 5.2, 5.5, 5.6, 11.2–11.6, 14.1, 14.2, 14.5_

  - [ ] 5.2 Implement globally coordinated Master/API admission in Dragonfly
    - Add configurable Dragonfly atomic admission/concurrency counters and leases around `RefreshSubmission`; return retryable rate-limit outcomes without success when exhausted or state health is uncertain.
    - Keep this global Master/API limit separate from Worker-local pools/semaphores: it is shared across Master instances and is never allocated, consumed, or replaced by Worker slots.
    - _Requirements: 9.1, 9.2, 10.2, 10.4, 15.2, 15.6_

  - [ ]* 5.3 Write property test for protected-input isolation
    - **Property 17: Invalid protected input has no downstream effect** — generated unauthenticated, unauthorized, oversized, malformed, unsupported-schema, invalid key/date/version requests return client/security errors without source retrieval or TripDetails writes.
    - **Validates: Requirements 11.5, 14.5**

  - [ ]* 5.4 Write Master API, global-admission, and contract tests
    - Verify deadlines, schema compatibility, idempotency/status semantics, failure-report identity, and a multi-instance global admission cap that cannot be exceeded by concurrent Master instances.
    - _Requirements: 5.2, 5.5, 5.6, 9.1, 9.2, 10.2, 11.2–11.6, 14.1, 14.2_

- [ ] 6. Declare and validate the isolated RabbitMQ topology
  - [ ] 6.1 Implement RabbitMQ topology declaration, bindings, permissions, and validation
    - Declare durable quorum topology for exactly one inventory queue, `inventory.refresh`, and exactly six periodic queues, `periodic.p1` through `periodic.p6`; declare their exchanges/bindings, retry queues/routes, and contextual DLQs.
    - Do not declare RabbitMQ `x-max-priority` anywhere. Inventory uses no P1–P6 priority; periodic uses only the six named queues.
    - Require publisher confirms for initial, retry, DLQ, and replay publication; configure manual acknowledgements, scoped consumer/publisher permissions, durable retry routing, and startup validation that rejects missing, misbound, non-quorum, unauthorized, or extra work queues.
    - _Requirements: 3.5, 7.1–7.7, 9.5, 12.3, 12.7, 14.1, 14.2, 15.5, 15.6; implements selected decisions in 16.1–16.2_

  - [ ]* 6.2 Write RabbitMQ topology declaration and permission tests
    - Use disposable RabbitMQ to verify quorum durability, all bindings, publisher-confirm requirements, contextual retry/DLQ routes, permission denial, absence of `x-max-priority`, one inventory queue, and exactly six periodic queues.
    - _Requirements: 3.5, 7.1–7.7, 9.5, 12.3, 12.7, 14.1, 14.2_

- [x] 7. Implement bounded Worker execution with strictly local concurrency controls
  - [x] 7.1 Implement stateless Worker execution and separate local pool/semaphore controls
    - Consume manually, obtain a fenced per-key claim, retrieve source credentials only through the approved secret interface, fetch authorized data, build TripDetails, and submit source version/observation UTC/fetch UTC/key/correlation only through Master_Service. Workers must be stateless across restarts: all correctness and recovery state must be held in Dragonfly.
    - Implement independently configured reserved local inventory pool for `inventory.refresh` and a local periodic pool for the six periodic queues. Bound goroutines, prefetch, and per-pool semaphores with cancellation/deadlines; Worker-local pools/semaphores may cap live process concurrency only, with no Dragonfly call per Worker slot and no local state for coordination correctness or recovery.
    - Document in code and metrics that Worker pools cap only process-local fetch/submit concurrency, whereas Dragonfly-coordinated Master/API admission is the cross-instance global cap.
    - _Requirements: 5.1, 5.4, 7.3, 8.1, 9.3, 9.5, 10.1–10.3, 13.1, 14.3, 15.2, 15.6_

  - [ ]* 7.2 Write property test for submission context preservation
    - **Property 9: Submission preserves source ordering context** — generated valid source results retain Refresh_Key, Freshness_Version, source observation time, fetch time, and correlation ID in the Worker submission.
    - **Validates: Requirements 8.1**

  - [ ]* 7.3 Write property test for acknowledgement eligibility
    - **Property 11: Submission outcomes determine acknowledgement** — only `ACCEPTED`, `DUPLICATE`, and `STALE` make acknowledgement eligible; every other generated outcome leaves the message unacknowledged.
    - **Validates: Requirements 5.6, 7.4, 7.5**

  - [ ]* 7.4 Write property test for global admission and local Worker concurrency limits
    - **Property 12: Admission and Worker concurrency respect limits** — generated arrivals, Master capacities, inventory/periodic local caps, and retry guidance never exceed the Dragonfly-coordinated global admission cap or the relevant local semaphore cap.
    - **Validates: Requirements 9.2, 9.3**

  - [ ]* 7.5 Write Worker pool and concurrency integration tests
    - Verify reserved local inventory capacity, periodic-pool cap, bounded goroutine lifetimes, no Dragonfly operation per Worker slot, retry never increases local concurrency, and global Master admission remains capped independently of all Worker pools.
    - _Requirements: 9.1–9.3, 9.5, 10.1–10.3, 15.2, 15.6_

- [ ] 8. Implement authenticated inventory-triggered flow and deterministic broker routing
  - [ ] 8.1 Implement authenticated `InventoryChange` ingestion and durable event-trigger creation
    - Validate TLS-authenticated identity, authorization, request/schema/size limits, immutable event identity, route/date, source time, and correlation before source access or publishing.
    - Atomically audit valid in-window events and create/update inventory-trigger/coalescing/publish-intent state before `SUCCESS`; audit valid out-of-window events without executable work and reject invalid input without work.
    - _Requirements: 3.1–3.3, 11.1, 11.4–11.6, 13.1, 14.1, 14.2, 14.4, 14.5_

  - [ ] 8.2 Implement immutable routing, publish-intent dispatch, and confirmation reconciliation
    - Route every `InventoryChange`-derived refresh only to `inventory.refresh`; route every `PeriodicRefresh` P1–P6 only to its matching `periodic.p1`–`periodic.p6` queue. Preserve immutable request ID, correlation ID, Refresh_Key, trigger type, source queue, and periodic priority where applicable.
    - Read Dragonfly-durable pending intents, publish with confirms, atomically mark confirmations, retain `PUBLISH_UNCERTAIN`, and reconcile/reissue only the identical immutable envelope. Do not use RabbitMQ as authoritative state.
    - _Requirements: 3.5, 7.1, 7.7, 9.5, 10.2, 12.7, 13.1, 13.2_

  - [ ]* 8.3 Write property test for side-effect-safe inventory event outcomes
    - **Property 5: Event validation and window outcomes are side-effect safe** — generated notifications create/update executable work only when valid and in-window, audit only when valid/out-of-window, and create no executable work when malformed/unknown.
    - **Validates: Requirements 3.1, 3.2, 3.3**

  - [ ]* 8.4 Write inventory ingestion and publish-confirm integration tests
    - Verify authenticated acceptance, audit-only out-of-window handling, atomic event coalescing, `inventory.refresh` routing only, confirm/unknown-confirm reconciliation, and failure-closed publication.
    - _Requirements: 3.1–3.5, 6.4–6.6, 7.1, 7.7, 13.1, 14.1–14.5_

- [ ] 9. Implement Scheduler cycles, tiering, and periodic-refresh decisions
  - [ ] 9.1 Implement fenced Scheduler cycles and durable candidate recovery
    - Use a configured 10-minute default cycle. From one injected-clock snapshot, evaluate active routes and only departure dates in `[today, today + refresh_window_days]`; persist start/completion, explicit empty candidate sets, counts, cursors, and failed-cycle retry state.
    - Acquire/renew a monotonic fenced lease, stop publication on failed renewal, retain candidates after later route inactivity, and prevent overlapping cycles from publishing duplicate Refresh_Key work.
    - _Requirements: 1.2, 2.1–2.5, 10.5, 13.1, 13.2, 15.2_

  - [ ] 9.2 Implement route/date tiers, configurable P1–P6 matrix, and bounded periodic decision selection
    - Classify every eligible route and date into exactly one HOT/WARM/COLD tier, validate a total configurable route-tier/date-tier matrix, select one P1–P6 priority for each combination, and record non-blocking configured fallback-tier telemetry.
    - Create only `PeriodicRefresh` intents for selected scheduled work, bounded by configured batch/rate limits. The Scheduler must never publish, synthesize, or reroute inventory events.
    - _Requirements: 2.2, 4.1–4.6, 9.4, 13.2_

  - [ ]* 9.3 Write property test for inactive/ineligible candidate suppression
    - **Property 2: Inactive routes and ineligible dates create no scheduled candidate.** Its generated test covers both inactive routes and ineligible dates.
    - **Validates: Requirements 1.3**

  - [ ]* 9.4 Write property test for retained candidate coverage
    - **Property 3: Candidate coverage survives later inactivity** — every pair in an active eligible snapshot is evaluated once and remains recorded after subsequent route deactivation.
    - **Validates: Requirements 2.2**

  - [ ]* 9.5 Write property test for tier and priority mapping
    - **Property 7: Tier/matrix mapping is total and unique** — generated valid thresholds, complete matrices, routes, and dates receive exactly one route tier, date tier, and configured P1–P6 priority.
    - **Validates: Requirements 4.1–4.5**

  - [ ]* 9.6 Write Scheduler example and recovery tests
    - Cover default 10-minute configuration, empty successful/failed cycles, dependency/time-out cursor retention, lease loss, volume-data fallback observability failure, overlapping cycles, and inventory-event exclusion.
    - _Requirements: 2.1–2.5, 4.6, 10.5, 13.2_

- [ ] 10. Implement periodic P1–P6 publishing and weighted/fair consumption
  - [ ] 10.1 Implement periodic publication to the six matched queues
    - Consume Scheduler `PeriodicRefresh` intents only, revalidate their tier/matrix priority, and publish P1 through P6 exclusively to `periodic.p1` through `periodic.p6` through the immutable routing adapter.
    - Retain Dragonfly intent/reconciliation state and publisher-confirm behavior; do not publish periodic work to `inventory.refresh` and do not create inventory work from the Scheduler.
    - _Requirements: 2.2, 3.5, 4.1–4.5, 7.1, 7.7, 9.4, 9.5, 13.2_

  - [ ] 10.2 Implement weighted and age-aware periodic consumption
    - Configure P1 with the highest periodic selection weight and ensure P6 eventual service. Apply configurable age thresholds that boost lower-priority eligible messages under sustained higher-priority load while retaining the independent reserved inventory pool.
    - Keep weighted/fair selection in the Worker’s periodic local pool; it must not alter RabbitMQ message priority, the inventory stream, Dragonfly coalescing state, or Master global admission.
    - _Requirements: 3.5, 7.7, 9.3, 9.5, 10.1–10.3, 15.2, 15.6; implements selected decision 16.2_

  - [ ]* 10.3 Write property test for periodic tier-to-queue mapping
    - **Property 19: Valid periodic key maps to exactly one configured periodic queue** — every generated valid periodic Refresh_Key and route/date tier pair maps through the configured matrix to exactly one of `periodic.p1`–`periodic.p6`, and no other queue.
    - **Validates: Requirements 2.2, 4.1, 4.2, 4.3, 4.4, 4.5**

  - [ ]* 10.4 Write property test for trigger-type queue isolation
    - **Property 20: Trigger-type queue isolation** — generated inventory triggers route only to `inventory.refresh`; generated periodic triggers route only to their matching periodic queue; no generated request crosses the streams.
    - **Validates: Requirements 3.5, 7.7, 9.5**

  - [ ]* 10.5 Write property test for age-based periodic starvation prevention
    - **Property 21: Aged lower-priority work becomes eligible** — under generated sustained higher-priority periodic load, lower-priority work at or beyond its configured age threshold becomes eligible for periodic selection without violating inventory reservation.
    - **Validates: Requirements 3.5, 9.3, 9.5, 15.2, 15.6**

  - [ ]* 10.6 Write property test for bounded scheduled publication
    - **Property 13: Scheduled publication respects its bound** — generated candidate counts and configured batch/rate limits never yield more published periodic requests than the configured bound.
    - **Validates: Requirements 9.4**

  - [ ]* 10.7 Write periodic flow, fairness, and queue-isolation integration tests
    - Verify `inventory.refresh` remains consumable during a periodic backlog, P1 receives periodic precedence, P6 receives eventual service/no starvation, age boosting takes effect, reserved inventory capacity survives saturation, and periodic/inventory routing never crosses queues.
    - _Requirements: 3.5, 4.1–4.5, 7.1, 7.7, 9.3–9.5, 10.2, 15.2, 15.6_

- [ ] 11. Implement retry, contextual DLQ, failure reporting, and terminal acknowledgement
  - [ ] 11.1 Implement retry classification, failure reporting, contextual DLQ handoff, acknowledgement, and replay
    - Classify each failure exactly once; apply bounded exponential backoff with jitter to transient/rate-limit outcomes without raising local Worker concurrency; retain the immutable original envelope throughout retry.
    - Preserve original trigger type, original queue, periodic priority where applicable, Refresh_Key, request ID, correlation ID, attempt metadata, and failure context on every retry, DLQ, and replay envelope. Periodic retry retains its priority/context unless an explicit, separately designed reclassification flow is introduced.
    - Submit exactly one idempotent permanent-failure report before publisher-confirmed contextual DLQ handoff; retry unavailable reporting from Dragonfly state; acknowledge only after terminal Master success or confirmed DLQ handoff, retry live-channel successful acknowledgement indefinitely, and allow redelivery after channel/process loss.
    - _Requirements: 5.5, 5.6, 7.1–7.7, 9.3, 12.1–12.7, 13.1, 15.2, 15.3_

  - [ ]* 11.2 Write property test for bounded failure classification and delay
    - **Property 14: Failure classification and retry delay are bounded** — generated typed failures have exactly one class, jittered exponential delays remain within configured bounds, and no retry exceeds the attempt limit.
    - **Validates: Requirements 12.1, 12.2**

  - [ ]* 11.3 Write property test for failure-report-before-terminal handling
    - **Property 15: Permanent failure reporting precedes terminal handling** — generated permanent failures and reporting outages preserve and attempt the original-correlation report before DLQ terminal handling.
    - **Validates: Requirements 5.5, 12.4, 12.5**

  - [ ]* 11.4 Write property test for correlation-preserving replay
    - **Property 16: Replay preserves correlation identity** — generated DLQ records are replayable only when their original correlation ID remains unchanged.
    - **Validates: Requirements 12.6**

  - [ ]* 11.5 Write retry, DLQ, acknowledgement, and recovery integration tests
    - Use disposable quorum/retry/DLQ queues to verify contextual routing, original envelope preservation, periodic priority retention, report/DLQ publish failure recovery, manual acknowledgement/redelivery, channel-loss behavior, source/Master outage handling, and never acknowledging non-success work.
    - _Requirements: 5.5, 5.6, 7.1–7.7, 12.1–12.7, 13.1_

- [ ] 12. Wire production runtime, observability, security controls, and HA readiness
  - [ ] 12.1 Create the feature composition root and managed lifecycle in `V1/cmd/proactive-refresh/main.go`
    - Construct validated Dragonfly, RabbitMQ topology, source, secret-management, telemetry, trace, Scheduler, Event_Ingestor, publisher, Worker pools, and Master_Service adapters; register protected versioned APIs.
    - Start/stop scheduler, publisher, consumer pools, HTTP serving, connections, and metrics with contexts, deadlines, bounded goroutines, graceful draining, and readiness/health gates. Gate state-changing work on certified Dragonfly writable-primary/fencing/replica/lag/backup/no-eviction health and TLS/certificate/dependency health.
    - _Requirements: 1.4, 2.1, 5.1, 9.5, 10.1, 10.3, 10.5, 11.1–11.3, 14.1–14.4, 15.1, 15.2, 15.5–15.7_

  - [ ] 12.2 Implement audit, trace, metrics, security-event, and redaction adapters
    - Emit correlation-linked structured outcomes and distributed traces for accepted, coalesced, published, consumed, retried, submitted, persisted, stale, duplicate, rate-limited, failed, and dead-lettered work. Export stream-specific queue depth/age/in-flight metrics, local pool metrics, and global admission metrics.
    - Redact credentials, tokens, and configured sensitive fields before logs, traces, metrics, failure reports, and Dragonfly audit; record authn/authz/replay/credential-access failures; provide bounded correlation-path lookup or documented rejection.
    - _Requirements: 13.1–13.6, 14.6, 15.3_

  - [ ]* 12.3 Write property test for sensitive-data redaction
    - **Property 18: Observability redacts sensitive values** — generated credential, token, and configured-sensitive inputs never appear in emitted logs, traces, metrics, Dragonfly audit details, or failure reports.
    - **Validates: Requirements 13.6, 14.6**

  - [ ]* 12.4 Write runtime lifecycle, observability, HA-readiness, and security contract tests
    - Test configuration/dependency gates, primary role/fencing/lag/persistence/backup/memory failures, graceful shutdown/connection draining, TLS/service identity/ACL enforcement, stream-specific metrics, audit-query bound/rejection, and versioned API compatibility.
    - _Requirements: 1.4, 10.5, 11.1–11.6, 13.3–13.6, 14.1–14.6, 15.2, 15.5, 15.6_

- [ ] 13. Verify end-to-end delivery, load, race safety, and recovery
  - [ ]* 13.1 Write disposable end-to-end, failure-recovery, load, and race suites
    - Exercise Dragonfly primary/replica persistence, AOF/snapshot restore, backup integrity/RPO evidence, fenced failover, retained failed cycles, pending-intent reconciliation, RabbitMQ redelivery, Master atomic freshness ordering, and final accepted/idempotent/retry/DLQ states.
    - Use deterministic integration, contract, race, leak, and fault-injection suites with 1/2/5/10 Worker profiles. Verify capacity growth, global admission bounds, local concurrency caps, isolated inventory capacity, P1 precedence, P6 eventual service, queue age, no lost requests, audit lookup bound, and no secret leakage.
    - _Requirements: 2.4, 5.6, 7.1–7.7, 8.2–8.5, 9.1–9.6, 10.2–10.4, 12.3, 12.7, 13.3–13.6, 14.4–14.6, 15.4_

  - [ ]* 13.2 Write architecture-authority isolation verification
    - Add automated architecture/configuration assertions that TripDetails, claims, coalescing, triggers/intents, scheduler/candidate state, failure state, audit state, and admission state exist only in Dragonfly-backed namespaces.
    - Assert the feature has no SQL/relational database, Cassandra, Redis, or process-local correctness/recovery-state dependency; assert RabbitMQ is used only for delivery, retry, and DLQ handling.
    - _Requirements: 5.3, 6.1–6.6, 10.1–10.5, 13.1, 15.1–15.7_

- [ ] 14. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional automated-test tasks and may be skipped for an MVP; implementation tasks remain mandatory. The plan retains design Properties 1–18 and adds Properties 19–21 for the explicit stream-routing and starvation-prevention invariants requested for this task-list revision.
- The approved design uses Go, so no language-selection prompt is required. Property tests use injected clocks and dependencies, not wall-clock time or production services; RabbitMQ/Dragonfly adapter and integration tests use disposable isolated infrastructure.
- RabbitMQ is strictly durable delivery/retry/DLQ infrastructure. Dragonfly is the sole authoritative store for every TripDetails record and all proactive-refresh feature state: the only permitted authoritative categories are Dragonfly-backed TripDetails, claims, coalescing, triggers/intents, scheduler/candidate state, failure state, audit state, and admission state. No SQL/relational database, Cassandra, Redis, or process-local correctness/recovery state may be introduced. Master_Service is the only TripDetails writer and uses the atomic per-key freshness primitive.
- Requirement 16 is fulfilled by the approved Architecture Decision Register in `design.md`; its selected decisions are encoded in code tasks rather than by documentation-only tasks.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2"] },
    { "id": 2, "tasks": ["2.1"] },
    { "id": 3, "tasks": ["2.2", "2.3", "2.4"] },
    { "id": 4, "tasks": ["4.1"] },
    { "id": 5, "tasks": ["4.2"] },
    { "id": 6, "tasks": ["4.3", "4.4", "4.5", "4.6"] },
    { "id": 7, "tasks": ["5.1"] },
    { "id": 8, "tasks": ["5.2", "5.3", "5.4"] },
    { "id": 9, "tasks": ["6.1"] },
    { "id": 10, "tasks": ["6.2"] },
    { "id": 11, "tasks": ["7.1"] },
    { "id": 12, "tasks": ["7.2", "7.3", "7.4", "7.5"] },
    { "id": 13, "tasks": ["8.1"] },
    { "id": 14, "tasks": ["8.2"] },
    { "id": 15, "tasks": ["8.3", "8.4"] },
    { "id": 16, "tasks": ["9.1"] },
    { "id": 17, "tasks": ["9.2"] },
    { "id": 18, "tasks": ["9.3", "9.4", "9.5", "9.6"] },
    { "id": 19, "tasks": ["10.1"] },
    { "id": 20, "tasks": ["10.2"] },
    { "id": 21, "tasks": ["10.3", "10.4", "10.5", "10.6", "10.7"] },
    { "id": 22, "tasks": ["11.1"] },
    { "id": 23, "tasks": ["11.2", "11.3", "11.4", "11.5"] },
    { "id": 24, "tasks": ["12.1"] },
    { "id": 25, "tasks": ["12.2"] },
    { "id": 26, "tasks": ["12.3", "12.4"] },
    { "id": 27, "tasks": ["13.1"] },
    { "id": 28, "tasks": ["13.2"] }
  ]
}
```
