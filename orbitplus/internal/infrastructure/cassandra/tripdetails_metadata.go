package cassandra

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gocql/gocql"

	"orbitplusmaster/internal/domain"
)

const (
	metadataTable      = "trip_details_metadata_by_stage"
	metadataRouteTable = "trip_details_metadata_by_route"
)

// Config contains the Cassandra settings supplied by runtime config.
type Config struct {
	Hosts    []string
	Port     int
	Keyspace string
	Username string
	Password string
	Timeout  time.Duration
}

// TripDetailsMetadataRepository persists only TripDetails lookup metadata.
type TripDetailsMetadataRepository struct {
	session *gocql.Session
}

func NewTripDetailsMetadataRepository(ctx context.Context, config Config) (*TripDetailsMetadataRepository, error) {
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
	repository := &TripDetailsMetadataRepository{session: session}
	if err := repository.ensureSchema(ctx); err != nil {
		session.Close()
		return nil, err
	}
	return repository, nil
}

func (repository *TripDetailsMetadataRepository) ensureSchema(ctx context.Context) error {
	query := `CREATE TABLE IF NOT EXISTS ` + metadataTable + ` (
		operator_code text,
		trip_code text,
		travel_date text,
		from_station_code text,
		to_station_code text,
		trip_stage_code text,
		updated_at timestamp,
		PRIMARY KEY ((operator_code, trip_code, travel_date, from_station_code, to_station_code), trip_stage_code)
	)`
	if err := repository.session.Query(query).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("create Cassandra metadata table: %w", err)
	}
	routeQuery := `CREATE TABLE IF NOT EXISTS ` + metadataRouteTable + ` (
		operator_code text,
		travel_date text,
		from_station_code text,
		to_station_code text,
		trip_code text,
		trip_stage_code text,
		updated_at timestamp,
		PRIMARY KEY ((operator_code, travel_date, from_station_code, to_station_code), trip_code, trip_stage_code)
	)`
	if err := repository.session.Query(routeQuery).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("create Cassandra route metadata table: %w", err)
	}
	return nil
}

// SaveStageMetadata upserts one Stage lookup row. Its primary key permits many
// stages for the same route/date while preventing duplicate stage rows.
func (repository *TripDetailsMetadataRepository) SaveStageMetadata(ctx context.Context, metadata domain.TripDetailsStageMetadata) error {
	query := `INSERT INTO ` + metadataTable + `
		(operator_code, trip_code, travel_date, from_station_code, to_station_code, trip_stage_code, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	if err := repository.session.Query(query, metadata.OperatorCode, metadata.TripCode, metadata.TravelDate,
		metadata.FromStationCode, metadata.ToStationCode, metadata.TripStageCode, metadata.UpdatedAt).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("save Cassandra stage metadata: %w", err)
	}
	routeQuery := `INSERT INTO ` + metadataRouteTable + `
		(operator_code, travel_date, from_station_code, to_station_code, trip_code, trip_stage_code, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	if err := repository.session.Query(routeQuery, metadata.OperatorCode, metadata.TravelDate, metadata.FromStationCode,
		metadata.ToStationCode, metadata.TripCode, metadata.TripStageCode, metadata.UpdatedAt).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("save Cassandra route metadata: %w", err)
	}
	return nil
}

func (repository *TripDetailsMetadataRepository) Close() {
	repository.session.Close()
}

func validIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if !(character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9') {
			return false
		}
	}
	return !strings.Contains(value, "__")
}

// FindStagesByRoute returns candidate stages for a Search route in metadata order.
func (repository *TripDetailsMetadataRepository) FindStagesByRoute(ctx context.Context, operatorCode, fromCode, toCode, travelDate string) ([]domain.TripDetailsStageMetadata, error) {
	query := `SELECT trip_code, trip_stage_code, updated_at FROM ` + metadataRouteTable + `
		WHERE operator_code=? AND travel_date=? AND from_station_code=? AND to_station_code=?`
	iter := repository.session.Query(query, operatorCode, travelDate, fromCode, toCode).WithContext(ctx).Iter()
	var results []domain.TripDetailsStageMetadata
	for {
		var result domain.TripDetailsStageMetadata
		if !iter.Scan(&result.TripCode, &result.TripStageCode, &result.UpdatedAt) {
			break
		}
		result.OperatorCode = operatorCode
		result.FromStationCode = fromCode
		result.ToStationCode = toCode
		result.TravelDate = travelDate
		results = append(results, result)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("find Cassandra route metadata: %w", err)
	}
	return results, nil
}

// FindStagesByTripRoute returns candidate stages for a Busmap route and trip.
func (repository *TripDetailsMetadataRepository) FindStagesByTripRoute(ctx context.Context, operatorCode, tripCode, fromCode, toCode, travelDate string) ([]domain.TripDetailsStageMetadata, error) {
	query := `SELECT trip_stage_code, updated_at FROM ` + metadataTable + `
		WHERE operator_code=? AND trip_code=? AND travel_date=? AND from_station_code=? AND to_station_code=?`
	iter := repository.session.Query(query, operatorCode, tripCode, travelDate, fromCode, toCode).WithContext(ctx).Iter()
	var results []domain.TripDetailsStageMetadata
	for {
		var result domain.TripDetailsStageMetadata
		if !iter.Scan(&result.TripStageCode, &result.UpdatedAt) {
			break
		}
		result.OperatorCode, result.TripCode, result.FromStationCode, result.ToStationCode, result.TravelDate = operatorCode, tripCode, fromCode, toCode, travelDate
		results = append(results, result)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("find Cassandra stage metadata: %w", err)
	}
	return results, nil
}
