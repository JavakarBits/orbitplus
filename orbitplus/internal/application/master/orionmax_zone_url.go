package master

import (
	"os"
	"strings"
)

var zoneURLByCode = map[string]string{
	"bits":         "http://app.ezeebits.com",
	"r2bits":       "http://app.r2.ezeebits.com",
	"r3bits":       "http://app.r3.ezeebits.com",
	"ybmbits":      "http://app.ybmtravels.in",
	"sbltbits":     "http://app.sbltbus.com",
	"svrtbits":     "http://app.srivenkataramanatravels.co.in",
	"parveenbits":  "http://app.parveentravels.in",
	"rmtbits":      "http://app.rathimeenatravels.in",
	"gotourbits":   "http://app.gotourtravels.com",
	"vinayagabits": "http://app.vinayagaselvamtravels.in",
	"vkvbits":      "http://app.vkvtravels.com",
	"prmbits":      "http://app.prmbus.com",
	"sbmbits":      "http://app.sbmbus.com",
}

func NormalizeZoneCode(zoneCode string) (string, bool) {
	zoneCode = strings.ToLower(strings.TrimSpace(zoneCode))
	_, exists := zoneURLByCode[zoneCode]
	return zoneCode, exists
}

func zoneURLFor(zoneCode string) (string, bool) {
	zoneCode, exists := NormalizeZoneCode(zoneCode)
	if !exists {
		return "", false
	}
	// Per-zone override takes precedence, e.g. BITS_ZONE_BITS_URL=https://dev-stage.ezeebits.in
	if override := strings.TrimSpace(os.Getenv("BITS_ZONE_" + strings.ToUpper(zoneCode) + "_URL")); override != "" {
		return strings.TrimRight(override, "/"), true
	}
	// Global override points every zone at one BITS environment (e.g. dev-stage),
	// so worker refresh jobs, live reads, and the Java block/confirm path all hit
	// the same host and operate on the same trip data.
	if override := strings.TrimSpace(os.Getenv("BITS_ZONE_DEFAULT_URL")); override != "" {
		return strings.TrimRight(override, "/"), true
	}
	return zoneURLByCode[zoneCode], true
}

// ZoneBitsBaseURL resolves the Bits endpoint a zone code addresses.
//
// A live read names its zone rather than inheriting one endpoint from
// configuration, because operators are spread across zones and a single
// configured host would silently query the wrong one. This is the same table
// the Orionmax inventory path uses, so both agree on where a zone lives.
func ZoneBitsBaseURL(zoneCode string) (string, bool) {
	return zoneURLFor(zoneCode)
}
