package worker

import (
	"context"
	"fmt"
	"strings"

	"orbitplusworker/internal/domain"
)

// BitsOperatorCredential is transient runtime source authentication material.
type BitsOperatorCredential struct {
	OperatorCode string
	Username     string
	APIToken     string
	BaseURL      string
}

func (credential BitsOperatorCredential) Validate(operatorCode string) error {
	if credential.OperatorCode != operatorCode || strings.TrimSpace(credential.Username) == "" || strings.TrimSpace(credential.APIToken) == "" || strings.TrimSpace(credential.BaseURL) == "" {
		return fmt.Errorf("resolved Bits credential is invalid")
	}
	return nil
}

// BitsTripDetailsRequest contains the action-specific Bits request built from a
// transient credential.
type BitsTripDetailsRequest struct {
	Message    domain.TripDetailsRefreshMessage
	Credential BitsOperatorCredential
}

// BitsTripDetailsResponse is the raw JSON response returned by Bits.
type BitsTripDetailsResponse struct {
	Body []byte
}

// TripDetailsClient retrieves current TripDetails from Bits.
type TripDetailsClient interface {
	FetchTripDetails(ctx context.Context, request BitsTripDetailsRequest) (BitsTripDetailsResponse, error)
}
