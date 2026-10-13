package http

import (
	"errors"
	"log"
	"net/http"

	"orbitplusmaster/internal/application/master"
)

// LogsHandler serves protected, read-only application log file APIs.
type LogsHandler struct {
	service *master.LogsReadService
	logger  *log.Logger
}

// NewLogsHandler constructs a Logs API handler.
func NewLogsHandler(service *master.LogsReadService) *LogsHandler {
	return &LogsHandler{service: service, logger: log.Default()}
}

// ServeHTTP returns the list of available log files, newest first.
func (handler *LogsHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Log access is not configured")
		return
	}
	files, err := handler.service.List(request.Context())
	if err != nil {
		handler.writeError(response, err)
		return
	}
	items := make([]logFileItem, 0, len(files))
	for _, file := range files {
		items = append(items, logFileItem{Source: file.Source, Name: file.Name, SizeBytes: file.SizeBytes, ModifiedUTC: file.ModifiedUTC.Format("2006-01-02 15:04:05")})
	}
	writeJSONData(response, logFilesResponse{Files: items})
}

// ServeContent returns the (possibly truncated) tail of a single log file.
func (handler *LogsHandler) ServeContent(response http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Log access is not configured")
		return
	}
	content, err := handler.service.Read(request.Context(), request.URL.Query().Get("source"), request.URL.Query().Get("name"))
	if err != nil {
		handler.writeError(response, err)
		return
	}
	writeJSONData(response, logContentResponse{Source: content.Source, Name: content.Name, SizeBytes: content.SizeBytes, Content: content.Content, Truncated: content.Truncated})
}

type logFileItem struct {
	Source      string `json:"source"`
	Name        string `json:"name"`
	SizeBytes   int64  `json:"sizeBytes"`
	ModifiedUTC string `json:"modifiedUtc"`
}
type logFilesResponse struct {
	Files []logFileItem `json:"files"`
}
type logContentResponse struct {
	Source    string `json:"source"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

func (handler *LogsHandler) writeError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, master.ErrLogsNotConfigured):
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Log access is not configured")
	case errors.Is(err, master.ErrInvalidLogFile):
		writeJSONStatus(response, http.StatusBadRequest, 0, "Log file request is invalid")
	case errors.Is(err, master.ErrLogFileNotFound):
		writeJSONStatus(response, http.StatusNotFound, 0, "Log file not found")
	default:
		handler.logger.Printf("Logs viewer read failed: %v", err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to load log data")
	}
}
