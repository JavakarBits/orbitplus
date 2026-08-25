package http

import (
	"errors"
	"net/url"
	"strings"
)

// Cache_Flag values accepted on the two persisted read routes.
const (
	// cacheFlagFromCache serves the persisted Dragonfly copy. This is the
	// value applied when the caller supplies no flag, so existing callers keep
	// today's behaviour.
	cacheFlagFromCache = 1
	// cacheFlagFromLive fetches the same lookup live from Bits and compares it
	// against the persisted copy.
	cacheFlagFromLive = 0
)

// cacheFlagParameter is the query parameter carrying the Cache_Flag. It is a
// query parameter rather than a path segment so that the registered route
// patterns stay unchanged and no existing caller breaks.
const cacheFlagParameter = "cacheFlag"

// zoneParameter is the query parameter naming the Bits zone the live fetch must
// use. It is a query parameter for the same reason cacheFlag is: the path
// mirrors Bits' own route shape, and adding a segment would break every caller.
const zoneParameter = "zone"

// errInvalidCacheFlag reports a Cache_Flag that is absent from the accepted
// set, undecodable, or supplied more than once.
var errInvalidCacheFlag = errors.New("invalid cache flag")

// errInvalidZone reports a zone that is undecodable, blank, or supplied more
// than once.
var errInvalidZone = errors.New("invalid zone")

// errAmbiguousParameter reports a parameter that is undecodable or repeated.
var errAmbiguousParameter = errors.New("ambiguous query parameter")

// singleQueryValue returns the sole decoded value for name.
//
// It walks rawQuery itself rather than using url.ParseQuery because url.Values
// cannot express the three outcomes these parameters need to keep apart:
// absent, present but undecodable, and present more than once. url.ParseQuery
// collapses repeats into a slice only after silently dropping pairs it failed
// to unescape, which would let a malformed value be read as absent.
//
// A parameter this service does not own is ignored, including one whose name
// cannot be decoded, so an unrelated malformed parameter cannot fail a request.
func singleQueryValue(rawQuery, name string) (string, bool, error) {
	value := ""
	found := false

	for remainder := rawQuery; remainder != ""; {
		var pair string
		pair, remainder, _ = strings.Cut(remainder, "&")
		if pair == "" {
			continue
		}
		rawName, rawValue, _ := strings.Cut(pair, "=")

		decodedName, err := url.QueryUnescape(rawName)
		if err != nil || decodedName != name {
			continue
		}
		if found {
			return "", false, errAmbiguousParameter
		}
		found = true

		decodedValue, err := url.QueryUnescape(rawValue)
		if err != nil {
			return "", false, errAmbiguousParameter
		}
		value = decodedValue
	}
	return value, found, nil
}

// parseZoneCode reads the zone from a raw query string. An absent zone returns
// an empty string and no error, so the cached path stays unaffected; requiring
// it is the live path's decision.
func parseZoneCode(rawQuery string) (string, error) {
	value, found, err := singleQueryValue(rawQuery, zoneParameter)
	if err != nil {
		return "", errInvalidZone
	}
	if !found {
		return "", nil
	}
	if strings.TrimSpace(value) == "" {
		return "", errInvalidZone
	}
	return value, nil
}

// parseCacheFlag reads the Cache_Flag from a raw query string.
//
// Comparison is byte-for-byte against "0" and "1" with no trimming, no numeric
// normalisation, and no case folding, so "00", "+1", " 1" and "true" are all
// rejected rather than being coerced to a value the caller did not ask for.
// An absent flag serves from cache, so existing callers keep today's behaviour.
func parseCacheFlag(rawQuery string) (int, error) {
	value, found, err := singleQueryValue(rawQuery, cacheFlagParameter)
	if err != nil {
		return cacheFlagFromCache, errInvalidCacheFlag
	}
	if !found {
		return cacheFlagFromCache, nil
	}
	switch value {
	case "1":
		return cacheFlagFromCache, nil
	case "0":
		return cacheFlagFromLive, nil
	default:
		return cacheFlagFromCache, errInvalidCacheFlag
	}
}
