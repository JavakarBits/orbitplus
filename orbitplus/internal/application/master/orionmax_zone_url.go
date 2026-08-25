package master

import "strings"

var zoneURLByCode = map[string]string{
	"bits":         "http://app.ezeebits.com",
	"r2bits":       "http://app.r2.ezeebits.com",
	"r3bits":       "http://app.r3.ezeebits.com",
	"ybmbits":      "http://app.ybmtravels.in",
	"sbltbits":     "http://app.sbltbus.com",
	"svrtbits":     "http://app.srivenkataramanatravels.co.in",
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
	return zoneURLByCode[zoneCode], true
}
