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
