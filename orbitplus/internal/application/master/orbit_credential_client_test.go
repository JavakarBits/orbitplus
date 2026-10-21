package master

import "testing"

func TestNormalizeZoneHost(t *testing.T) {
	cases := []struct {
		name        string
		environment AppEnvironment
		input       string
		want        string
	}{
		{"bare host defaults to https in development", Development, "app.rathimeenatravels.in", "https://app.rathimeenatravels.in"},
		{"bare host defaults to https in production", Production, "app.rathimeenatravels.in", "https://app.rathimeenatravels.in"},
		{"existing http scheme is preserved", Production, "http://app.example.in", "http://app.example.in"},
		{"existing https scheme is preserved", Development, "https://app.example.in", "https://app.example.in"},
		{"trailing slash is trimmed", Development, "app.example.in/", "https://app.example.in"},
		{"surrounding whitespace is trimmed", Production, "  app.example.in  ", "https://app.example.in"},
		{"empty stays empty", Production, "", ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := &OrbitCredentialClient{environment: testCase.environment}
			if got := client.normalizeZoneHost(testCase.input); got != testCase.want {
				t.Fatalf("normalizeZoneHost(%q) [env=%s] = %q, want %q", testCase.input, testCase.environment, got, testCase.want)
			}
		})
	}
}
