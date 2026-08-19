package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	uiSessionCookie   = "orbitplus_ui_session"
	uiSessionDuration = 8 * time.Hour
)

// UIAccessAuth protects the OrbitPlus UI with an environment-configured token.
type UIAccessAuth struct {
	accessToken   []byte
	secureCookies bool
}

// NewUIAccessAuth constructs the UI access-token gate.
func NewUIAccessAuth(accessToken string, secureCookies bool) *UIAccessAuth {
	return &UIAccessAuth{accessToken: []byte(accessToken), secureCookies: secureCookies}
}

func (auth *UIAccessAuth) enabled() bool {
	return auth != nil && len(auth.accessToken) > 0
}

// Login serves the token form and establishes a signed session after validation.
func (auth *UIAccessAuth) Login(response http.ResponseWriter, request *http.Request) {
	if !auth.enabled() {
		http.Error(response, "OrbitPlus UI access is not configured", http.StatusServiceUnavailable)
		return
	}
	switch request.Method {
	case http.MethodGet:
		http.ServeFile(response, request, "ui/login.html")
	case http.MethodPost:
		auth.loginPost(response, request)
	default:
		response.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		http.Error(response, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (auth *UIAccessAuth) loginPost(response http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(response, request.Body, 4096)
	if err := request.ParseForm(); err != nil {
		http.Error(response, "Invalid access token", http.StatusBadRequest)
		return
	}
	suppliedToken := []byte(request.PostForm.Get("access_token"))
	if len(suppliedToken) != len(auth.accessToken) || subtle.ConstantTimeCompare(suppliedToken, auth.accessToken) != 1 {
		http.Error(response, "Invalid access token", http.StatusUnauthorized)
		return
	}
	expiresAt := time.Now().Add(uiSessionDuration)
	http.SetCookie(response, auth.sessionCookie(expiresAt))
	http.Redirect(response, request, "/orbitplus/", http.StatusSeeOther)
}

// Logout clears the UI session cookie.
func (auth *UIAccessAuth) Logout(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", http.MethodPost)
		http.Error(response, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.SetCookie(response, &http.Cookie{Name: uiSessionCookie, Path: "/orbitplus", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: auth.secureCookies})
	http.Redirect(response, request, "/orbitplus/login", http.StatusSeeOther)
}

// RequirePage redirects unauthenticated browser requests to the login page.
func (auth *UIAccessAuth) RequirePage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !auth.enabled() {
			http.Error(response, "OrbitPlus UI access is not configured", http.StatusServiceUnavailable)
			return
		}
		if !auth.authorized(request) {
			http.Redirect(response, request, "/orbitplus/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(response, request)
	})
}

// RequireAPI returns JSON errors for unauthenticated report API requests.
func (auth *UIAccessAuth) RequireAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !auth.enabled() {
			writeJSONStatus(response, http.StatusServiceUnavailable, 0, "OrbitPlus UI access is not configured")
			return
		}
		if !auth.authorized(request) {
			writeJSONStatus(response, http.StatusUnauthorized, 0, "Unauthorized")
			return
		}
		next.ServeHTTP(response, request)
	})
}

// RequireEnabled protects UI assets while permitting the login page to load them.
func (auth *UIAccessAuth) RequireEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !auth.enabled() {
			http.Error(response, "OrbitPlus UI access is not configured", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func (auth *UIAccessAuth) authorized(request *http.Request) bool {
	cookie, err := request.Cookie(uiSessionCookie)
	return err == nil && auth.validSession(cookie.Value)
}

func (auth *UIAccessAuth) sessionCookie(expiresAt time.Time) *http.Cookie {
	return &http.Cookie{Name: uiSessionCookie, Value: auth.sessionValue(expiresAt), Path: "/orbitplus", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: auth.secureCookies, Expires: expiresAt, MaxAge: int(uiSessionDuration.Seconds())}
}

func (auth *UIAccessAuth) sessionValue(expiresAt time.Time) string {
	expires := strconv.FormatInt(expiresAt.Unix(), 10)
	mac := hmac.New(sha256.New, auth.accessToken)
	_, _ = mac.Write([]byte(expires))
	return expires + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (auth *UIAccessAuth) validSession(value string) bool {
	expires, signature, found := strings.Cut(value, ".")
	if !found {
		return false
	}
	expiresUnix, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || time.Now().After(time.Unix(expiresUnix, 0)) {
		return false
	}
	providedSignature, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, auth.accessToken)
	_, _ = mac.Write([]byte(expires))
	return hmac.Equal(providedSignature, mac.Sum(nil))
}

func serveUIFile(path string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.ServeFile(response, request, path)
	})
}

func redirectTo(path string) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, path, http.StatusSeeOther)
	}
}
