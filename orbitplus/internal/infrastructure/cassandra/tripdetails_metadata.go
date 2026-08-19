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
	metadataRouteTable    = "trip_details_metadata_by_route"
	metadataScheduleTable = "trip_details_metadata_by_schedule"
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
	scheduleQuery := `CREATE TABLE IF NOT EXISTS ` + metadataScheduleTable + ` (
		operator_code text,
		schedule_code text,
		travel_date text,
		trip_code text,
		trip_stage_code text,
		updated_at timestamp,
		PRIMARY KEY ((operator_code, schedule_code, travel_date), trip_code, trip_stage_code)
	)`
	if err := repository.session.Query(scheduleQuery).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("create Cassandra schedule metadata table: %w", err)
	}
	return nil
}

// SaveRouteMetadata upserts one route-to-trip/stage lookup row.
func (repository *TripDetailsMetadataRepository) SaveRouteMetadata(ctx context.Context, metadata domain.TripDetailsStageMetadata) error {
	routeQuery := `INSERT INTO ` + metadataRouteTable + `
		(operator_code, travel_date, from_station_code, to_station_code, trip_code, trip_stage_code, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	if err := repository.session.Query(routeQuery, metadata.OperatorCode, metadata.TravelDate, metadata.FromStationCode,
		metadata.ToStationCode, metadata.TripCode, metadata.TripStageCode, metadata.UpdatedAt).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("save Cassandra route metadata: %w", err)
	}
	return nil
}

// SaveScheduleMetadata upserts one schedule-to-trip/stage lookup row.
func (repository *TripDetailsMetadataRepository) SaveScheduleMetadata(ctx context.Context, metadata domain.TripDetailsStageMetadata) error {
	scheduleQuery := `INSERT INTO ` + metadataScheduleTable + `
		(operator_code, schedule_code, travel_date, trip_code, trip_stage_code, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`
	if err := repository.session.Query(scheduleQuery, metadata.OperatorCode, metadata.ScheduleCode, metadata.TravelDate,
		metadata.TripCode, metadata.TripStageCode, metadata.UpdatedAt).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("save Cassandra schedule metadata: %w", err)
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

// FindStagesBySchedule returns candidate stages for one operator schedule and date.
func (repository *TripDetailsMetadataRepository) FindStagesBySchedule(ctx context.Context, operatorCode, scheduleCode, travelDate string) ([]domain.TripDetailsStageMetadata, error) {
	query := `SELECT trip_code, trip_stage_code, updated_at FROM ` + metadataScheduleTable + `
		WHERE operator_code=? AND schedule_code=? AND travel_date=?`
	iter := repository.session.Query(query, operatorCode, scheduleCode, travelDate).WithContext(ctx).Iter()
	var results []domain.TripDetailsStageMetadata
	for {
		var result domain.TripDetailsStageMetadata
		if !iter.Scan(&result.TripCode, &result.TripStageCode, &result.UpdatedAt) {
			break
		}
		result.OperatorCode = operatorCode
		result.ScheduleCode = scheduleCode
		result.TravelDate = travelDate
		results = append(results, result)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("find Cassandra schedule metadata: %w", err)
	}
	return results, nil
}
