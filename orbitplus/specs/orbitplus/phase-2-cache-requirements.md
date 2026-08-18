# Requirements Document

## Introduction

This document is a separate Phase 2 addendum to the approved Phase 1 ingestion specification. It defines requirements for retaining accepted Bits TripDetails content in Dragonfly and indexing metadata in Cassandra. It does not modify Phase 1 behavior, the Worker payload, `POST /api/tripdetails`, or any existing HTTP response contract.

Phase 1 continues to read the complete Request body, validate exactly one JSON value, preserve valid schema-agnostic input, log it, and return its established outcome. Phase 2 begins only after that validation succeeds.

## Glossary

- **Trip**: Content logically common to one supplied `operatorCode` and `tripCode`.
- **Stage**: Content identified by one supplied `tripStageCode` within a Trip.
- **BUSMAP**: Supplied seat-layout content associated with one Stage.
- **Dragonfly**: The primary store for TripDetails content required for future response reconstruction.
- **Cassandra metadata**: Lookup and existence information only; never complete TripDetails content.

## Requirements

### Phase 2 purpose and baseline

Phase 2 adds cache-backed TripDetails storage after successful Phase 1 validation only.

## Scope

Phase 2 stores TripDetails content in Dragonfly and only lookup metadata in Cassandra. It supports future reconstruction of SEARCH and BUSMAP responses from Dragonfly after Cassandra metadata identifies the relevant Trip and stage. The Master must preserve supplied fields, nesting, and optional data without fixed TripDetails or seat schemas.

## Non-goals

This phase does not add or change public HTTP endpoints, Worker behavior, authentication, query response contracts, RabbitMQ, scheduling, freshness calculation, deduplication policy, version comparison policy, TTL values, data enrichment, seat calculations, or Cassandra storage of complete TripDetails JSON. SEARCHBUSMAP storage is not implemented by this requirements set.

## Storage responsibilities

- **Dragonfly** is the primary TripDetails content store. It stores JSON content required to reconstruct approved future SEARCH and BUSMAP responses.
- **Cassandra** stores metadata and lookup/index fields only. It must never store complete TripDetails JSON, `stageFare[]`, `seatLayoutList[]`, or arbitrary nested Bits content.
- **Trip** means information common to a `tripCode`; **Stage** means information identified by a supplied `tripStageCode`; **BUSMAP** means supplied seat-layout content for a Stage.

## Dragonfly logical keys

The following are independent JSON values, not an authorization to use one hash or one flat entry:

- `trip:{operatorCode}:{tripCode}`: canonical SEARCH Trip content, owned and written only by SEARCH ingestion. It is the only SEARCH cache document that contains `bus`.
- `stage:{operatorCode}:{tripCode}:{tripStageCode}`: canonical Stage content, owned and written only by SEARCH ingestion. It contains only `tripStageCode`, `fromStation`, `toStation`, `stationPoint`, and `stageFare[]`; it never contains `bus`.
- `busmap:{operatorCode}:{tripCode}:{tripStageCode}`: complete supplied BUSMAP response entry, owned and written only by BUSMAP ingestion.

A Trip may have multiple Stages. Writes for one `tripStageCode` must not overwrite the Stage or BUSMAP key of any other `tripStageCode`. BUSMAP ingestion must never replace a canonical Trip or Stage document, or add or update Cassandra metadata; its only persistence write is its own Stage-specific BUSMAP key.

## Common, Stage, and BUSMAP content

The Trip value contains every supplied SEARCH field except the fixed Stage fields `tripStageCode`, `fromStation`, `toStation`, `stationPoint`, and `stageFare[]`. It is the only SEARCH document that contains `bus`, and its `bus` object excludes `seatLayoutList[]`. The Stage value contains only those five fixed Stage fields. The BUSMAP value retains the complete supplied BUSMAP entry so its API-specific values, formatting, array order, optional fields, and unknown fields can be reproduced independently of SEARCH.

This is the approved ownership rule for SEARCH cache storage. Unknown SEARCH fields belong to the Trip value; no field is silently discarded. Stage-specific fields remain isolated by `tripStageCode`, and a Stage document never duplicates the Trip-owned `bus` object.

`stageFare[].availableSeatCount` remains aggregate Fare / Seat Availability data. It must not be converted to individual seat-layout records. `seatLayoutList[]` is excluded from Trip and Stage cache documents and is stored only in the matching BUSMAP cache key when supplied.

## Ingestion and storage lifecycle

For a valid Phase 1 request, the Master SHALL:

1. Retain the existing complete-body read and exactly-one-value validation behavior.
2. Identify a supplied `actionType` descriptively, without rejecting absent, unknown, or incomplete action-specific content.
3. Locate the actual Bits response using the approved Worker payload structure. When the Worker supplies `orbitResponse`, TripDetails entries are obtained from its supplied `data[]`; every supplied entry is processed independently.
4. For SEARCH entries, obtain the identifiers needed for canonical storage and lookup: `operatorCode`, `tripCode`, `tripStageCode`, `fromStation.code`, `toStation.code`, and `travelDate`. For BUSMAP entries, obtain only `operatorCode`, `tripCode`, and `tripStageCode` needed for the dedicated response key.
5. SEARCH alone writes every non-Stage field, including `bus` without `seatLayoutList[]`, to the Trip key. It writes only `tripStageCode`, `fromStation`, `toStation`, `stationPoint`, and `stageFare[]` to the Stage key.
6. BUSMAP alone writes the complete supplied BUSMAP entry to the matching BUSMAP key. It must not write or update the Trip key, Stage key, or Cassandra metadata. SEARCH must not create a BUSMAP key.
7. SEARCH writes corresponding Cassandra metadata only after its required Dragonfly content writes satisfy the approved partial-write policy.
8. Return the existing successful ingestion response only when the approved Phase 2 write-consistency policy permits acknowledgement. Cache or metadata failures must use the existing server-side error handling contract and must not expose internal details or Request body content.

No cache processing may replace Phase 1 logging or mutate the original valid parsed JSON. If required identifiers are absent, the valid JSON must remain accepted by Phase 1; Phase 2 cache behavior for that entry is an unresolved decision.

## SEARCH requirements

A SEARCH response may contain many `data[]` entries for the same `tripCode` and different `tripStageCode` values. The Master SHALL retain all non-Stage content, including the SEARCH `bus` object without `seatLayoutList[]`, once in `trip:{operatorCode}:{tripCode}`. It SHALL retain only `tripStageCode`, `fromStation`, `toStation`, `stationPoint`, and `stageFare[]` for each supplied Stage in `stage:{operatorCode}:{tripCode}:{tripStageCode}`. A Stage write must never overwrite another Stage. SEARCH must preserve all supplied `stageFare[]` values and must not fabricate `seatLayoutList[]`.

## BUSMAP requirements

A BUSMAP response SHALL retain the complete supplied response entry at `busmap:{operatorCode}:{tripCode}:{tripStageCode}`. This includes every top-level field, nested object, optional field, unknown field, `seatLayoutList[]` item, formatting-sensitive string, and contract-significant array order. BUSMAP content must remain associated with the same Stage key and must not mix data across stages. Missing optional fields remain missing and null values remain null; no field, empty list, or seat record may be fabricated. The BUSMAP read API returns this dedicated entry semantically unchanged, while SEARCH continues to reconstruct only from SEARCH-owned Trip and Stage documents.

## SEARCHBUSMAP

The approved Phase 1 baseline requires structural preservation but does not define a safe Phase 2 split for SEARCHBUSMAP. The Master SHALL preserve the received JSON through Phase 1 handling. It SHALL not write a new cache layout for SEARCHBUSMAP until a separate approved ownership and reconstruction rule exists.

## Cassandra metadata and stage lookup

Cassandra metadata must support existence checks and mapping a future request to the correct Dragonfly keys. Each metadata record conceptually includes `operator_code`, `trip_code`, `trip_stage_code`, `from_station_code`, `to_station_code`, `travel_date`, and `updated_at`. `trip_stage_code` is mandatory metadata because it identifies the Stage cache key.

The metadata access pattern must support the lookup `operatorCode + tripCode + fromStationCode + toStationCode + travelDate → one or more tripStageCode values`. It must preserve multiple legitimate matches rather than silently overwriting a Stage. The precise Cassandra table primary key, partitioning, clustering, and query strategy are design decisions; complete JSON is prohibited in all designs.

## Future retrieval requirements

Future approved SEARCH and BUSMAP retrieval behavior shall first consult Cassandra metadata. If required metadata is absent, the TripDetails content is unavailable. If metadata identifies a Stage, the Master shall retrieve the corresponding Trip and Stage values from Dragonfly. SEARCH reconstruction merges them with the Trip `bus` object as the sole canonical Search bus source; a historic Stage document containing `bus` is read without that member. The matching BUSMAP value is read separately when required and present. Reconstruction must preserve the approved Bits response shape and never expose internal cache keys.

This addendum authorizes no retrieval endpoint or response contract. It defines only the storage and lookup prerequisite for a future approved API.

## Data preservation and integrity

Phase 2 storage shall preserve arbitrary nested and unknown Bits fields. It shall not rename, remove, default, coerce, or semantically validate fields. It shall not create fake stages, empty arrays, empty layouts, or seat records. The five fixed Stage fields—`tripStageCode`, `fromStation`, `toStation`, `stationPoint`, and `stageFare[]`—are stored per Stage; every other supplied SEARCH field, including unknown fields, is stored in the Trip document. Cassandra must never become a fallback content store.

## Failure, atomicity, and update requirements

Dragonfly and Cassandra are separate systems and cannot be assumed to share an atomic transaction. The implementation must log non-sensitive diagnostics, avoid logging Request body content in errors, and must not acknowledge a request as successfully cache-backed when the approved write-consistency policy has not been met.

The required outcome for Dragonfly-success/Cassandra-failure, Cassandra-success/Dragonfly-failure, both failures, cleanup or compensation, retries, and recovery is unresolved. Likewise, handling repeated TripDetails, later updates, older updates, concurrent writes, and conflicting common content is unresolved. No TTL or expiration policy is defined or implied.

## Acceptance criteria

1. Existing Phase 1 request validation, logging, endpoint, and HTTP outcomes remain unchanged.
2. Cassandra records only metadata and never complete TripDetails JSON or arbitrary payload content.
3. Dragonfly uses independent Trip, Stage, and supplied BUSMAP keys with the specified identifiers.
4. Multiple Stages for one Trip create independent Stage keys and never overwrite one another.
5. SEARCH creates or updates Trip and Stage content, preserves aggregate `availableSeatCount`, and does not fabricate BUSMAP data.
6. BUSMAP preserves the complete supplied response entry, including `seatLayoutList[]`, formatting, optional fields, unknown fields, and array order, under the correct Stage-specific key.
7. Future metadata lookup can return one or more matching `tripStageCode` values without data loss.
8. Unknown and nested supplied fields survive approved lossless storage and reconstruction rules.
9. Cache-backed success is not acknowledged after a failed required Dragonfly or Cassandra operation.
10. SEARCHBUSMAP receives no guessed cache structure.

## Approved implementation decisions

- Valid JSON that cannot provide `operatorCode`, `tripCode`, `tripStageCode`, `travelDate`, `fromStation.code`, or `toStation.code` required for persistence SHALL return the existing server-error outcome. No fake key, default identifier, or persistence bypass is permitted.
- The Trip key stores every supplied SEARCH field except `tripStageCode`, `fromStation`, `toStation`, `stationPoint`, and `stageFare[]`. It is the only SEARCH cache document that stores `bus`, excluding `bus.seatLayoutList`. The Stage key stores only those five Stage fields. Historic Stage documents that contain `bus` remain readable, but that member is ignored during response reconstruction. The matching BUSMAP key stores the complete supplied BUSMAP response entry and is served independently to preserve BUSMAP contract parity.
- Worker envelopes with `orbitResponse.data[]` and direct Bits responses with `data[]` are supported. A Worker BUSMAP response may instead supply one complete Trip entry directly as `orbitResponse`, provided that it has no `data` member, or as one object in `orbitResponse.data`. A direct Bits BUSMAP response may supply a single entry in `data` rather than a `data[]` list; when its `actionType` is absent and it supplies `seatLayoutList[]`, the Master classifies it as BUSMAP only for storage. The supplied Worker `operatorCode` and `actionType` take precedence when present.
- `SEARCHBUSMAP` remains Phase 2 storage TODO. A valid request is logged and receives the existing success response, but creates no Dragonfly or Cassandra record.
- The Cassandra metadata table is optimized for `operatorCode + tripCode + travelDate + fromStationCode + toStationCode` lookup and supports zero, one, or many `tripStageCode` values. A unique Stage row for those lookup components prevents duplicate logical records.
- Required Dragonfly and Cassandra writes must both succeed before the existing success outcome is returned. A failure returns the existing server-error outcome with non-sensitive diagnostics; compensation and retry are not implemented.
- The latest successfully received value replaces the previous value for the same cache key or metadata record. No freshness, version, timestamp comparison, TTL, expiration, invalidation, or scheduling is performed.

## Remaining Phase 2 unresolved decisions

- Dragonfly and Cassandra production connection settings, credentials, TLS, availability/readiness, timeout values, and operational health requirements.
- Future SEARCH/BUSMAP retrieval endpoint contracts, response shapes, missing-data behavior, and how an API selects among multiple matching stages.
