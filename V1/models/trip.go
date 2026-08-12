package models

type OrbitTrip struct {
	TripKey         string              `json:"tripkey"`
	ModifiedDate    string              `json:"modifiedDate"`
	TripDate        string              `json:"tripDate"`
	FromStationCode string              `json:"fromStationCode"`
	ToStationCode   string              `json:"toStationCode"`
	SearchResults   []OrbitSearchResult `json:"data"`
}

type OrbitTripBleve struct {
	TripKey         string                   `json:"tripkey"`
	ModifiedDate    string                   `json:"modifiedDate"`
	TripDate        string                   `json:"tripDate"`
	FromStationCode string                   `json:"fromStationCode"`
	ToStationCode   string                   `json:"toStationCode"`
	SearchResults   []OrbitSearchResultBleve `json:"data"`
}

type BitsTrip struct {
	TripKey         string             `json:"tripkey"`
	ModifiedDate    string             `json:"modifiedDate"`
	TripDate        string             `json:"tripDate"`
	FromStationCode string             `json:"fromStationCode"`
	ToStationCode   string             `json:"toStationCode"`
	SearchResults   []BitsSearchResult `json:"data"`
}

type BitsTripBleve struct {
	TripKey         string                  `json:"tripkey"`
	ModifiedDate    string                  `json:"modifiedDate"`
	TripDate        string                  `json:"tripDate"`
	FromStationCode string                  `json:"fromStationCode"`
	ToStationCode   string                  `json:"toStationCode"`
	SearchResults   []BitsSearchResultBleve `json:"data"`
}
