package models

type Station struct {
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	DateTime     string         `json:"dateTime"`
	StationPoint []StationPoint `json:"stationPoint"`
}
