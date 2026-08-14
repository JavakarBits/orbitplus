# Requirements Document

## Introduction

**Document**: Proactive TripDetails Refresh Worker checklist

**Purpose**: Validate the Worker-only specification and plan.

**Feature**: [spec.md](../spec.md)

## Glossary

- **Worker**: The standalone `orbitplusworker` process described by the linked specification.

## Requirements

### Scope and Flow

- [x] The scope is limited to `RabbitMQ → orbitplusworker → Bits Service → external OrbitPlus destination → RabbitMQ ACK`.
- [x] Scheduler, publishers, Master implementation/storage/query APIs, Dragonfly, credential service, and V1 are explicitly out of scope.
- [x] The documents do not define Worker retries, retry queues, dead-letter configuration, distributed coordination, or external destination internals.

## Message and Route Contract

- [x] `operatorCode` and `actionType` are required for every message.
- [x] `search` and `searchbusmap` require `fromCode`, `toCode`, and `tripDate`.
- [x] `busmap` requires `tripCode`, `fromStationCode`, `toStationCode`, and `travelDate`.
- [x] All three documented Bits GET routes use the required parameters.
- [x] Dynamic Bits path segments are required to be safely escaped.
- [x] Credentials are documented only as temporary approved development values built in Worker code from `operatorCode`; no credential-service contract is specified.

## Delivery, Logging, and Configuration

- [x] Raw Bits JSON is logged only after a successful fetch and is sent unchanged in the existing `orbitResponse` field.
- [x] The external destination POST is `{ORBITPLUS_URL}/v1/trip-details/refreshes`.
- [x] Only `ACCEPTED`, `DUPLICATE`, and `STALE` permit ACK.
- [x] Invalid messages, Bits errors, OrbitPlus errors/retryable responses, and ACK errors remain unacknowledged for existing RabbitMQ redelivery/DLQ behavior.
- [x] Logs prohibit credentials, credential objects, credential-bearing request URLs, passwords, headers, and secrets.
- [x] `WORKER_CONCURRENCY` is distinguished from `RABBITMQ_PREFETCH`.
- [x] Configuration retains only `APP_ENV`, RabbitMQ settings, `BITS_BASE_URL`, `ORBITPLUS_URL`, `WORKER_CONCURRENCY`, optional `WORKER_HTTP_TIMEOUT`, and optional Health API settings.
- [x] Legacy `ORBIT_USERNAME`, `ORBIT_API_TOKEN`, `ORBIT_ZONE_URL`, and `WORKER_OPERATION_TIMEOUT` are absent.

## Quality

- [x] Requirements are precise, internally consistent, and limited to documented Worker behavior.
- [x] No Master-internal persistence, Dragonfly, query, freshness, deduplication, rate-limit, or security requirement remains.
