package master

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"orbitplusmaster/internal/domain"
)

// maxCollectedDifferences bounds how many DifferenceEntry values one comparison
// retains. The walk continues past the cap so the reported total stays accurate;
// only collection stops, so a wholly different multi-megabyte document cannot
// exhaust memory.
const maxCollectedDifferences = 1000

// entryRootPath marks a difference that concerns a whole entry rather than a
// field inside it. Member paths are unprefixed, so this token cannot collide
// with one.
const entryRootPath = "$"

// canonicalValue is a JSON value normalised so that representation differences
// do not register as content differences.
//
// The normalisation rules, and why each exists:
//   - object member order is irrelevant: the two producers serialise in
//     different orders and that is not a change
//   - an absent member and a member whose value is null are equal: the same
//     reason
//   - numbers compare by numeric value, not by text, so 1, 1.0, 1e0 and 0.10
//     versus 0.1 are equal; fares are the field most likely to differ by
//     formatting alone, and a float64 round trip would either mask a real
//     difference or invent one
//   - array order IS significant: dropping or reordering elements changes
//     indices, and a reordering is a real change in a seat layout
type canonicalValue interface {
	render() string
}

type canonicalNull struct{}
type canonicalBool bool
type canonicalString string
type canonicalNumber struct{ value *big.Rat }
type canonicalArray []canonicalValue
type canonicalObject struct {
	names  []string // sorted
	values map[string]canonicalValue
}

func (canonicalNull) render() string          { return "null" }
func (value canonicalBool) render() string    { return strconv.FormatBool(bool(value)) }
func (value canonicalString) render() string  { return strconv.Quote(string(value)) }
func (value canonicalNumber) render() string  { return value.value.RatString() }

func (value canonicalArray) render() string {
	parts := make([]string, len(value))
	for index, element := range value {
		parts[index] = element.render()
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func (value canonicalObject) render() string {
	parts := make([]string, 0, len(value.names))
	for _, name := range value.names {
		parts = append(parts, strconv.Quote(name)+":"+value.values[name].render())
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// canonicaliseJSON decodes raw JSON into canonical form.
//
// UseNumber keeps numbers as their original text, matching the ingestion path,
// so no float64 rounding happens before comparison.
func canonicaliseJSON(raw json.RawMessage) (canonicalValue, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode JSON for comparison: %w", err)
	}
	return canonicalise(decoded), nil
}

// canonicalise rebuilds every container into a new value, so it never writes
// through a pointer into its input. That is how comparison leaves both inputs
// untouched: not by copying defensively at the boundary, but by never mutating.
func canonicalise(value any) canonicalValue {
	switch typed := value.(type) {
	case nil:
		return canonicalNull{}
	case bool:
		return canonicalBool(typed)
	case string:
		return canonicalString(typed)
	case json.Number:
		if rational, ok := new(big.Rat).SetString(typed.String()); ok {
			return canonicalNumber{value: rational}
		}
		// Unreachable from encoding/json, but a hand-built input must not panic.
		return canonicalString(typed.String())
	case []any:
		elements := make(canonicalArray, len(typed))
		for index, element := range typed {
			elements[index] = canonicalise(element)
		}
		return elements
	case map[string]any:
		object := canonicalObject{values: make(map[string]canonicalValue, len(typed))}
		for name, member := range typed {
			if member == nil {
				// Absent and explicitly null are the same thing.
				continue
			}
			object.values[name] = canonicalise(member)
			object.names = append(object.names, name)
		}
		sort.Strings(object.names)
		return object
	default:
		return canonicalString(fmt.Sprint(typed))
	}
}

// CompareEntries compares one cached entry against one Bits entry and returns
// the differences plus the untruncated total.
func CompareEntries(identity domain.EntryIdentity, cache, bits json.RawMessage) ([]domain.DifferenceEntry, int, error) {
	cacheValue, err := canonicaliseJSON(cache)
	if err != nil {
		return nil, 0, err
	}
	bitsValue, err := canonicaliseJSON(bits)
	if err != nil {
		return nil, 0, err
	}
	collector := &differenceCollector{identity: identity}
	collector.walk("", cacheValue, bitsValue)
	return collector.entries, collector.total, nil
}

type differenceCollector struct {
	identity domain.EntryIdentity
	entries  []domain.DifferenceEntry
	total    int
}

func (collector *differenceCollector) add(path string, kind domain.DifferenceKind, cacheValue, bitsValue string) {
	collector.total++
	if len(collector.entries) >= maxCollectedDifferences {
		return
	}
	if path == "" {
		path = entryRootPath
	}
	collector.entries = append(collector.entries, domain.DifferenceEntry{
		Identity:   collector.identity,
		Path:       path,
		Kind:       kind,
		CacheValue: cacheValue,
		BitsValue:  bitsValue,
	})
}

// walk descends both canonical values in lockstep. The traversal is
// deterministic: object members in sorted order, array indices ascending,
// depth first. Two runs over the same inputs therefore produce the same paths
// in the same order, which keeps stored rows and test failures reproducible.
func (collector *differenceCollector) walk(path string, cache, bits canonicalValue) {
	cacheObject, cacheIsObject := cache.(canonicalObject)
	bitsObject, bitsIsObject := bits.(canonicalObject)
	if cacheIsObject && bitsIsObject {
		for _, name := range unionNames(cacheObject, bitsObject) {
			cacheMember, inCache := cacheObject.values[name]
			bitsMember, inBits := bitsObject.values[name]
			switch {
			case inCache && inBits:
				collector.walk(appendMember(path, name), cacheMember, bitsMember)
			case inCache:
				collector.add(appendMember(path, name), domain.DifferenceValueMismatch, cacheMember.render(), "null")
			default:
				collector.add(appendMember(path, name), domain.DifferenceValueMismatch, "null", bitsMember.render())
			}
		}
		return
	}

	cacheArray, cacheIsArray := cache.(canonicalArray)
	bitsArray, bitsIsArray := bits.(canonicalArray)
	if cacheIsArray && bitsIsArray {
		longest := len(cacheArray)
		if len(bitsArray) > longest {
			longest = len(bitsArray)
		}
		for index := 0; index < longest; index++ {
			indexPath := path + "[" + strconv.Itoa(index) + "]"
			switch {
			case index < len(cacheArray) && index < len(bitsArray):
				collector.walk(indexPath, cacheArray[index], bitsArray[index])
			case index < len(cacheArray):
				collector.add(indexPath, domain.DifferenceValueMismatch, cacheArray[index].render(), "null")
			default:
				collector.add(indexPath, domain.DifferenceValueMismatch, "null", bitsArray[index].render())
			}
		}
		return
	}

	// Different kinds, or two scalars. Compare renderings and do not recurse.
	cacheRendered, bitsRendered := cache.render(), bits.render()
	if cacheRendered != bitsRendered {
		collector.add(path, domain.DifferenceValueMismatch, cacheRendered, bitsRendered)
	}
}

func unionNames(left, right canonicalObject) []string {
	seen := make(map[string]struct{}, len(left.names)+len(right.names))
	names := make([]string, 0, len(left.names)+len(right.names))
	for _, name := range append(append([]string{}, left.names...), right.names...) {
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// appendMember formats a member step. A name containing the path syntax itself,
// or an empty name, is bracketed and quoted so that every produced path stays
// unambiguous and resolvable.
func appendMember(path, name string) string {
	if name != "" && !strings.ContainsAny(name, `.[]"`) {
		return path + "." + name
	}
	return path + "[" + strconv.Quote(name) + "]"
}

// EntryIdentityOf reads the pairing identity from a Bits entry. A missing or
// non-string member yields the empty string for that component.
func EntryIdentityOf(raw json.RawMessage) domain.EntryIdentity {
	var members map[string]any
	if err := json.Unmarshal(raw, &members); err != nil {
		return domain.EntryIdentity{}
	}
	text := func(name string) string {
		value, _ := members[name].(string)
		return value
	}
	return domain.EntryIdentity{
		OperatorCode:  text("operatorCode"),
		TripCode:      text("tripCode"),
		TripStageCode: text("tripStageCode"),
	}
}
