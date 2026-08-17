# orbitplus Master Design Specification

## Purpose and current-phase boundary

Orbitplus is the Master service that receives one raw Bits TripDetails JSON response from a Worker through an HTTP POST ingestion endpoint. This phase is limited to reading the complete request body, syntactically validating one JSON value, recognizing a supplied `actionType`, preserving the received value, writing valid JSON to the Terminal in pretty-printed form, and returning the applicable HTTP outcome.

The Master treats the received value as schema-agnostic. It preserves every supplied field, arbitrary nested value, and unknown field without transformation, removal, synthesis, or semantic validation. A supplied `actionType` is recognized descriptively; it is optional and never requires action-specific fields or a fixed response shape.

## Ingestion lifecycle

1. The Worker submits the complete raw Bits TripDetails JSON response through the unresolved HTTP POST endpoint.
2. Orbitplus reads the complete Request body before evaluating JSON validity.
3. A read failure writes a Non-sensitive error to the Terminal, returns the unresolved read-failure HTTP outcome, and terminates processing. It must not initiate validation, parsing, preservation, or Pretty-printed JSON output.
4. A successfully read body must contain exactly one Syntactically valid JSON value. Trailing whitespace is allowed; a second value or other trailing non-whitespace data is invalid.
5. Invalid JSON writes a Non-sensitive validation error without Request body content, is not Pretty-printed, has no recovery or fallback processing, and returns an unresolved Client error response.
6. Valid JSON is retained structurally as the parsed JSON value, Pretty-printed to the Terminal, and returns the unresolved successful HTTP outcome.

## Action-dependent Bits content

`actionType` recognition does not alter the preserved JSON value or add semantic validation.

- **`SEARCH`:** Preserve all supplied Normal Trip Data and `stageFare[]`. Each supplied `stageFare[]` entry, including `fare`, `seatType`, `seatName`, and `availableSeatCount`, remains Aggregate Availability. `bus.seatLayoutList[]` is not expected or required; when absent, it remains absent or not applicable and no Seat Layout Data is created.
- **`BUSMAP`:** Preserve the supplied TripDetails structure. If `bus.seatLayoutList[]` is supplied, preserve every item as Seat Layout Data, including all supplied optional, unknown, and nested fields. Fields such as `code`, `busSeatType`, `seatGendar`, `seatStatus`, `rowPos`, `colPos`, `seatPos`, `layer`, `seatName`, `seatFare`, `serviceTax`, `discountFare`, `orientation`, and `emergencyExitDoor` are optional examples, not a required schema.
- **`SEARCHBUSMAP`:** Preserve the actual Bits response structure without inventing or imposing a fixed schema.
- **Any action:** Missing optional or action-dependent fields remain absent or not applicable. `stageFare[].availableSeatCount` is Aggregate Availability and is distinct from individual `bus.seatLayoutList[]` items. No fake, default, or empty seat-layout data is created.

## Conceptual future data classification

This classification is documentation only for a future phase. It does not authorize current storage, persistence, Dragonfly use, splitting, querying, or additional processing.

| Category | Conceptual content | Absence rule |
| --- | --- | --- |
| Metadata | Supplied `operatorCode`, `fromStationCode`, `toStationCode`, `scheduleCode`, `tripCode`, `tripDate`, and `updatedAt` | Missing values remain absent. |
| Common Trip Details | Supplied common trip-level information | No fixed field list is defined. |
| Fare / Seat Availability | Supplied `stageFare[]`, including aggregate `availableSeatCount` | Missing values remain absent. |
| Seat Layout | Supplied `bus.seatLayoutList[]` items only | Absent or inapplicable data remains absent. |

## Unresolved HTTP contract approval items

Do not infer or implement any of the following without explicit approval: the POST endpoint path; the successful HTTP status code and response body; the request-body-read-failure status code and response body; and the invalid-JSON Client error status code and response body.

## Explicitly out of scope

Dragonfly; database persistence; TripDetails storage or splitting; query APIs or GET TripDetails; freshness tracking, deduplication, version comparison, TTL/expiration, or scheduling; RabbitMQ publishing; Worker implementation; credential service; seat-layout or seat-availability calculation; data enrichment or transformation; and V1.

## Requirements alignment

This specification implements the terminology and intent of `requirements.md`: complete-body reading, schema-agnostic structural preservation, safe JSON rejection, action-dependent preservation without fabrication, and documentation-only future classification. No unresolved HTTP decision is selected here.
