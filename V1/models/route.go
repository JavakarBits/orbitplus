package models

type Route struct {
	FromStation Station `json:"fromStation"`
	ToStation   Station `json:"toStation"`
}
