package models

type OrbitSearchResult struct {
	TripCode           string  `json:"tripCode"`
	TripDate           string  `json:"tripDate"`
	ModifiedDate       string  `json:"modifiedDate"`
	AvailableSeatCount int     `json:"availableSeatCount"`
	JourneyMinutes     int     `json:"journeyMinutes"`
	Fares              []int   `json:"fares,omitempty"`
	Bus                Bus     `json:"bus"`
	FromStation        Station `json:"fromStation"`
	ToStation          Station `json:"toStation"`
	Operator           Base    `json:"operator"`
	TripStatus         Base    `json:"tripStatus"`
	Amenities          []Base  `json:"amenities"`
	Tax                Tax     `json:"tax"`
}

type OrbitSearchResultBleve struct {
	TripCode           string `json:"tripCode"`
	TripDate           string `json:"tripDate"`
	ModifiedDate       string `json:"modifiedDate"`
	AvailableSeatCount int    `json:"availableSeatCount"`
	JourneyMinutes     int    `json:"journeyMinutes"`
	Fares              string `json:"fares,omitempty"`
	Bus                string `json:"bus"`
	FromStation        string `json:"fromStation"`
	ToStation          string `json:"toStation"`
	Operator           string `json:"operator"`
	TripStatus         string `json:"tripStatus"`
	Amenities          string `json:"amenities"`
	Tax                string `json:"tax"`
}

type BitsSearchResultResponse struct {
	BitsSearchResult []BitsSearchResult `json:"data"`
}

type BitsSearchResult struct {
	TripCode         string           `json:"tripCode"`
	TripStageCode    string           `json:"tripStageCode"`
	TravelDate       string           `json:"travelDate"`
	DisplayName      string           `json:"displayName"`
	StageFare        []StageFare      `json:"stageFare,omitempty"`
	TravelTime       string           `json:"travelTime"`
	CloseTime        string           `json:"closeTime"`
	Bus              Bus              `json:"bus"`
	Schedule         Schedule         `json:"schedule"`
	FromStation      Station          `json:"fromStation"`
	ToStation        Station          `json:"toStation"`
	TripStatus       Base             `json:"tripStatus"`
	Operator         Operator         `json:"operator"`
	Amenities        []Base           `json:"amenities,omitempty"`
	Activities       []Base           `json:"activities,omitempty"`
	CancellationTerm CancellationTerm `json:"cancellationTerm,omitempty"`
}

type BitsSearchResultBleve struct {
	ModifiedDate     string `json:"modifiedDate"`
	TripCode         string `json:"tripCode"`
	TripStageCode    string `json:"tripStageCode"`
	TravelDate       string `json:"travelDate"`
	DisplayName      string `json:"displayName"`
	StageFare        string `json:"stageFare"`
	TravelTime       string `json:"travelTime"`
	CloseTime        string `json:"closeTime"`
	Bus              string `json:"bus"`
	Schedule         string `json:"schedule"`
	FromStation      string `json:"fromStation"`
	ToStation        string `json:"toStation"`
	TripStatus       string `json:"tripStatus"`
	Operator         string `json:"operator"`
	Amenities        string `json:"amenities"`
	Activities       string `json:"activities"`
	CancellationTerm string `json:"cancellationTerm"`
}

type StageFare struct {
	Fare               float64 `json:"fare"`
	AvailableSeatCount int     `json:"availableSeatCount"`
	SeatType           string  `json:"seatType"`
}

type Schedule struct {
	Code          string `json:"code"`
	ServiceNumber string `json:"serviceNumber"`
	Tax           Tax    `json:"tax"`
}

type CancellationTerm struct {
	Code               string               `json:"code"`
	Datetime           string               `json:"datetime"`
	CancellationPolicy []CancellationPolicy `json:"policyList,omitempty"`
}

type CancellationPolicy struct {
	Code            string  `json:"code"`
	FromValue       int     `json:"fromValue"`
	ToValue         int     `json:"toValue"`
	DeductionAmount float64 `json:"deductionAmount"`
	PolicyPattern   string  `json:"policyPattern"`
	PercentageFlag  int     `json:"percentageFlag"`
}
