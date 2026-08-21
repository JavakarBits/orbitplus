package cassandra

import (
	"context"
	"fmt"

	"github.com/gocql/gocql"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/domain"
)

const cacheFreshnessDifferenceTable = "cache_freshness_difference"

// CacheFreshnessDifferenceRepository persists cache-versus-Bits differences.
//
// Every statement in this file restricts the partition key. A SELECT without a
// WHERE clause would be an unbounded multi-partition scan returning rows in
// token order, which is what makes the queue_metrix report show arbitrary rows
// rather than recent ones. That pattern is deliberately not repeated here.
type CacheFreshnessDifferenceRepository struct {
	session *gocql.Session
}

// NewCacheFreshnessDifferenceRepository connects to the existing
// cache_freshness_difference table. The table is created by
// scripts/create_cassandra_tables.cql, not at startup.
func NewCacheFreshnessDifferenceRepository(ctx context.Context, config Config) (*CacheFreshnessDifferenceRepository, error) {
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
	return &CacheFreshnessDifferenceRepository{session: session}, nil
}

// SaveDifference inserts one difference row.
func (repository *CacheFreshnessDifferenceRepository) SaveDifference(ctx context.Context, record domain.RecordedDifference) error {
	differenceID, err := differenceUUID(record.DifferenceID)
	if err != nil {
		return err
	}
	query := `INSERT INTO ` + cacheFreshnessDifferenceTable + `
		(operator_code, detected_on, detected_at, difference_id, action_type, trip_code,
		trip_stage_code, from_code, to_code, trip_date, verification_outcome,
		difference_count, difference_paths, cache_repaired)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if err := repository.session.Query(query,
		record.OperatorCode, record.DetectedOn, record.DetectedAt, differenceID,
		record.ActionType, record.TripCode, record.TripStageCode, record.FromCode,
		record.ToCode, record.TripDate, string(record.VerificationOutcome),
		record.DifferenceCount, record.DifferencePaths, record.CacheRepaired,
	).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("save cache freshness difference: %w", err)
	}
	return nil
}

// ListDifferences reads exactly one partition, newest first. Ordering comes from
// the table's clustering order, so no sort is needed in application code.
func (repository *CacheFreshnessDifferenceRepository) ListDifferences(ctx context.Context, operatorCode, detectedOn string, limit int) ([]domain.RecordedDifference, error) {
	query := `SELECT difference_id, detected_at, action_type, trip_code, trip_stage_code,
		from_code, to_code, trip_date, verification_outcome, difference_count,
		difference_paths, cache_repaired
		FROM ` + cacheFreshnessDifferenceTable + ` WHERE operator_code=? AND detected_on=? LIMIT ?`
	iter := repository.session.Query(query, operatorCode, detectedOn, limit).WithContext(ctx).Iter()

	records := make([]domain.RecordedDifference, 0, limit)
	for {
		record := domain.RecordedDifference{OperatorCode: operatorCode, DetectedOn: detectedOn}
		var differenceID gocql.UUID
		var outcome string
		if !iter.Scan(&differenceID, &record.DetectedAt, &record.ActionType, &record.TripCode,
			&record.TripStageCode, &record.FromCode, &record.ToCode, &record.TripDate,
			&outcome, &record.DifferenceCount, &record.DifferencePaths, &record.CacheRepaired) {
			break
		}
		record.DifferenceID = differenceID.String()
		record.VerificationOutcome = domain.VerificationOutcome(outcome)
		records = append(records, record)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("list cache freshness differences: %w", err)
	}
	return records, nil
}

// Close closes the Cassandra connection.
func (repository *CacheFreshnessDifferenceRepository) Close() {
	repository.session.Close()
}

// differenceUUID accepts a caller-supplied identifier and generates one when
// absent, so the domain type stays a string and no driver type leaks inward.
func differenceUUID(value string) (gocql.UUID, error) {
	if value == "" {
		return gocql.TimeUUID(), nil
	}
	parsed, err := gocql.ParseUUID(value)
	if err != nil {
		return gocql.UUID{}, fmt.Errorf("invalid difference identifier: %w", err)
	}
	return parsed, nil
}

var _ master.CacheDifferenceWriter = (*CacheFreshnessDifferenceRepository)(nil)
