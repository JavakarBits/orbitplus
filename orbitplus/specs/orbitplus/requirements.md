# Requirements Document

## Introduction

Orbitplus accepts one complete raw Bits TripDetails JSON response submitted by the Worker through an HTTP POST endpoint. For a syntactically valid JSON request, Orbitplus reads the complete request body, preserves the submitted JSON structure and all fields without schema-based transformation, writes a pretty-printed representation to the Terminal, and returns a successful HTTP response. The Master recognizes action-dependent Bits response content without assuming one fixed response shape. This specification intentionally covers no persistence, downstream processing, querying, or other future architecture.

## Glossary

- **Orbitplus**: The HTTP service specified by this document.
- **Master**: Orbitplus when handling a Bits TripDetails JSON response.
- **Worker**: The external client that submits a raw Bits TripDetails JSON response to Orbitplus.
- **Bits TripDetails JSON response**: A JSON value supplied by the Worker that represents a response from Bits; Orbitplus treats the value as schema-agnostic input.
- **Request body**: The complete sequence of bytes carried in an incoming HTTP POST request.
- **Syntactically valid JSON**: A Request body that is accepted as a single JSON value by a conforming JSON parser, with no non-whitespace bytes after that value.
- **Pretty-printed JSON**: A textual JSON representation that preserves the parsed JSON value and uses indentation and line breaks.
- **Terminal**: The standard output or configured process log output visible to the Orbitplus operator.
- **Client error response**: An HTTP response with a 4xx status code.
- **Non-sensitive error**: An error message that identifies a Request body read or JSON-validation failure without including Request body content.
- **actionType**: A Bits-supplied value that identifies the response action, including `SEARCH`, `BUSMAP`, and `SEARCHBUSMAP` when supplied.
- **Normal Trip Data**: Trip information supplied by Bits in a `SEARCH` response other than `stageFare[]`; the current phase defines no fixed field set for Normal Trip Data.
- **stageFare[]**: Bits-supplied aggregate fare and seat-availability data. Each supplied availability entry represents `fare`, `seatType`, `seatName`, and `availableSeatCount` as aggregate availability information.
- **Aggregate Availability**: The availability represented by `stageFare[].availableSeatCount`, which is distinct from individual Seat Layout Data.
- **Seat Layout Data**: Individual seat-layout information supplied only when `bus.seatLayoutList[]` is present.
- **Conceptual Future Logical Separation**: A documentation-only classification for a future phase that authorizes no logical splitting, persistence, Dragonfly use, storage, or extra processing in the current phase.

## Requirements

### Requirement 1: Accept a raw Bits TripDetails JSON response

**User Story:** As a Worker, I want to submit a raw Bits TripDetails JSON response through HTTP, so that Orbitplus can validate and display the received response.

#### Acceptance Criteria

1. WHEN the Worker sends an HTTP POST request to the TripDetails ingestion endpoint, THE Orbitplus SHALL read the complete Request body before evaluating JSON validity.
2. IF reading the Request body fails, THEN THE Orbitplus SHALL write a Non-sensitive error identifying the Request body read failure to the Terminal.
3. IF reading the Request body fails, THEN THE Orbitplus SHALL return an error HTTP response after logging the read failure, without initiating recovery or invoking JSON validation, parsing, preservation, or Pretty-printed JSON output after the read failure is detected.
4. WHEN the complete Request body has been read without failure and contains Syntactically valid JSON, THE Orbitplus SHALL preserve the complete raw Bits TripDetails JSON response as the parsed JSON value, including all supplied fields, arbitrary nested values, and fields not otherwise named in this specification.
5. WHEN the complete Request body has been read without failure and contains Syntactically valid JSON, THE Orbitplus SHALL accept the Request body without applying semantic validation to the JSON value.
6. WHEN the complete Request body has been read without failure and contains Syntactically valid JSON, THE Orbitplus SHALL write Pretty-printed JSON representing the parsed JSON value to the Terminal.
7. WHEN the complete Request body has been read without failure and contains Syntactically valid JSON, THE Orbitplus SHALL return a successful HTTP response.

### Requirement 2: Reject invalid JSON input

**User Story:** As an Orbitplus operator, I want invalid submitted JSON to be rejected and diagnosed safely, so that invalid input is not displayed as a valid response.

#### Acceptance Criteria

1. WHEN JSON parsing of the complete Request body fails for any cause, THE Orbitplus SHALL return a Client error response without attempting recovery or fallback processing.
2. WHEN JSON parsing of the complete Request body fails for any cause, THE Orbitplus SHALL write a Non-sensitive error identifying the JSON-validation failure to the Terminal.
3. WHEN JSON parsing of the complete Request body fails for any cause, THE Orbitplus SHALL omit Pretty-printed JSON from the Terminal.
4. IF the Non-sensitive error includes diagnostic details, THEN THE Orbitplus SHALL exclude Request body content from the Non-sensitive error.

### Requirement 3: Preserve action-dependent Bits TripDetails content

**User Story:** As a Worker, I want the Master to retain the actual Bits response for each action, so that action-dependent trip, availability, and seat-layout information remains available without fabrication or schema assumptions.

#### Acceptance Criteria

1. WHEN a Syntactically valid JSON Bits TripDetails JSON response supplies an actionType, THE Master SHALL recognize the supplied actionType without assuming a single fixed response shape.
2. WHEN the supplied actionType is `SEARCH`, THE Master SHALL preserve the supplied Normal Trip Data and `stageFare[]` as Aggregate Availability data.
3. WHEN the supplied actionType is `SEARCH`, THE Master SHALL treat an absent `bus.seatLayoutList[]` as not applicable and SHALL create no Seat Layout Data.
4. WHEN the supplied actionType is `BUSMAP` and `bus.seatLayoutList[]` is supplied, THE Master SHALL preserve `bus.seatLayoutList[]` as Seat Layout Data.
5. WHEN `bus.seatLayoutList[]` is supplied, THE Master SHALL retain each supplied seat-layout item and any supplied optional examples of `code`, `busSeatType`, `seatGendar`, `seatStatus`, `rowPos`, `colPos`, `seatPos`, `layer`, `seatName`, `seatFare`, `serviceTax`, `discountFare`, `orientation`, and `emergencyExitDoor` as individual Seat Layout Data.
6. WHEN the supplied actionType is `SEARCHBUSMAP`, THE Master SHALL preserve the actual received Bits structure without assigning an invented fixed schema.
7. WHEN `stageFare[].availableSeatCount` is supplied, THE Master SHALL preserve the value as Aggregate Availability distinct from individual Seat Layout Data.
8. WHEN `bus.seatLayoutList[]` is absent for any supplied actionType, THE Master SHALL retain the absence as not applicable and SHALL create no fake or default Seat Layout Data.
9. WHEN a Syntactically valid JSON Bits TripDetails JSON response omits a field that is absent or not guaranteed for the supplied actionType, THE Master SHALL accept and preserve the response without treating the omission as a semantic validation failure.

### Requirement 4: Limit future logical separation to documentation

**User Story:** As a future Orbitplus designer, I want a documented logical classification of supplied Bits data, so that future design can distinguish data categories without expanding the current processing scope.

#### Acceptance Criteria

1. WHEN a future logical representation is documented for a supplied Bits TripDetails JSON response, THE Master SHALL limit the classification to Metadata, Common Trip Details, Fare / Seat Availability, and Seat Layout.
2. WHEN Metadata is available in a future logical representation, THE Master SHALL classify supplied trip-identifying information such as `operatorCode`, `fromStationCode`, `toStationCode`, `scheduleCode`, `tripCode`, `tripDate`, and `updatedAt` as Metadata.
3. WHEN Common Trip Details are available in a future logical representation, THE Master SHALL classify the supplied common trip-level information as Common Trip Details without defining a fixed field set in the current phase.
4. WHEN `stageFare[]` is available in a future logical representation, THE Master SHALL classify `stageFare[]` as Fare / Seat Availability.
5. WHEN `bus.seatLayoutList[]` is supplied in a future logical representation, THE Master SHALL classify `bus.seatLayoutList[]` as Seat Layout.
6. WHEN a value is missing or not applicable in a future logical representation, THE Master SHALL retain the value as absent or not applicable without synthesizing a value.
7. WHILE the current phase is active, THE Master SHALL use the Conceptual Future Logical Separation only as a documentation-only classification.

## Clarification

The existing schema-agnostic requirements already allow action-dependent Bits response structures and optional seat-layout data. The prior requirements did not explicitly describe those semantics; this update clarifies them without identifying a conflict or selecting any unresolved HTTP contract decision.

## Unresolved Decisions

- The HTTP path of the TripDetails ingestion endpoint is not specified.
- The successful HTTP status code and response body shape are not specified.
- The error HTTP status code and response body shape for a Request body read failure are not specified.
- The Client error status code and response body shape for invalid JSON are not specified.
