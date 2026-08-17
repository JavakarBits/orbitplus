# orbitplus Design and Delivery Plan

## Scope and approval gate

This plan covers only the behavior documented in [requirements.md](requirements.md) and [spec.md](spec.md): complete raw-body reading, syntactic one-value JSON validation, schema-agnostic preservation, valid-input terminal pretty-printing, safe failure handling, and action-dependent content preservation without a fixed response schema.

No delivery or implementation step below is actionable until all unresolved HTTP contract decisions have been explicitly approved and recorded: the endpoint path; success status and body; read-failure status and body; and invalid-JSON 4xx status and body. The approval gate does not permit inferring, substituting, or extending any HTTP decision.

## Delivery sequence after explicit HTTP approval

1. **Record the approved HTTP contract.** Update only the requirements and design documents with the approved path and the three approved response contracts. Do not add unapproved HTTP details.
2. **Define the request lifecycle.** Add the approved POST route and ensure complete body acquisition occurs before JSON evaluation. Route read failure, invalid JSON, and valid JSON to the specified terminal and approved HTTP outcomes.
3. **Implement schema-agnostic JSON handling.** Accept exactly one JSON value with optional trailing whitespace, retain all arbitrary, nested, and unknown values, and render valid values as indented terminal JSON. Do not require or semantically validate `actionType` or action-specific fields.
4. **Implement action-dependent preservation.** Recognize supplied `actionType` without imposing a shape. Preserve SEARCH Normal Trip Data and `stageFare[]` aggregate availability; retain supplied BUSMAP `bus.seatLayoutList[]` item-for-item; retain actual SEARCHBUSMAP structure; and leave any absent layout absent without fabrication.
5. **Implement safe failure behavior.** For body-read failure, emit only a non-sensitive read-failure message and stop. For invalid JSON, emit only a non-sensitive validation message, omit pretty JSON, stop processing, and apply the approved 4xx contract. Exclude body content from diagnostics.
6. **Verify the approved behavior.** Add example tests for the approved response contracts, deterministic read failure, design-only category review, and scope review. Add the property tests listed in `spec.md`, each with at least 100 iterations and its feature/property comment.

## Future logical model documentation

The following model is conceptual future documentation only; it does not describe current processing or storage:

- **Metadata:** Available trip-identifying values only—such as `operatorCode`, `fromStationCode`, `toStationCode`, `scheduleCode`, `tripCode`, `tripDate`, and `updatedAt`. A missing identifier remains absent.
- **Common Trip Details:** Supplied common trip-level information, without a fixed field set.
- **Fare / Seat Availability:** Supplied `stageFare[]`, including aggregate `availableSeatCount`.
- **Seat Layout:** Only supplied `bus.seatLayoutList[]` and its individual layout items.

## Verification gates

| Gate | Evidence |
| --- | --- |
| Approval gate | Explicit approval records the endpoint path and all three response contracts. Until then, delivery steps are blocked. |
| Preservation gate | Tests demonstrate structural retention of arbitrary values, action recognition without fixed schema, SEARCH aggregate availability, supplied BUSMAP layout retention, actual SEARCHBUSMAP retention, and no fabricated layout. |
| Failure-safety gate | Tests demonstrate complete-read ordering, termination after read failure, safe malformed-JSON rejection, no pretty JSON for failures, and no body content in terminal errors. |
| Conceptual-model gate | Documentation review confirms only the four future categories, available identifiers only, and absence retained as absent. |
| Scope gate | Review confirms the delivered behavior and tests match the approved requirements and no unapproved contract detail was added. |
