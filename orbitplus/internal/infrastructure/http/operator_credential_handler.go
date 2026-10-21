package http

import (
	"context"
	"log"
	"net/http"
	"strings"

	"orbitplusmaster/internal/domain"
)

// OperatorCredentialReader reads a stored operator credential by code.
// *cassandra.QueueMetrixRepository implements it.
type OperatorCredentialReader interface {
	FindOperatorCredential(ctx context.Context, operatorCode string) (domain.OperatorCredential, bool, error)
}

// OperatorCredentialHandler serves the worker credential ping: given an
// operator code, it returns the operator's stored BITS username/apiToken so the
// worker does not call Orbit. It is mounted behind InternalTokenAuth.
type OperatorCredentialHandler struct {
	reader OperatorCredentialReader
	logger *log.Logger
}

// NewOperatorCredentialHandler constructs the handler.
func NewOperatorCredentialHandler(reader OperatorCredentialReader) *OperatorCredentialHandler {
	return &OperatorCredentialHandler{reader: reader, logger: log.Default()}
}

type operatorCredentialResponse struct {
	OperatorCode string `json:"operatorCode"`
	ZoneCode     string `json:"zoneCode"`
	ZoneURL      string `json:"zoneUrl"`
	Username     string `json:"username"`
	APIToken     string `json:"apiToken"`
	ActiveFlag   int    `json:"activeFlag"`
}

// Serve returns one operator's stored credential as JSON.
func (handler *OperatorCredentialHandler) Serve(response http.ResponseWriter, request *http.Request) {
	if handler.reader == nil {
		writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Operator credential store is not configured")
		return
	}
	operatorCode := strings.TrimSpace(request.PathValue("operatorCode"))
	if operatorCode == "" {
		writeJSONStatus(response, http.StatusBadRequest, 0, "operatorCode is required")
		return
	}
	credential, found, err := handler.reader.FindOperatorCredential(request.Context(), operatorCode)
	if err != nil {
		handler.logger.Printf("operator credential lookup failed: operator_code=%q error=%v", operatorCode, err)
		writeJSONStatus(response, http.StatusInternalServerError, 0, "Unable to read operator credential")
		return
	}
	if !found {
		writeJSONStatus(response, http.StatusNotFound, 0, "Operator credential not found")
		return
	}
	// The credential's username/apiToken are secret: they are returned in the
	// body (the purpose of this endpoint) but never logged.
	writeJSONData(response, operatorCredentialResponse{
		OperatorCode: credential.OperatorCode,
		ZoneCode:     credential.ZoneCode,
		ZoneURL:      credential.ZoneURL,
		Username:     credential.Username,
		APIToken:     credential.APIToken,
		ActiveFlag:   credential.ActiveFlag,
	})
}
