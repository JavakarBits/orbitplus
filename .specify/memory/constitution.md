<!-- Sync Impact Report
Version change: 1.0.0 → 2.0.0
Modified principles:
  - "Cache Reliability" → "Proactive TripDetails Freshness"
  - "API Stability" → "Worker-Based Processing"
  - "Resilient Communication" → "Reliable Message Delivery"
  - "Simplicity First" → "Minimal Dependencies"
  - "Observability" → "Observability & Traceability"
Added sections: Service Architecture (new)
Removed sections: Development Workflow (replaced by Service Architecture)
Renamed sections: "Technology Constraints" → "Technology Standards"
Follow-up TODOs: None
-->

# OrbitPlus Constitution

## Core Principles

### I. Proactive TripDetails Freshness

The system MUST proactively maintain TripDetails for all upcoming
trips within a 40-day window. TripDetails MUST be refreshed through
both inventory-driven triggers and periodic scheduling. Stale
TripDetails MUST NOT persist beyond their configured refresh
interval. The system MUST NOT rely on user requests to trigger
data updates — freshness is maintained ahead of demand.

**Rationale**: OrbitPlus exists to ensure TripDetails are always
current when needed; reactive fetching defeats the system's purpose.

### II. Worker-Based Processing

All TripDetails processing MUST be performed by dedicated worker
services consuming from RabbitMQ queues. Workers MUST be
horizontally scalable to handle high-volume processing. Each
worker MUST process messages idempotently — redelivered messages
MUST NOT produce duplicate or corrupt TripDetails. Workers MUST
acknowledge messages only after successful processing and
delivery to `orbitplusservice`.

**Rationale**: Decoupled worker architecture enables independent
scaling and fault isolation; message queues absorb load spikes
without cascading failures.

### III. Reliable Message Delivery

Messages between services MUST be delivered reliably via RabbitMQ.
No TripDetails update MUST be silently lost. Failed deliveries
MUST be retried or dead-lettered for inspection. The final
delivery of processed TripDetails JSON to `orbitplusservice`
MUST be confirmed before the originating message is acknowledged.

**Rationale**: Data loss in the pipeline means stale TripDetails
served to users; reliability is non-negotiable for a proactive
system.

### IV. Minimal Dependencies

New dependencies MUST only be introduced when they provide clear
architectural or operational value that cannot be achieved with
the existing stack. The core stack (Go, Gin, RabbitMQ, Dragonfly)
MUST remain stable. Utility libraries MUST be evaluated for
maintenance status, license compatibility, and binary size impact
before adoption.

**Rationale**: Each dependency is a maintenance and security
liability; a focused stack reduces operational complexity.

### V. Observability & Traceability

All worker processing MUST be logged with sufficient context to
trace a TripDetails update from queue message to database write.
Processing failures MUST be distinguishable from infrastructure
failures. Queue depth, processing latency, and refresh success
rates MUST be monitorable. Each TripDetails refresh cycle MUST
be traceable end-to-end.

**Rationale**: A proactive system that fails silently degrades
invisibly; operators MUST know when freshness guarantees are
not being met.

## Technology Standards

- **Language**: Go 1.25.0
- **HTTP Framework**: Gin 1.10.1
- **Message Broker**: RabbitMQ
- **Cache Store**: Dragonfly
- **Master Service**: `orbitplusservice`
- **Data Format**: TripDetails JSON

New technology additions MUST be justified by a concrete need that
cannot be met by the existing stack. Replacement of stack components
requires constitution amendment.

## Service Architecture

The system follows a pipeline architecture:

```text
Inventory System
       │
       ▼
orbitplusservice
       │
       ▼
RabbitMQ
       │
       ▼
Worker Service
       │
       ▼
Inventory / BusMap
       │
       ▼
TripDetails JSON
       │
       ▼
orbitplusservice
       │
       ▼
TripDetails DB
```

**Architectural constraints**:
- `orbitplusservice` is the single authority for publishing work
  to RabbitMQ and receiving completed TripDetails
- Worker services MUST NOT write directly to TripDetails DB
- Workers fetch source data (Inventory/BusMap), compose
  TripDetails JSON, and deliver to `orbitplusservice`
- The system MUST support inventory-driven refresh (triggered by
  inventory changes) and periodic refresh (scheduled maintenance
  of the 40-day window)

## Governance

This constitution supersedes informal practices and ad-hoc decisions.
All specification, planning, and implementation work MUST comply with
these principles.

**Amendment Procedure**:
1. Propose the change with rationale
2. Document impact on existing code and specifications
3. Update constitution version per semantic versioning rules
4. Record amendment date

**Versioning Policy**:
- MAJOR: Principle removal or backward-incompatible redefinition
- MINOR: New principle added or existing principle materially expanded
- PATCH: Clarifications, wording fixes, non-semantic refinements

**Compliance**: All specifications and plans produced by Spec Kit
commands MUST be validated against this constitution. Conflicts
are resolved in favor of constitution principles.

**Version**: 2.0.0 | **Ratified**: 2026-08-12 | **Last Amended**: 2026-08-12
