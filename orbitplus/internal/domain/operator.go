package domain

// Operator identifies one Orbit operator eligible for route refresh.
type Operator struct {
	Code       string
	ZoneCode   string
	ActiveFlag int
}

// Active reports whether the operator can be selected for refresh work.
func (operator Operator) Active() bool {
	return operator.ActiveFlag == 1
}

// OperatorCredential is the BITS credential resolved from the Orbit credential
// service for one operator. It is credential material: never log Username or
// APIToken.
type OperatorCredential struct {
	OperatorCode string
	ZoneCode     string
	ZoneURL      string
	Username     string
	APIToken     string
	ActiveFlag   int
}
