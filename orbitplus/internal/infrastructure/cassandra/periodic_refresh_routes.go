package cassandra

import (
	"context"
	"fmt"

	"github.com/gocql/gocql"

	"orbitplusmaster/internal/domain"
)

const periodicRefreshRoutesTable = "periodic_refresh_routes"

// PeriodicRefreshRoutesRepository reads routes scheduled for periodic refresh.
type PeriodicRefreshRoutesRepository struct {
	session *gocql.Session
}

// NewPeriodicRefreshRoutesRepository connects to the existing periodic_refresh_routes table.
func NewPeriodicRefreshRoutesRepository(ctx context.Context, config Config) (*PeriodicRefreshRoutesRepository, error) {
	if !validIdentifier(config.Keyspace) {
		return nil, fmt.Errorf("invalid Cassandra keyspace")
	}
	cluster := gocql.NewCluster(config.Hosts...)
	cluster.Port = config.Port
	cluster.Keyspace = config.Keyspace
	cluster.Timeout = config.Timeout
	cluster.ConnectTimeout = config.Timeout
	if config.Username != "" || config.Password != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{Username: config.Username, Password: config.Password}
	}
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("connect to Cassandra: %w", err)
	}
	return &PeriodicRefreshRoutesRepository{session: session}, nil
}

// ListActivePeriodicRefreshRoutes returns every active route without filtering by ticket count.
func (repository *PeriodicRefreshRoutesRepository) ListActivePeriodicRefreshRoutes(ctx context.Context) ([]domain.PeriodicRefreshRoute, error) {
	return repository.listPeriodicRefreshRoutes(ctx, true)
}

// ListPeriodicRefreshRoutes returns active and inactive routes by querying both
// Cassandra boolean partitions independently.
func (repository *PeriodicRefreshRoutesRepository) ListPeriodicRefreshRoutes(ctx context.Context) ([]domain.PeriodicRefreshRoute, error) {
	active, err := repository.listPeriodicRefreshRoutes(ctx, true)
	if err != nil {
		return nil, err
	}
	inactive, err := repository.listPeriodicRefreshRoutes(ctx, false)
	if err != nil {
		return nil, err
	}
	return append(active, inactive...), nil
}

// SavePeriodicRefreshRoute upserts a periodic refresh route into its active-state partition.
func (repository *PeriodicRefreshRoutesRepository) SavePeriodicRefreshRoute(ctx context.Context, route domain.PeriodicRefreshRoute) error {
	query := `INSERT INTO ` + periodicRefreshRoutesTable + `
		(is_active, operator_code, travel_date, from_station, to_station, ticket_count, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	if err := repository.session.Query(query, route.IsActive, route.OperatorCode, route.TravelDate,
		route.FromStation, route.ToStation, route.TicketCount, route.UpdatedAt).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("save periodic refresh route: %w", err)
	}
	return nil
}

func (repository *PeriodicRefreshRoutesRepository) listPeriodicRefreshRoutes(ctx context.Context, active bool) ([]domain.PeriodicRefreshRoute, error) {
	query := `SELECT operator_code, travel_date, from_station, to_station, ticket_count, is_active, updated_at
		FROM ` + periodicRefreshRoutesTable + ` WHERE is_active=?`
	iter := repository.session.Query(query, active).WithContext(ctx).Iter()
	routes := make([]domain.PeriodicRefreshRoute, 0)
	for {
		var route domain.PeriodicRefreshRoute
		if !iter.Scan(&route.OperatorCode, &route.TravelDate, &route.FromStation, &route.ToStation,
			&route.TicketCount, &route.IsActive, &route.UpdatedAt) {
			break
		}
		routes = append(routes, route)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("list periodic refresh routes active=%t: %w", active, err)
	}
	return routes, nil
}

// Close closes the Cassandra connection.
func (repository *PeriodicRefreshRoutesRepository) Close() {
	repository.session.Close()
}
