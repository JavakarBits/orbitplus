package http

import (
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/infrastructure/bits"
)

// A cacheFlag=0 read must reach Bits as the operator named in the incoming URL.
// This walks the whole chain, route pattern through outbound request, because
// the three values are read in the handler and consumed in the adapter, and a
// unit test on either half alone would not notice them being dropped between.
func TestLiveReadForwardsRequestCredentialsToBits(t *testing.T) {
	testCases := []struct {
		name         string
		routePattern string
		requestPath  string
		expectedPath string
	}{
		{
			name:         "search",
			routePattern: "GET /orbitplus/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/{fromCode}/{toCode}/{tripDate}",
			requestPath:  "/orbitplus/api/3.0/json/OP9/user9/token9/search/CITY_A/CITY_B/2026-08-20?cacheFlag=0",
			expectedPath: "/busservices/api/3.0/json/OP9/user9/token9/search/CITY_A/CITY_B/2026-08-20",
		},
		{
			name:         "busmap",
			routePattern: "GET /orbitplus/api/3.0/json/{operatorCode}/{username}/{apiToken}/busmap/{tripCode}/{fromStationCode}/{toStationCode}/{travelDate}",
			requestPath:  "/orbitplus/api/3.0/json/OP7/user7/token7/busmap/TRIP_Z/STN_A/STN_B/2026-08-22?cacheFlag=0",
			expectedPath: "/busservices/api/3.0/json/OP7/user7/token7/busmap/TRIP_Z/STN_A/STN_B/2026-08-22",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var receivedPath string
			bitsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				_, _ = w.Write([]byte(`{"status":1,"data":[{"tripCode":"T1"}]}`))
			}))
			defer bitsServer.Close()

			handler := newLiveOnlyReadHandler(t, bitsServer)
			mux := http.NewServeMux()
			if testCase.name == "search" {
				mux.HandleFunc(testCase.routePattern, handler.ServeSearch)
			} else {
				mux.HandleFunc(testCase.routePattern, handler.ServeBusMap)
			}

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testCase.requestPath, nil))

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200. body: %s", recorder.Code, recorder.Body.String())
			}
			if receivedPath != testCase.expectedPath {
				t.Errorf("outbound Bits path:\n  got:  %s\n  want: %s", receivedPath, testCase.expectedPath)
			}
		})
	}
}

// A read whose credentials are absent must not reach Bits at all.
func TestLiveReadWithoutCredentialsIssuesNoBitsRequest(t *testing.T) {
	bitsServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no outbound request should be issued when the URL carries no credentials")
	}))
	defer bitsServer.Close()

	handler := newLiveOnlyReadHandler(t, bitsServer)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /orbitplus/api/3.0/json/{operatorCode}/search/{fromCode}/{toCode}/{tripDate}", handler.ServeSearch)

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"/orbitplus/api/3.0/json/OP9/search/CITY_A/CITY_B/2026-08-20?cacheFlag=0", nil))

	if recorder.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", recorder.Code)
	}
}

// newLiveOnlyReadHandler builds a handler wired for the live path only. A nil
// read service, difference writer, and repairer keep the test to one concern:
// which credentials leave the process.
func newLiveOnlyReadHandler(t *testing.T, bitsServer *httptest.Server) *TripDetailsReadHandler {
	t.Helper()
	fetcher, err := bits.NewBitsTripDetailsClient(bitsServer.Client(), master.VerificationConfig{
		BitsBaseURL:   bitsServer.URL,
		HTTPTimeout:   5 * time.Second,
		MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatalf("NewBitsTripDetailsClient: %v", err)
	}
	verifier, err := master.NewCacheFreshnessVerifier(fetcher, nil, nil, nil, 1, log.Default())
	if err != nil {
		t.Fatalf("NewCacheFreshnessVerifier: %v", err)
	}
	return NewTripDetailsReadHandler(nil, verifier)
}

// A Bits refusal must not surface as 502. Bits is reachable and answered, so a
// gateway error tells the caller to retry something that cannot succeed and
// points an operator at an outage that is not happening.
func TestLiveReadMapsBitsRejectionToNotFound(t *testing.T) {
	testCases := []struct {
		name string
		body string
		want int
	}{
		{
			name: "expired travel date",
			body: `{"status":0,"errorCode":"309A","errorDesc":"Expired travel date"}`,
			want: http.StatusNotFound,
		},
		{
			name: "route not found",
			body: `{"status":0,"errorCode":"318","errorDesc":"route not found","data":"65 days"}`,
			want: http.StatusNotFound,
		},
		{
			name: "genuine unavailability still reports a gateway error",
			body: `{"status":1,"datetime":"x"}`,
			want: http.StatusBadGateway,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			bitsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(testCase.body))
			}))
			defer bitsServer.Close()

			mux := http.NewServeMux()
			mux.HandleFunc("GET /orbitplus/api/3.0/json/{operatorCode}/{username}/{apiToken}/search/{fromCode}/{toCode}/{tripDate}",
				newLiveOnlyReadHandler(t, bitsServer).ServeSearch)

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
				"/orbitplus/api/3.0/json/noonetravels/user/token/search/STF17D52/STF1GI77/2026-08-21?cacheFlag=0", nil))

			if recorder.Code != testCase.want {
				t.Errorf("status = %d, want %d. body: %s", recorder.Code, testCase.want, recorder.Body.String())
			}
			// Neither case may echo upstream wording to the caller.
			for _, upstream := range []string{"errorCode", "309A", "318", "route not found", "65 days"} {
				if strings.Contains(recorder.Body.String(), upstream) {
					t.Errorf("response body carries upstream content %q: %s", upstream, recorder.Body.String())
				}
			}
		})
	}
}
