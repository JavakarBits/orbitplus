package models

type Operator struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	Username   string `json:"username"`
	ApiToken   string `json:"apiToken"`
	ActiveFlag int    `json:"activeFlag"`
}
