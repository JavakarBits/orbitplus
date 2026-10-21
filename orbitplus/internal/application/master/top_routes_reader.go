package master

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

var (
	// ErrTopRoutesNotConfigured means no reader is available for the report.
	ErrTopRoutesNotConfigured = errors.New("top routes report is not configured")
	// ErrInvalidTopRoutesRange means the from/to date range is missing or invalid.
	ErrInvalidTopRoutesRange = errors.New("top routes date range is invalid")
)

const (
	// topRoutesDateLayout is the trip-date format stored in queue_metrix.
	topRoutesDateLayout = "2006-01-02"
	// topRoutesMaxRows bounds how many queue_metrix rows the scan examines, so
	// the report cannot accumulate an unbounded result set.
	topRoutesMaxRows = 50000
	// topRoutesPageSize is the driver paging size for the scan.
	topRoutesPageSize = 1000
	// topRoutesDefaultLimit bounds how many ranked routes are returned.
	topRoutesDefaultLimit = 100
)

// TopRouteRow is one aggregated route in the report: a unique from->to on a
// trip date, with how many queue_metrix records matched and some context.
type TopRouteRow struct {
	FromStation   string    `json:"fromStation"`
	ToStation     string    `json:"toStation"`
	TripDate      string    `json:"tripDate"`
	Count         int       `json:"count"`
	OperatorCount int       `json:"operatorCount"`
	Operators     []string  `json:"operators"`
	Zones         []string  `json:"zones"`
	LastUpdatedAt time.Time `json:"lastUpdatedAt"`
}

// TopRoutesReport is the full ranked result for a date range.
type TopRoutesReport struct {
	FromDate     string        `json:"fromDate"`
	ToDate       string        `json:"toDate"`
	Routes       []TopRouteRow `json:"routes"`
	RowsExamined int           `json:"rowsExamined"`
	Truncated    bool          `json:"truncated"`
	GeneratedAt  time.Time     `json:"generatedAt"`
}

// TopRoutesRouteRecord is the lightweight row the reader returns for
// aggregation — only the columns the report needs.
type TopRoutesRouteRecord struct {
	FromStation  string
	ToStation    string
	TripDate     string
	OperatorCode string
	Zone         string
	UpdatedAt    time.Time
}

// TopRoutesScanResult is one bounded scan over queue_metrix for the report.
type TopRoutesScanResult struct {
	Records      []TopRoutesRouteRecord
	RowsExamined int
	Truncated    bool
}

// TopRoutesReader scans queue_metrix for route records whose trip_date falls in
// the inclusive [fromDate, toDate] range. *cassandra.QueueMetrixRepository
// implements it.
type TopRoutesReader interface {
	ScanTripDateRange(ctx context.Context, fromDate, toDate string, maxRows, pageSize int) (TopRoutesScanResult, error)
}

// TopRoutesService builds the "top routes by count" report, grouping matched
// queue_metrix records by from-station -> to-station -> trip_date and ranking
// them by how many records each pair had in the date range.
type TopRoutesService struct {
	reader TopRoutesReader
	now    func() time.Time
}

// NewTopRoutesService constructs the report service. A nil reader disables it.
func NewTopRoutesService(reader TopRoutesReader) *TopRoutesService {
	return &TopRoutesService{reader: reader, now: time.Now}
}

// Report returns the ranked top routes whose trip_date is within [fromDate,
// toDate] (inclusive). Dates are "YYYY-MM-DD". limit <= 0 uses the default.
func (service *TopRoutesService) Report(ctx context.Context, fromDate, toDate string, limit int) (TopRoutesReport, error) {
	if service == nil || service.reader == nil {
		return TopRoutesReport{}, ErrTopRoutesNotConfigured
	}
	from := strings.TrimSpace(fromDate)
	to := strings.TrimSpace(toDate)
	fromParsed, errFrom := time.Parse(topRoutesDateLayout, from)
	toParsed, errTo := time.Parse(topRoutesDateLayout, to)
	if errFrom != nil || errTo != nil || toParsed.Before(fromParsed) {
		return TopRoutesReport{}, ErrInvalidTopRoutesRange
	}
	if limit <= 0 {
		limit = topRoutesDefaultLimit
	}

	scan, err := service.reader.ScanTripDateRange(ctx, from, to, topRoutesMaxRows, topRoutesPageSize)
	if err != nil {
		return TopRoutesReport{}, err
	}

	type aggregate struct {
		count     int
		operators map[string]struct{}
		zones     map[string]struct{}
		lastSeen  time.Time
	}
	groups := make(map[topRoutesKey]*aggregate)
	for _, record := range scan.Records {
		key := topRoutesKey{from: record.FromStation, to: record.ToStation, tripDate: record.TripDate}
		group := groups[key]
		if group == nil {
			group = &aggregate{operators: make(map[string]struct{}), zones: make(map[string]struct{})}
			groups[key] = group
		}
		group.count++
		if operator := strings.TrimSpace(record.OperatorCode); operator != "" {
			group.operators[operator] = struct{}{}
		}
		if zone := strings.TrimSpace(record.Zone); zone != "" {
			group.zones[zone] = struct{}{}
		}
		if record.UpdatedAt.After(group.lastSeen) {
			group.lastSeen = record.UpdatedAt
		}
	}

	routes := make([]TopRouteRow, 0, len(groups))
	for key, group := range groups {
		routes = append(routes, TopRouteRow{
			FromStation:   key.from,
			ToStation:     key.to,
			TripDate:      key.tripDate,
			Count:         group.count,
			OperatorCount: len(group.operators),
			Operators:     sortedKeys(group.operators),
			Zones:         sortedKeys(group.zones),
			LastUpdatedAt: group.lastSeen,
		})
	}
	// Rank by count (desc); break ties by trip date then route for stability.
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Count != routes[j].Count {
			return routes[i].Count > routes[j].Count
		}
		if routes[i].TripDate != routes[j].TripDate {
			return routes[i].TripDate < routes[j].TripDate
		}
		if routes[i].FromStation != routes[j].FromStation {
			return routes[i].FromStation < routes[j].FromStation
		}
		return routes[i].ToStation < routes[j].ToStation
	})
	if len(routes) > limit {
		routes = routes[:limit]
	}

	return TopRoutesReport{
		FromDate:     from,
		ToDate:       to,
		Routes:       routes,
		RowsExamined: scan.RowsExamined,
		Truncated:    scan.Truncated,
		GeneratedAt:  service.now().UTC(),
	}, nil
}

type topRoutesKey struct {
	from     string
	to       string
	tripDate string
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
