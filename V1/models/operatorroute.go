package models

type OrbitOperatorRouteResponse struct {
	OperatorRoute []OperatorRoute `json:"data"`
}
type OperatorRoute struct {
	Operator  Operator `json:"operator"`
	Routes    []Route  `json:"routes"`
	RouteKeys []string `json:"routeKeys"`
}

type OperatorRouteBleve struct {
	OperatorCode string   `json:"operatorCode"`
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	RouteKeys    []string `json:"routeKeys"`
	ModifiedDate string   `json:"modifiedDate"`
	Operator     string   `json:"operator"`
	Routes       string   `json:"routes"`
}
