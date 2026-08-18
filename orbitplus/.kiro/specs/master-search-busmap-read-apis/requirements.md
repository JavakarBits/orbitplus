# Requirements Document

## Introduction

OrbitPlus MasterService receives and persists Worker TripDetails. This feature adds two read-only, BusIQ-compatible APIs that reconstruct already persisted SEARCH and BUSMAP data. Each read consults Cassandra metadata first and then only the necessary Dragonfly JSON. It never calls an upstream service. Ingestion and its persistence behavior are not changed.

## Public API contract

The MasterService SHALL register these exact routes and bind every brace-delimited path segment as named:

- `GET /busservices/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/{fromCode}/{toCode}/{tripDate}`
- `GET /busservices/api/3.0/json/{operatorCode}/{username}/{apiToken}/busmap/{tripCode}/{fromStationCode}/{toStationCode}/{travelDate}`

`username` and `apiToken` are request credentials. They are not lookup data, are not part of Dragonfly or Cassandra keys, and MUST NOT be written to logs.

### Deterministic read outcome convention

The supplied consumer contract does not define a no-data or invalid-request body. For this feature, MasterService SHALL use the following convention, matching the established JSON `status`/`message` style:

| Outcome | HTTP status | JSON body |
| --- | --- | --- |
| Route is malformed or a bound required lookup field is empty | 400 | `{"status":0,"message":"Invalid request"}` |
| Metadata is absent, no candidate passes resolution, or required selected content is absent | 404 | `{"status":0,"message":"Trip details not found"}` |
| Cassandra/Dragonfly/reconstruction failure | 500 | `{"status":0,"message":"Internal server error"}` |

A successful response retains the established BusIQ response status, envelope, and JSON data shape for the applicable API. No-data is never represented as an invented successful empty data/seat-layout response.

## Glossary

- **Search Read API**: The exact `GET .../search/...` route above.
- **BUSMAP Read API**: The exact `GET .../busmap/...` route above.
- **Route Lookup**: `operatorCode`, origin station code, destination station code, and travel date from a read request. For BUSMAP, `tripCode` is an additional lookup constraint.
- **Stage Metadata**: Cassandra lookup-only records identifying candidate `tripCode` and `tripStageCode`; never complete TripDetails JSON.
- **Candidate Stage**: One trip/stage pair returned by Stage Metadata.
- **Trip Content**, **Stage Content**, and **BUSMAP Content**: Raw Dragonfly JSON at `trip:{operatorCode}:{tripCode}`, `stage:{operatorCode}:{tripCode}:{tripStageCode}`, and `busmap:{operatorCode}:{tripCode}:{tripStageCode}` respectively.
- **Stage Resolver**: The shared operation that evaluates Candidate Stages against a Route Lookup.
- **Ordered-Station Rule**: A candidate is selected only when its stored Stage Content contains the requested origin before the requested destination in its station order; stations may be intermediate, not only terminals.
- **Raw preservation**: Preservation of stored JSON field names, numbers, strings, booleans, objects, arrays, unknown fields, nested values, array order, and omitted-versus-`null` distinctions. It prohibits a fixed struct from silently dropping or coercing stored values.
- **Upstream Service**: Any TripDetails data source other than Cassandra or Dragonfly.
- **Non-sensitive Diagnostic**: A failure log containing an operation and failure classification, but no request credential, dependency credential, or complete stored payload.

## Requirements

### Requirement 1: Preserve ingestion and persistence

**User story:** As a Worker, I want existing TripDetails ingestion and persistence unchanged so that current integrations remain stable.

#### Acceptance criteria

1. WHEN valid ingestion completes, THE MasterService SHALL retain `200` with `{"status":1,"message":"Trip details received successfully"}`.
2. WHEN invalid ingestion JSON is the first processing error, THE MasterService SHALL retain `400` with `{"status":0,"message":"Invalid trip details JSON"}`.
3. WHEN a body-read or required persistence error is the first processing error, THE MasterService SHALL retain `500` with `{"status":0,"message":"Internal server error"}`.
4. WHEN cacheable SEARCH or BUSMAP ingestion succeeds, THE MasterService SHALL retain the existing Trip, Stage, BUSMAP, and Stage Metadata write behavior and key formats.
5. WHEN `SEARCHBUSMAP` ingestion succeeds, THE MasterService SHALL retain the existing behavior of creating no new Dragonfly or Stage Metadata record.

### Requirement 2: Register and serve Search reads

**User story:** As a BusIQ consumer, I want to retrieve persisted SEARCH results for a route without a live upstream request.

#### Acceptance criteria

1. WHEN a consumer sends the exact Search Read API path, THE MasterService SHALL bind `operatorCode`, `username`, `apiToken`, `fromCode`, `toCode`, and `tripDate` from it.
2. WHEN a valid Search lookup is received, THE MasterService SHALL query Stage Metadata using `operatorCode`, `fromCode`, `toCode`, and `tripDate` before reading any Dragonfly content.
3. WHEN Stage Metadata returns Candidate Stages, THE MasterService SHALL pass them and the Route Lookup to the shared Stage Resolver.
4. WHEN the resolver selects Route Stages, THE MasterService SHALL retrieve the raw Trip Content and retain the raw Stage Content used to select each stage.
5. WHEN selected Trip Content is missing for one candidate but available for another, THE MasterService SHALL omit only the candidate with missing Trip Content and return every available selected candidate.
6. WHEN multiple selected candidates have Trip Content, THE MasterService SHALL return each as a distinct SEARCH data item in resolver-selected order.
7. WHEN reconstructing a successful SEARCH response, THE MasterService SHALL preserve raw stored Trip and Stage response fields, including unknown nested values, `stageFare`, order, and omitted-versus-`null` distinctions required by the existing BusIQ response contract.
8. WHEN reconstructing SEARCH data, THE MasterService SHALL NOT fabricate, read, or emit `seatLayoutList` unless the established SEARCH contract explicitly requires persisted BUSMAP Content.
9. WHEN metadata is absent, no Candidate Stage resolves, or no selected candidate has Trip Content, THE MasterService SHALL return the documented `404` outcome without reading unrelated Dragonfly keys or calling an Upstream Service.
10. WHEN response reconstruction cannot preserve required raw stored data, THE MasterService SHALL return the documented `500` outcome and SHALL NOT emit a partial success response.

### Requirement 3: Resolve stages consistently, including intermediate stations

**User story:** As a BusIQ consumer, I want both read APIs to select the same persisted route stage, including journeys between intermediate stations.

#### Acceptance criteria

1. WHEN either Read API receives Candidate Stages, THE MasterService SHALL invoke the same Stage Resolver with the applicable Route Lookup.
2. WHEN a candidate's stored station order contains the origin before the destination, THE Stage Resolver SHALL select that candidate.
3. WHEN either requested station is absent, the destination is not after the origin, or station-order evaluation fails, THE Stage Resolver SHALL exclude that candidate and continue evaluating the remaining candidates.
4. WHEN origin or destination is an intermediate station in Stage Content, THE Stage Resolver SHALL apply the same Ordered-Station Rule used for terminal stations.
5. WHEN one or more candidates satisfy the Ordered-Station Rule and others do not or fail evaluation, THE Stage Resolver SHALL return exactly the selected candidates in their metadata order.
6. WHEN no candidate satisfies the Ordered-Station Rule, THE Stage Resolver SHALL return no Route Stages.
7. WHEN equivalent Route Lookup values and Candidate Stage Content are supplied through Search and BUSMAP, THE Stage Resolver SHALL return equivalent Route Stages independent of its caller.

### Requirement 4: Serve BUSMAP reads

**User story:** As a BusIQ consumer, I want to retrieve a persisted BUSMAP seat layout for a selected route without a live upstream request.

#### Acceptance criteria

1. WHEN a consumer sends the exact BUSMAP Read API path, THE MasterService SHALL bind `operatorCode`, `username`, `apiToken`, `tripCode`, `fromStationCode`, `toStationCode`, and `travelDate` from it.
2. WHEN a valid BUSMAP lookup is received, THE MasterService SHALL query Stage Metadata using `operatorCode`, `tripCode`, `fromStationCode`, `toStationCode`, and `travelDate` before reading Dragonfly content.
3. WHEN metadata returns Candidate Stages, THE MasterService SHALL invoke the shared Stage Resolver with the BUSMAP Route Lookup and Candidate Stages.
4. WHEN the resolver selects a stage, THE MasterService SHALL read BUSMAP Content only with that selected candidate's `operatorCode`, `tripCode`, and `tripStageCode`.
5. WHEN BUSMAP Content is available, THE MasterService SHALL preserve every raw `seatLayoutList` item, empty list, unknown field, nested value, item order, and omitted-versus-`null` distinction required by the existing BusIQ BUSMAP response contract.
6. WHEN metadata is absent, no candidate resolves, or required BUSMAP Content is absent, THE MasterService SHALL return the documented `404` outcome without creating an empty `seatLayoutList`, a synthetic seat, or a partial success response.
7. WHEN required BUSMAP Content cannot be read or preserved, THE MasterService SHALL return the documented `500` outcome without calling an Upstream Service.

### Requirement 5: Restrict sources and protect credentials

**User story:** As an operator, I want deterministic persisted-store reads and credential-safe diagnostics so that the service neither exposes secrets nor introduces an unapproved upstream dependency.

#### Acceptance criteria

1. WHEN either Read API processes a valid lookup, THE MasterService SHALL use Cassandra Stage Metadata first and Dragonfly Trip, Stage, or BUSMAP Content only as required by the applicable response.
2. WHEN either Read API processes any request, THE MasterService SHALL perform zero Upstream Service calls.
3. WHEN Cassandra returns an error, THE MasterService SHALL return the documented `500` outcome and SHALL NOT read Dragonfly content.
4. WHEN Dragonfly returns an error, THE MasterService SHALL return the documented `500` outcome and SHALL NOT call an Upstream Service.
5. WHEN handling either Read API, connecting to dependencies, or logging a read failure, THE MasterService SHALL exclude `apiToken`, `username`, and all dependency credentials from logs.
6. WHEN a Read API failure is logged, THE MasterService SHALL write a Non-sensitive Diagnostic.
7. WHEN a read route is malformed or contains an empty required bound lookup field, THE MasterService SHALL return the documented `400` outcome without reading Cassandra, Dragonfly, or an Upstream Service.

## Executable correctness properties

Implementation tasks SHALL automate these properties using in-memory Cassandra, Dragonfly, logger, and upstream fakes. They make no network calls.

1. **Exact-route binding (example-based):** Requests to each supplied literal route bind every named segment to its corresponding lookup or credential value. Missing/malformed route values yield exactly `400` and `{"status":0,"message":"Invalid request"}` with no dependency calls.
2. **Metadata-first, source-isolated reads (property-based):** For every generated valid Search or BUSMAP lookup and every generated metadata/cache outcome, execution records Cassandra before Dragonfly, makes zero upstream calls, and does no Dragonfly read after metadata error or empty metadata. Search metadata queries use operator/route/date; BUSMAP queries also use trip code.
3. **Shared intermediate-station resolution (property-based):** For every generated ordered station sequence and origin/destination pair, the resolver selects exactly candidates where origin occurs before destination, including intermediate pairs. Equivalent Search and BUSMAP inputs yield equivalent selected candidates in metadata order.
4. **SEARCH raw-preservation (property-based):** For arbitrary stored Trip and Stage JSON with unknown nested values, numbers, arrays, `null`s, omitted fields, and `stageFare`, successful reconstruction retains all contract-required raw values and selected order. It introduces no `seatLayoutList` unless the approved SEARCH contract requires BUSMAP Content.
5. **BUSMAP raw-preservation (property-based):** For arbitrary stored BUSMAP JSON containing ordered `seatLayoutList` entries, including unknown nested content and an available empty list, successful reconstruction is structurally equivalent. Missing BUSMAP Content produces exactly the documented `404` body and creates no empty list or synthetic seat.
6. **Deterministic outcomes (example-based):** Metadata miss, no resolved stage, and required selected-content miss return `404` with `{"status":0,"message":"Trip details not found"}`. Cassandra, Dragonfly, and reconstruction failures return `500` with `{"status":0,"message":"Internal server error"}`. No case returns an invented successful empty data response.
7. **Credential-safe diagnostics (property-based):** For arbitrary generated `username`, `apiToken`, and dependency-credential markers across Cassandra and Dragonfly failures, captured logs contain none of those markers and contain an operation plus failure classification.
8. **Ingestion regression (example-based):** Valid SEARCH and BUSMAP ingestion, invalid JSON, body-read failure, persistence failure, and SEARCHBUSMAP ingestion retain the outcomes and persistence behavior in Requirement 1.

## Scope boundary

These paths and parameters are supplied, available public contracts; implementation MUST register them exactly as stated. Retrieval is strictly Cassandra metadata followed by the necessary Dragonfly content, with common stage resolution for intermediate stations and no upstream fallback. Read success reconstructs the established BusIQ response from raw stored JSON without schema-driven loss. Ingestion, persistence layout, freshness, retries, authentication policy, and any change to existing ingestion behavior remain out of scope.
