package domain

import "encoding/json"

// TripDetailsResponse is a light top-level model for the raw Bits TripDetails
// response. The "data" payload is intentionally kept as json.RawMessage so
// that no fields are dropped or reshaped in this phase.
type TripDetailsResponse struct {
	Status   int             `json:"status"`
	Datetime string          `json:"datetime"`
	Data     json.RawMessage `json:"data"`
}
