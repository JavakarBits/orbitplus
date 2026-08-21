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

// errInvalidCacheFlag reports a Cache_Flag that is absent from the accepted
// set, undecodable, or supplied more than once.
var errInvalidCacheFlag = errors.New("invalid cache flag")

// parseCacheFlag reads the Cache_Flag from a raw query string.
//
// This walks rawQuery itself rather than using url.ParseQuery because
// url.Values cannot express the three outcomes this feature needs to keep
// apart: absent, present but undecodable, and present more than once.
// url.ParseQuery collapses repeats into a slice only after silently dropping
// pairs it failed to unescape, which would let a malformed flag be read as
// absent and therefore be served from cache.
//
// Comparison is byte-for-byte against "0" and "1" with no trimming, no numeric
// normalisation, and no case folding, so "00", "+1", " 1" and "true" are all
// rejected rather than being coerced to a value the caller did not ask for.
func parseCacheFlag(rawQuery string) (int, error) {
	flag := cacheFlagFromCache
	found := false

	for remainder := rawQuery; remainder != ""; {
		var pair string
		pair, remainder, _ = strings.Cut(remainder, "&")
		if pair == "" {
			continue
		}
		rawName, rawValue, _ := strings.Cut(pair, "=")

		name, err := url.QueryUnescape(rawName)
		if err != nil || name != cacheFlagParameter {
			// A parameter this feature does not own, including one whose name
			// cannot be decoded, is ignored rather than rejected.
			continue
		}
		if found {
			return cacheFlagFromCache, errInvalidCacheFlag
		}
		found = true

		value, err := url.QueryUnescape(rawValue)
		if err != nil {
			return cacheFlagFromCache, errInvalidCacheFlag
		}
		switch value {
		case "1":
			flag = cacheFlagFromCache
		case "0":
			flag = cacheFlagFromLive
		default:
			return cacheFlagFromCache, errInvalidCacheFlag
		}
	}
	return flag, nil
}
