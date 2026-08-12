package models

type BitsBusMapResponse struct {
	BusMap BitsBusMap `json:"data"`
}
type BitsBusMap struct {
	TripCode         string           `json:"tripCode"`
	TripStageCode    string           `json:"tripStageCode"`
	TravelDate       string           `json:"travelDate"`
	TripStatus       Base             `json:"tripStatus"`
	Bus              Bus              `json:"bus"`
	Schedule         Schedule         `json:"schedule"`
	FromStation      Station          `json:"fromStation"`
	ToStation        Station          `json:"toStation"`
	Operator         Operator         `json:"operator"`
	CancellationTerm CancellationTerm `json:"cancellationTerm"`
}

type BitsBusMapBleve struct {
	Key              string `json:"key"`
	ModifiedDate     string `json:"modifiedDate"`
	TripCode         string `json:"tripCode"`
	FromStationCode  string `json:"fromStationCode"`
	ToStationCode    string `json:"toStationCode"`
	OperatorCode     string `json:"operatorCode"`
	TripStageCode    string `json:"tripStageCode"`
	TravelDate       string `json:"travelDate"`
	TripStatus       string `json:"tripStatus"`
	Bus              string `json:"bus"`
	Schedule         string `json:"schedule"`
	FromStation      string `json:"fromStation"`
	ToStation        string `json:"toStation"`
	Operator         string `json:"operator"`
	CancellationTerm string `json:"cancellationTerm"`
}
