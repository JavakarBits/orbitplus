package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"orbitplusmaster/internal/application/master"
)

// CacheSettingsHandler serves the protected admin Cache settings API: the
// runtime-editable Hot/Warm age thresholds and the IST daily cache window.
type CacheSettingsHandler struct {
	service *master.CacheSettingsService
	logger  *log.Logger
}

// NewCacheSettingsHandler constructs the handler.
func NewCacheSettingsHandler(service *master.CacheSettingsService) *CacheSettingsHandler {
	return &CacheSettingsHandler{service: service, logger: log.Default()}
}

// cacheSettingsPayload is the JSON shape exchanged with the UI. Ages are in
// hours (decimals allowed, e.g. 4 or 0.5); the window is HH:MM in IST.
type cacheSettingsPayload struct {
	HotMaxAgeHours  float64 `json:"hotMaxAgeHours"`
	WarmMaxAgeHours float64 `json:"warmMaxAgeHours"`
	WindowEnabled   bool    `json:"windowEnabled"`
	WindowStart     string  `json:"windowStart"`
	WindowEnd       string  `json:"windowEnd"`
	Timezone        string  `json:"timezone"`
	UpdatedAt       string  `json:"updatedAt,omitempty"`
}

// ServeHTTP returns the current settings (GET) or saves new settings (POST).
func (handler *CacheSettingsHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Cache settings are not configured")
		return
	}
	switch request.Method {
	case http.MethodGet:
		handler.serveGet(response, request)
	case http.MethodPost:
		handler.servePost(response, request)
	default:
		writeJSONStatus(response, http.StatusMethodNotAllowed, 0, "Method not allowed")
	}
}

func (handler *CacheSettingsHandler) serveGet(response http.ResponseWriter, request *http.Request) {
	settings, err := handler.service.Get(request.Context())
	if err != nil {
		handler.writeError(response, err)
		return
	}
	writeJSONData(response, toPayload(settings))
}

func (handler *CacheSettingsHandler) servePost(response http.ResponseWriter, request *http.Request) {
	var payload cacheSettingsPayload
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeJSONStatus(response, http.StatusBadRequest, 0, "Request body must be valid JSON")
		return
	}
	startMinute, err := parseClockMinutes(payload.WindowStart)
	if err != nil {
		writeJSONStatus(response, http.StatusBadRequest, 0, "windowStart must be HH:MM (00:00–23:59)")
		return
	}
	endMinute, err := parseClockMinutes(payload.WindowEnd)
	if err != nil {
		writeJSONStatus(response, http.StatusBadRequest, 0, "windowEnd must be HH:MM (00:00–23:59)")
		return
	}
	settings := master.CacheSettings{
		HotMaxAge:         time.Duration(payload.HotMaxAgeHours * float64(time.Hour)),
		WarmMaxAge:        time.Duration(payload.WarmMaxAgeHours * float64(time.Hour)),
		WindowEnabled:     payload.WindowEnabled,
		WindowStartMinute: startMinute,
		WindowEndMinute:   endMinute,
	}
	saved, err := handler.service.Save(request.Context(), settings)
	if err != nil {
		handler.writeError(response, err)
		return
	}
	writeJSONData(response, toPayload(saved))
}

func (handler *CacheSettingsHandler) writeError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, master.ErrCacheSettingsNotConfigured):
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Cache settings are not configured")
	case errors.Is(err, master.ErrInvalidCacheSettings):
		writeJSONStatus(response, http.StatusBadRequest, 0, "Hot and Warm ages must be positive with Hot ≤ Warm, and window times valid")
	default:
		handler.logger.Printf("cache settings request failed: %v", err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to process cache settings")
	}
}

func toPayload(settings master.CacheSettings) cacheSettingsPayload {
	payload := cacheSettingsPayload{
		HotMaxAgeHours:  settings.HotMaxAge.Hours(),
		WarmMaxAgeHours: settings.WarmMaxAge.Hours(),
		WindowEnabled:   settings.WindowEnabled,
		WindowStart:     formatClockMinutes(settings.WindowStartMinute),
		WindowEnd:       formatClockMinutes(settings.WindowEndMinute),
		Timezone:        "Asia/Kolkata (IST)",
	}
	if !settings.UpdatedAt.IsZero() {
		payload.UpdatedAt = settings.UpdatedAt.Format(time.RFC3339)
	}
	return payload
}

// parseClockMinutes converts "HH:MM" to minutes-from-midnight. An empty value
// is treated as 00:00 so a disabled window still submits cleanly.
func parseClockMinutes(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	var hour, minute int
	if _, err := fmt.Sscanf(value, "%d:%d", &hour, &minute); err != nil {
		return 0, err
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("out of range")
	}
	return hour*60 + minute, nil
}

func formatClockMinutes(minutes int) string {
	if minutes < 0 || minutes > 1439 {
		minutes = 0
	}
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}
