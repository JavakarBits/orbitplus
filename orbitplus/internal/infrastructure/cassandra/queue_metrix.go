package cassandra

import (
	"context"
	"fmt"

	"github.com/gocql/gocql"

	"orbitplusmaster/internal/domain"
)

const queueMetrixTable = "queue_metrix"

// QueueMetrixRepository persists queue job lifecycle records in Cassandra.
type QueueMetrixRepository struct {
	session *gocql.Session
}

// NewQueueMetrixRepository connects to Cassandra and ensures queue_metrix exists.
func NewQueueMetrixRepository(ctx context.Context, config Config) (*QueueMetrixRepository, error) {
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
	repository := &QueueMetrixRepository{session: session}
	if err := repository.ensureSchema(ctx); err != nil {
		session.Close()
		return nil, err
	}
	return repository, nil
}

func (repository *QueueMetrixRepository) ensureSchema(ctx context.Context) error {
	query := `CREATE TABLE IF NOT EXISTS ` + queueMetrixTable + ` (
		reference_id text PRIMARY KEY,
		activity_type text,
		action_type text,
		operator_code text,
		schedule_code text,
		trip_code text,
		from_station text,
		to_station text,
		travel_date text,
		zone text,
		queue_status text,
		queued_at timestamp,
		completed_at timestamp,
		dead_lettered_at timestamp,
		failure_message text,
		updated_at timestamp
	)`
	if err := repository.session.Query(query).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("create Cassandra queue metrix table: %w", err)
	}
	return nil
}

// SaveReceived creates the lifecycle record before Worker job publication.
func (repository *QueueMetrixRepository) SaveReceived(ctx context.Context, metric domain.QueueMetrix) error {
	query := `INSERT INTO ` + queueMetrixTable + `
		(reference_id, activity_type, action_type, operator_code, schedule_code, trip_code,
		from_station, to_station, travel_date, zone, queue_status,
		failure_message, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if err := repository.session.Query(query, metric.ReferenceID, metric.ActivityType, metric.ActionType,
		metric.OperatorCode, metric.ScheduleCode, metric.TripCode, metric.SourceStationCode,
		metric.DestinationStationCode, metric.TravelDate, metric.Zone, metric.QueueStatus,
		metric.FailureMessage, metric.UpdatedAt).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("save queue metrix received record: %w", err)
	}
	return nil
}

// MarkQueued records a broker-confirmed publication.
func (repository *QueueMetrixRepository) MarkQueued(ctx context.Context, metric domain.QueueMetrix) error {
	query := `UPDATE ` + queueMetrixTable + ` SET queue_status=?, trip_code=?, travel_date=?,
		queued_at=?, failure_message=?, updated_at=? WHERE reference_id=?`
	if err := repository.session.Query(query, metric.QueueStatus, metric.TripCode, metric.TravelDate,
		metric.QueuedAt, metric.FailureMessage, metric.UpdatedAt, metric.ReferenceID).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("mark queue metrix queued: %w", err)
	}
	return nil
}

// MarkCompleted records a successfully persisted Worker response.
func (repository *QueueMetrixRepository) MarkCompleted(ctx context.Context, metric domain.QueueMetrix) error {
	query := `UPDATE ` + queueMetrixTable + ` SET queue_status=?, completed_at=?,
		failure_message=?, updated_at=? WHERE reference_id=?`
	if err := repository.session.Query(query, metric.QueueStatus, metric.CompletedAt,
		metric.FailureMessage, metric.UpdatedAt, metric.ReferenceID).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("mark queue metrix completed: %w", err)
	}
	return nil
}

// MarkDead records a build, publish, or persistence failure.
func (repository *QueueMetrixRepository) MarkDead(ctx context.Context, metric domain.QueueMetrix) error {
	query := `UPDATE ` + queueMetrixTable + ` SET queue_status=?, dead_lettered_at=?,
		failure_message=?, updated_at=? WHERE reference_id=?`
	if err := repository.session.Query(query, metric.QueueStatus, metric.DeadLetteredAt,
		metric.FailureMessage, metric.UpdatedAt, metric.ReferenceID).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("mark queue metrix dead: %w", err)
	}
	return nil
}

// Close closes the Cassandra connection.
func (repository *QueueMetrixRepository) Close() {
	repository.session.Close()
}
