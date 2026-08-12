package models

type Base struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	ActiveFlag int    `json:"activeFlag"`
}
