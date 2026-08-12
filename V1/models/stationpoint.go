package models

type StationPoint struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	DateTime  string `json:"dateTime"`
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`
	Address   string `json:"address"`
	Landmark  string `json:"landmark"`
	Number    string `json:"number"`
}
