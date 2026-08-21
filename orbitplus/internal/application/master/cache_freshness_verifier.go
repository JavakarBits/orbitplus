package master

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"orbitplusmaster/internal/domain"
)

// ErrVerificationBusy reports that every live verification slot is occupied.
// The caller receives HTTP 429 and no outbound Bits request is made.
var ErrVerificationBusy = errors.New("live verification busy")

// differenceWriteTimeout bounds the background difference write. It is
// independent of the caller's request, so a disconnect cannot cancel a
// recording and a slow Cassandra cannot slow a read.
const differenceWriteTimeout = 5 * time.Second

// VerifyResult is what the read handler needs to answer the caller.
type VerifyResult struct {
	// Data is the Bits data member, passed through unchanged.
	Data json.RawMessage
	// DataEmpty reports a data member of null, an empty array, or an empty
	// object, which the handler turns into 404.
	DataEmpty bool
	// Outcome and DifferenceCount are for logging; the row is already recorded.
	Outcome         domain.VerificationOutcome
	DifferenceCount int
}

// CacheFreshnessVerifier serves a Cache_Flag value of 0: it fetches the lookup
// live from Bits, compares that against the cached copy, and records any
// difference.
//
// The concurrency cap exists because the read routes this hangs off are
// unauthenticated. Without it, anyone able to reach master could drive unbounded
// load onto Bits.
type CacheFreshnessVerifier struct {
	fetcher BitsTripDetailsFetcher
	reader  *TripDetailsReadService
	writer  CacheDifferenceWriter
	// repairer overwrites the cached copy with the live copy after a difference.
	// It is the same storage path the Worker callback uses, so the repaired
	// documents are split and indexed identically.
	repairer *TripDetailsStorage
	// slots is a counting semaphore. A send acquires, a receive releases.
	slots  chan struct{}
	now    func() time.Time
	logger *log.Logger
}

// NewCacheFreshnessVerifier constructs the verifier. A nil reader, writer, or
// repairer disables comparison, recording, or repair respectively, while still
// serving the live copy.
func NewCacheFreshnessVerifier(
	fetcher BitsTripDetailsFetcher,
	reader *TripDetailsReadService,
	writer CacheDifferenceWriter,
	repairer *TripDetailsStorage,
	maxConcurrent int,
	logger *log.Logger,
) (*CacheFreshnessVerifier, error) {
	if fetcher == nil {
		return nil, fmt.Errorf("Bits fetcher is required for live verification")
	}
	if maxConcurrent < 1 {
		return nil, fmt.Errorf("live verification concurrency must be at least 1")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &CacheFreshnessVerifier{
		fetcher:  fetcher,
		reader:   reader,
		writer:   writer,
		repairer: repairer,
		slots:    make(chan struct{}, maxConcurrent),
		now:      time.Now,
		logger:   logger,
	}, nil
}

// Verify fetches the lookup live from Bits, compares it against the cached copy,
// and records a difference row for every non-matching outcome.
//
// The slot is acquired before the fetcher is touched, so a request rejected for
// capacity makes no outbound call. It is released before the recording is
// dispatched, so upstream concurrency is bounded by upstream work only.
func (verifier *CacheFreshnessVerifier) Verify(ctx context.Context, lookup BitsLookup, remoteAddress string) (VerifyResult, error) {
	select {
	case verifier.slots <- struct{}{}:
	default:
		verifier.logger.Printf("live verification rejected: remote_addr=%q action=%s operator=%q outcome=BUSY",
			remoteAddress, lookup.Action, lookup.OperatorCode)
		return VerifyResult{}, ErrVerificationBusy
	}

	result, fetchErr := verifier.fetcher.FetchTripDetails(ctx, lookup)
	var outcome domain.VerificationOutcome
	var differences []domain.DifferenceEntry
	var total int

	if fetchErr != nil {
		outcome = domain.OutcomeSourceUnavailable
	} else {
		outcome, differences, total = verifier.compareAgainstCache(ctx, lookup, result)
	}
	<-verifier.slots

	// Repair before recording, so the row states whether the cache was fixed.
	repaired := false
	if outcome == domain.OutcomeDifferent || outcome == domain.OutcomeCacheMissing {
		repaired = verifier.repairCache(ctx, lookup, result)
	}
	if outcome != domain.OutcomeMatch {
		verifier.record(lookup, outcome, differences, total, repaired)
	}
	verifier.logger.Printf("live verification completed: remote_addr=%q action=%s operator=%q outcome=%s differences=%d repaired=%t",
		remoteAddress, lookup.Action, lookup.OperatorCode, outcome, total, repaired)

	if fetchErr != nil {
		return VerifyResult{Outcome: outcome}, fetchErr
	}
	return VerifyResult{
		Data:            result.Data,
		DataEmpty:       result.Empty,
		Outcome:         outcome,
		DifferenceCount: total,
	}, nil
}

// compareAgainstCache reconstructs the cached copy and compares it entry by
// entry against the live copy.
func (verifier *CacheFreshnessVerifier) compareAgainstCache(ctx context.Context, lookup BitsLookup, result BitsResult) (
	domain.VerificationOutcome, []domain.DifferenceEntry, int,
) {
	if verifier.reader == nil {
		return domain.OutcomeSourceUnavailable, nil, 0
	}
	cacheEntries, err := verifier.cachedEntries(ctx, lookup)
	if err != nil {
		if errors.Is(err, ErrTripDetailsNotFound) {
			if result.Empty {
				// Neither side has anything. Nothing is stale.
				return domain.OutcomeMatch, nil, 0
			}
			return domain.OutcomeCacheMissing, nil, 0
		}
		return domain.OutcomeSourceUnavailable, nil, 0
	}

	bitsEntries := splitBitsEntries(result)
	differences, total := pairAndCompare(cacheEntries, bitsEntries)
	if total == 0 {
		return domain.OutcomeMatch, nil, 0
	}
	return domain.OutcomeDifferent, differences, total
}

// repairCache overwrites the cached copy with the live copy.
//
// It reuses TripDetailsStorage, the same path the Worker callback uses, so the
// repaired documents are split into trip and stage projections and indexed in
// Cassandra exactly as a normal refresh would do it. Reimplementing the split
// here would be a second place to keep in sync.
//
// Note this makes verification non-repeatable: a second cacheFlag=0 call for the
// same lookup will report MATCH, because the first call fixed it.
func (verifier *CacheFreshnessVerifier) repairCache(ctx context.Context, lookup BitsLookup, result BitsResult) bool {
	if verifier.repairer == nil || result.Empty || len(result.Data) == 0 {
		return false
	}
	envelope := map[string]any{
		"actionType":   strings.ToLower(lookup.Action),
		"operatorCode": lookup.OperatorCode,
	}
	var orbitResponse any
	if err := json.Unmarshal([]byte(`{"data":`+string(result.Data)+`}`), &orbitResponse); err != nil {
		verifier.logger.Printf("cache repair skipped: action=%s operator=%q reason=unreadable_live_copy",
			lookup.Action, lookup.OperatorCode)
		return false
	}
	envelope["orbitResponse"] = orbitResponse

	if err := verifier.repairer.Store(ctx, envelope); err != nil {
		verifier.logger.Printf("cache repair failed: action=%s operator=%q error=%v",
			lookup.Action, lookup.OperatorCode, err)
		return false
	}
	verifier.logger.Printf("cache repaired from live copy: action=%s operator=%q",
		lookup.Action, lookup.OperatorCode)
	return true
}

// cachedEntries reads the cached copy through the same service that serves
// cacheFlag=1, so both paths see identical data.
func (verifier *CacheFreshnessVerifier) cachedEntries(ctx context.Context, lookup BitsLookup) ([]json.RawMessage, error) {
	routeLookup := RouteLookup{
		OperatorCode: lookup.OperatorCode,
		TripCode:     lookup.TripCode,
		FromCode:     lookup.FromCode,
		ToCode:       lookup.ToCode,
		TravelDate:   lookup.TravelDate,
	}
	if lookup.Action == BitsActionBusMap {
		entry, err := verifier.reader.BusMap(ctx, routeLookup)
		if err != nil {
			return nil, err
		}
		return []json.RawMessage{entry}, nil
	}
	entries, err := verifier.reader.Search(ctx, routeLookup)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// splitBitsEntries normalises the live data member into a list of entries, so
// search arrays and busmap objects compare the same way.
func splitBitsEntries(result BitsResult) []json.RawMessage {
	if result.Empty || len(result.Data) == 0 {
		return nil
	}
	if result.DataKind == BitsDataKindArray {
		var entries []json.RawMessage
		if err := json.Unmarshal(result.Data, &entries); err != nil {
			return nil
		}
		return entries
	}
	return []json.RawMessage{result.Data}
}

// pairAndCompare matches entries on identity and compares each pair. Entries
// present on only one side are reported whole rather than field by field.
func pairAndCompare(cacheEntries, bitsEntries []json.RawMessage) ([]domain.DifferenceEntry, int) {
	var differences []domain.DifferenceEntry
	total := 0

	cacheByIdentity := make(map[domain.EntryIdentity]json.RawMessage, len(cacheEntries))
	cacheOrder := make([]domain.EntryIdentity, 0, len(cacheEntries))
	for _, entry := range cacheEntries {
		identity := EntryIdentityOf(entry)
		if _, exists := cacheByIdentity[identity]; !exists {
			cacheOrder = append(cacheOrder, identity)
		}
		cacheByIdentity[identity] = entry
	}

	matched := make(map[domain.EntryIdentity]struct{}, len(bitsEntries))
	for _, bitsEntry := range bitsEntries {
		identity := EntryIdentityOf(bitsEntry)
		cacheEntry, exists := cacheByIdentity[identity]
		if !exists {
			total++
			differences = append(differences, domain.DifferenceEntry{
				Identity: identity, Path: entryRootPath,
				Kind: domain.DifferenceMissingInCache, BitsValue: string(bitsEntry),
			})
			continue
		}
		matched[identity] = struct{}{}
		entryDifferences, entryTotal, err := CompareEntries(identity, cacheEntry, bitsEntry)
		if err != nil {
			total++
			differences = append(differences, domain.DifferenceEntry{
				Identity: identity, Path: entryRootPath, Kind: domain.DifferenceValueMismatch,
			})
			continue
		}
		total += entryTotal
		differences = append(differences, entryDifferences...)
	}

	for _, identity := range cacheOrder {
		if _, exists := matched[identity]; exists {
			continue
		}
		total++
		differences = append(differences, domain.DifferenceEntry{
			Identity: identity, Path: entryRootPath,
			Kind: domain.DifferenceMissingInBits, CacheValue: string(cacheByIdentity[identity]),
		})
	}
	return differences, total
}

// record writes the difference row in the background. A read must never wait on
// a recording, and a failed recording must never change the response.
func (verifier *CacheFreshnessVerifier) record(
	lookup BitsLookup,
	outcome domain.VerificationOutcome,
	differences []domain.DifferenceEntry,
	total int,
	repaired bool,
) {
	if verifier.writer == nil {
		return
	}
	record := buildRecordedDifference(lookup, outcome, differences, total, repaired, verifier.now())

	go func() {
		writeCtx, cancel := context.WithTimeout(context.Background(), differenceWriteTimeout)
		defer cancel()
		if err := verifier.writer.SaveDifference(writeCtx, record); err != nil {
			verifier.logger.Printf("cache difference write failed: operator=%q action=%s %s error=%v",
				record.OperatorCode, record.ActionType, describeOutcome(record), err)
			return
		}
		verifier.logger.Printf("cache difference recorded: operator=%q action=%s %s",
			record.OperatorCode, record.ActionType, describeOutcome(record))
	}()
}
