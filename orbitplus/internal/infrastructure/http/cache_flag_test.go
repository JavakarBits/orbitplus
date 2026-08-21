package http

import (
	"errors"
	"testing"
)

func TestParseCacheFlagAccepted(t *testing.T) {
	testCases := []struct {
		name     string
		rawQuery string
		expected int
	}{
		{name: "absent", rawQuery: "", expected: cacheFlagFromCache},
		{name: "other parameter only", rawQuery: "other=x", expected: cacheFlagFromCache},
		{name: "several other parameters", rawQuery: "a=1&b=2", expected: cacheFlagFromCache},
		{name: "lowercase name is a different parameter", rawQuery: "cacheflag=0", expected: cacheFlagFromCache},
		{name: "short name is a different parameter", rawQuery: "cache=0", expected: cacheFlagFromCache},
		{name: "explicit one", rawQuery: "cacheFlag=1", expected: cacheFlagFromCache},
		{name: "explicit zero", rawQuery: "cacheFlag=0", expected: cacheFlagFromLive},
		{name: "percent encoded one", rawQuery: "cacheFlag=%31", expected: cacheFlagFromCache},
		{name: "percent encoded zero", rawQuery: "cacheFlag=%30", expected: cacheFlagFromLive},
		{name: "percent encoded name", rawQuery: "%63acheFlag=0", expected: cacheFlagFromLive},
		{name: "flag after another parameter", rawQuery: "other=x&cacheFlag=0", expected: cacheFlagFromLive},
		{name: "flag before another parameter", rawQuery: "cacheFlag=0&other=x", expected: cacheFlagFromLive},
		{name: "undecodable other parameter is ignored", rawQuery: "other=%zz&cacheFlag=0", expected: cacheFlagFromLive},
		{name: "empty pair is skipped", rawQuery: "&cacheFlag=0&", expected: cacheFlagFromLive},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			flag, err := parseCacheFlag(testCase.rawQuery)
			if err != nil {
				t.Fatalf("parseCacheFlag(%q) returned error %v, want nil", testCase.rawQuery, err)
			}
			if flag != testCase.expected {
				t.Errorf("parseCacheFlag(%q) = %d, want %d", testCase.rawQuery, flag, testCase.expected)
			}
		})
	}
}

func TestParseCacheFlagRejected(t *testing.T) {
	rawQueries := []string{
		"cacheFlag=",               // empty value
		"cacheFlag=%20 1",          // leading whitespace
		"cacheFlag=1%20",           // trailing whitespace
		"cacheFlag=00",             // leading zero
		"cacheFlag=01",             // leading zero
		"cacheFlag=+1",             // sign, and + decodes to a space
		"cacheFlag=-0",             // sign
		"cacheFlag=2",              // out of range
		"cacheFlag=true",           // not a digit
		"cacheFlag=%zz",            // undecodable value
		"cacheFlag=%",              // truncated escape
		"cacheFlag=1&cacheFlag=1",  // repeated, values agree
		"cacheFlag=0&cacheFlag=1",  // repeated, values disagree
		"cacheFlag=0&cacheFlag=%zz", // repeated, second undecodable
	}

	for _, rawQuery := range rawQueries {
		t.Run(rawQuery, func(t *testing.T) {
			flag, err := parseCacheFlag(rawQuery)
			if !errors.Is(err, errInvalidCacheFlag) {
				t.Fatalf("parseCacheFlag(%q) returned error %v, want errInvalidCacheFlag", rawQuery, err)
			}
			if flag != cacheFlagFromCache {
				t.Errorf("parseCacheFlag(%q) = %d on rejection, want the safe default %d", rawQuery, flag, cacheFlagFromCache)
			}
		})
	}
}
