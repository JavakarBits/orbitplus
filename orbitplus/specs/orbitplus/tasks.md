# Implementation Plan: orbitplus Master

## Overview

Deliver only the first Master capability: Worker → approved HTTP POST → Orbitplus → receive raw Bits TripDetails JSON → validate it → Pretty-print valid JSON to the Terminal → return the approved HTTP response. The received JSON remains schema-agnostic throughout.

## Prerequisite — HTTP contract approval gate

**No implementation or test task below may begin until explicit approval records all of these decisions in `requirements.md` and `spec.md`:**

- POST ingestion endpoint path.
- Successful HTTP status code and response body.
- Request-body-read-failure HTTP status code and response body.
- Invalid-JSON HTTP status code and response body.

Do not infer, select, or substitute any of these values. This gate does not authorize changes outside the approved contract.

## Tasks

- [ ] 1. Create the basic Orbitplus Master service structure needed for the approved ingestion capability only.
- [ ] 2. After HTTP contract approval, expose exactly the one approved POST ingestion endpoint; add no additional APIs, routes, or authentication behavior.
- [ ] 3. Read the complete Request body before JSON validation. On a read failure, write only a Non-sensitive read-failure error, return only the approved outcome, and stop without validation, preservation, or Pretty-printed output.
- [ ] 4. Validate the successfully read body as exactly one Syntactically valid JSON value. Permit trailing whitespace only; reject an empty or whitespace-only body, malformed JSON, a second JSON value, and other trailing non-whitespace data.
- [ ] 5. Preserve each valid parsed JSON value structurally, including arbitrary values, unknown fields, and nested content. Apply no semantic schema validation, transformation, removal, or synthesis. Recognize `actionType` only when supplied; do not require it or action-specific fields.
- [ ] 6. Pretty-print valid JSON to the Terminal and return only the approved success outcome. For invalid JSON, write only a Non-sensitive validation error, do not print the submitted JSON, do not recover or fall back, and return only the approved Client error outcome. Error diagnostics must never contain Request body content.
- [ ] 7. Verify non-transforming action-dependent preservation: retain supplied `SEARCH` Normal Trip Data and `stageFare[]`, keeping `stageFare[].availableSeatCount` as Aggregate Availability; leave absent `bus.seatLayoutList[]` absent or not applicable; retain every supplied `BUSMAP` layout item unchanged; and retain the actual `SEARCHBUSMAP` structure without inventing a schema.
- [ ] 8. Add focused tests—do not add a property-based testing framework—for: valid `SEARCH`; valid `BUSMAP` with `bus.seatLayoutList[]`; valid `SEARCH` without a layout; valid `SEARCHBUSMAP`; arbitrary/unknown fields; missing or unknown `actionType`; missing action-specific fields; empty and whitespace-only bodies; malformed JSON; valid JSON followed by a second value or non-whitespace data; and request-body read failure.
- [ ] 9. In the focused tests, verify valid JSON is Pretty-printed; invalid JSON is not Pretty-printed; errors never include Request body content; `stageFare[].availableSeatCount` is never converted to layout data; and supplied `bus.seatLayoutList[]` is retained without modification.
- [ ] 10. Run all focused tests and manually verify the approved endpoint using supplied `SEARCH` JSON. Confirm the approved response, Terminal output, and failure behavior.
- [ ] 11. Final verification: confirm the implementation still contains only the approved ingestion capability and rerun all tests.

## Scope guardrails

Do not implement Dragonfly, Redis, database persistence, TripDetails storage or splitting, metadata persistence, query APIs or GET TripDetails, freshness tracking, deduplication, version comparison, TTL/expiration, scheduling, RabbitMQ publishing, Worker changes, credential service, seat-layout or seat-availability calculation, data enrichment or transformation, unapproved authentication, or additional APIs.

Metadata, Common Trip Details, Fare / Seat Availability, and Seat Layout are documentation-only conceptual categories. Do not create them as separate models, storage structures, persistence objects, or processing pipelines in this phase.

## Dependency order

HTTP contract approval → project/service setup → approved POST endpoint → complete body read → JSON validation → valid JSON Terminal output → failure handling → action-dependent preservation verification → focused tests → manual endpoint verification → final test pass.

## Notes

The HTTP approval gate is mandatory and does not permit selecting unresolved contract values. Conceptual future data categories remain documentation only.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1"] },
    { "id": 1, "tasks": ["2", "3", "4", "5", "6", "7"] },
    { "id": 2, "tasks": ["8", "9"] },
    { "id": 3, "tasks": ["10"] },
    { "id": 4, "tasks": ["11"] }
  ]
}
```