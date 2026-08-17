# Specification Validation Checklist

## Introduction

**Document**: Proactive TripDetails Refresh Worker validation checklist

**Purpose**: Validate the Worker-only specification and plan for internal consistency, completeness, and implementation readiness. This document is a validation checklist, not an independent functional requirements specification.

**Feature**: [spec.md](../spec.md)

## Glossary

- **Worker**: The standalone `orbitplusworker` process described by the linked specification.

## Checklist

### Scope and Flow

- [x] The scope is limited to `RabbitMQ → orbitplusworker → Bits Service → external OrbitPlus destination → RabbitMQ ACK`.
- [x] Scheduler, publishers, Master implementation/storage/query APIs, Dragonfly, credential service, V1, duplicate detection, freshness tracking, and version comparison are explicitly out of scope.
- [x] The documents do not define Worker retries, retry queues, dead-letter configuration, distributed coordination, or external destination internals.

### Message and Route Contract

- [x] `operatorCode` and `actionType` are required for every message.
- [x] `search` and `searchbusmap` require `fromCode`, `toCode`, and `tripDate`.
- [x] `busmap` requires `tripCode`, `fromStationCode`, `toStationCode`, and `travelDate`.
- [x] All three documented Bits GET routes use the required parameters.
- [x] Dynamic Bits path segments are required to be safely escaped.
- [x] Credentials are documented as temporary hardcoded development constants for the Bits username and API token; `operatorCode` is used as the operator path segment but does not derive the username or API token. No credential-service contract is specified.

### Delivery, Logging, and Configuration

- [x] Raw Bits JSON is logged only after a successful fetch and is sent unchanged in the existing `orbitResponse` field.
- [x] The external destination POST is `{ORBITPLUS_URL}/api/tripdetails`.
- [x] Only `ACCEPTED` permits ACK in the current phase. `ACCEPTED` is the only outcome the existing OrbitPlus destination currently produces for a successful submission.
- [x] `DUPLICATE` and `STALE` are documented as future ACK-eligible outcomes only; they are not currently producible and are outside the current phase.
- [x] Invalid messages, Bits errors, OrbitPlus errors/retryable responses, and ACK errors remain unacknowledged for existing RabbitMQ redelivery/DLQ behavior.
- [x] Logs prohibit credentials, credential objects, credential-bearing request URLs, passwords, headers, and secrets.
- [x] `WORKER_CONCURRENCY` is distinguished from `RABBITMQ_PREFETCH`.
- [x] Configuration retains only `APP_ENV`, RabbitMQ settings, `BITS_BASE_URL`, `ORBITPLUS_URL`, `WORKER_CONCURRENCY`, optional `WORKER_HTTP_TIMEOUT`, and optional Health API settings.
- [x] Legacy `ORBIT_USERNAME`, `ORBIT_API_TOKEN`, `ORBIT_ZONE_URL`, and `WORKER_OPERATION_TIMEOUT` are absent.

### Authentication

- [x] Worker → OrbitPlus authentication is acknowledged as existing in implementation (bearer-token mechanism) but is documented as a temporary detail, not the final contract.
- [x] The intended architectural direction is a dedicated context token specifically for Worker → OrbitPlus communication; token name, HTTP header, format, validation, storage, and configuration variable names are unresolved.
- [x] No authentication implementation details are specified or finalized in the current phase.

### Quality

- [x] Requirements are precise, internally consistent, and limited to documented Worker behavior.
- [x] No Master-internal persistence, Dragonfly, query, freshness, deduplication, rate-limit, or security requirement remains.
- [x] The documents do not assert unresolved HTTP contract decisions that belong to the Master project.
