package models

type Bus struct {
	Code           string      `json:"code"`
	Name           string      `json:"name"`
	BusType        string      `json:"busType"`
	CategoryCode   string      `json:"categoryCode"`
	SeatLayoutList []BusLayout `json:"seatLayoutList,omitempty"`
}

type BusLayout struct {
	Code        string  `json:"code"`
	BusSeatType Base    `json:"busSeatType"`
	SeatStatus  Base    `json:"seatStatus"`
	RowPos      int     `json:"rowPos"`
	ColPos      int     `json:"colPos"`
	Layer       int     `json:"layer"`
	SeatName    string  `json:"seatName"`
	SeatFare    float64 `json:"seatFare"`
	ServiceTax  float64 `json:"serviceTax"`
}
