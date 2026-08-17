# orbitplus Requirements

## Introduction

**Purpose:** Define the Phase 1 functional requirements for orbitplus and document future responsibilities.

**Feature:** [spec.md](./spec.md)

orbitplus is the TripDetails microservice. Phase 1 covers ingestion: receive Worker submissions, validate JSON syntax, log the payload, and return an acknowledgement response. Future phases cover processing, caching, storage, and serving TripDetails data to the Orbit application.

## Glossary

- **orbitplus**: The TripDetails microservice specified by this document.
- **orbitplusworker**: The separate Worker service that fetches TripDetails from Bits and submits them to orbitplus.
- **Worker envelope**: The JSON request body sent by orbitplusworker, containing `actionType`, `operatorCode`, action-specific fields, and `orbitResponse`.
- **orbitResponse**: The field within the Worker envelope containing the raw Bits JSON response forwarded unchanged.
- **actionType**: Identifies the type of Bits fetch performed: `search`, `busmap`, or `searchbusmap` (lowercase).
- **Orbit application**: The downstream consumer that will read TripDetails data from orbitplus in future phases.

## Requirements

### Requirement 1: Receive Worker submissions

**User Story:** As orbitplusworker, I want to submit TripDetails data to orbitplus through HTTP, so that orbitplus can acknowledge receipt and the Worker can ACK the RabbitMQ delivery.

#### Acceptance Criteria

1. WHEN orbitplusworker sends an HTTP POST to `/api/tripdetails` with a JSON body, orbitplus SHALL read the complete request body before evaluating JSON validity.
2. IF reading the request body fails, orbitplus SHALL return HTTP 500 with `{"status":0,"message":"Internal server error"}`.
3. IF the request body is not syntactically valid JSON, orbitplus SHALL return HTTP 400 with `{"status":0,"message":"Invalid trip details JSON"}`.
4. IF the request body is syntactically valid JSON, orbitplus SHALL log the raw JSON payload to the terminal and return HTTP 200 with `{"status":1,"message":"Trip details received successfully"}`.
5. The Worker maps HTTP 2xx + `status:1` to ACCEPTED and ACKs the RabbitMQ delivery. This response contract is the Worker → orbitplus integration agreement.

### Requirement 2: Worker envelope structure

**User Story:** As orbitplusworker, I submit a structured envelope so that orbitplus can identify the action type and access the raw Bits response when processing begins.

#### Acceptance Criteria

1. The Worker envelope contains `actionType` (always present, lowercase: `search`, `busmap`, or `searchbusmap`) and `operatorCode` (always present).
2. For `search` and `searchbusmap`, the envelope additionally contains `fromCode`, `toCode`, and `tripDate`.
3. For `busmap`, the envelope additionally contains `tripCode`, `fromStationCode`, `toStationCode`, and `travelDate`.
4. The envelope contains `orbitResponse` carrying the raw Bits JSON response unchanged.
5. In Phase 1, orbitplus validates only that the complete body is syntactically valid JSON. It does not parse or validate individual envelope fields.

### Requirement 3: Health endpoint

**User Story:** As an operator, I want a health endpoint so that I can verify orbitplus is running.

#### Acceptance Criteria

1. `GET /health` SHALL return HTTP 200 with `{"status":"UP"}`.

### Requirement 4: Logging

**User Story:** As an operator, I want received payloads logged so that I can verify data is flowing through the system.

#### Acceptance Criteria

1. After successful JSON validation, orbitplus SHALL log the raw JSON payload to the terminal.
2. Error responses SHALL NOT include the request body content in log messages or response bodies beyond the standard error messages.

## Phase 1 boundaries

Phase 1 does not implement:
- Parsing or validation of individual Worker envelope fields
- Action-specific processing of `orbitResponse`
- Cache or storage
- Query/read APIs
- Duplicate or stale detection
- Authentication validation

These are future orbitplus responsibilities, not permanent exclusions.

## Future responsibilities

The following are expected future orbitplus capabilities (not current requirements):

- **TripDetails processing:** Parse envelope, route by `actionType`, process `orbitResponse` content.
- **Cache and storage:** Maintain TripDetails data for serving the Orbit application.
- **Duplicate/stale detection:** Return distinguishable responses (`DUPLICATE`, `STALE`) enabling the Worker to ACK without re-processing.
- **Orbit-facing read APIs:** Search, Busmap, TripDetails, and station-related endpoints for the Orbit application.
- **Authentication:** Dedicated Worker → orbitplus context token (details unresolved).
- **Inventory event handling:** High-priority refresh triggered by inventory events (external publisher scope).
- **Periodic refresh support:** Receiving data from the scheduled refresh pipeline.
