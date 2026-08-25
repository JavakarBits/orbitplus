package cassandra

import (
	"context"
	"errors"
	"fmt"

	"github.com/gocql/gocql"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/domain"
)

const queueMetrixTable = "queue_metrix"

// QueueMetrixRepository persists queue job lifecycle records in Cassandra.
type QueueMetrixRepository struct {
	session *gocql.Session
}

// NewQueueMetrixRepository connects to the existing queue_metrix table.
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
	return &QueueMetrixRepository{session: session}, nil
}

// SaveReceived creates the lifecycle record before Worker job publication.
func (repository *QueueMetrixRepository) SaveReceived(ctx context.Context, metric domain.QueueMetrix) error {
	query := `INSERT INTO ` + queueMetrixTable + `
		(reference_id, activity_type, action_type, operator_code, schedule_code, trip_code,
		from_station, to_station, travel_date, zone, message, queue_status,
		queued_at, completed_at, dead_lettered_at, failure_message, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if err := repository.session.Query(query, metric.ReferenceID, metric.ActivityType, metric.ActionType,
		metric.OperatorCode, metric.ScheduleCode, metric.TripCode, metric.SourceStationCode,
		metric.DestinationStationCode, metric.TripDate, metric.Zone, string(metric.WorkerPayload), metric.QueueStatus,
		nil, nil, nil, metric.FailureMessage, metric.UpdatedAt).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("save queue metrix received record: %w", err)
	}
	return nil
}

// FindByReferenceID returns one queue lifecycle record by its exact primary key.
func (repository *QueueMetrixRepository) FindByReferenceID(ctx context.Context, referenceID string) (domain.QueueMetrix, bool, error) {
	const query = `SELECT reference_id, activity_type, action_type, operator_code, schedule_code, trip_code,
		from_station, to_station, travel_date, zone, message, trip_codes, queue_status, queued_at, completed_at,
		dead_lettered_at, failure_message, updated_at FROM queue_metrix WHERE reference_id=?`
	var metric domain.QueueMetrix
	var message string
	err := repository.session.Query(query, referenceID).WithContext(ctx).Scan(&metric.ReferenceID, &metric.ActivityType, &metric.ActionType,
		&metric.OperatorCode, &metric.ScheduleCode, &metric.TripCode, &metric.SourceStationCode, &metric.DestinationStationCode,
		&metric.TripDate, &metric.Zone, &message, &metric.UpdatedTripCodes, &metric.QueueStatus, &metric.QueuedAt,
		&metric.CompletedAt, &metric.DeadLetteredAt, &metric.FailureMessage, &metric.UpdatedAt)
	if errors.Is(err, gocql.ErrNotFound) {
		return domain.QueueMetrix{}, false, nil
	}
	if err != nil {
		return domain.QueueMetrix{}, false, fmt.Errorf("find queue metrix record: %w", err)
	}
	metric.WorkerPayload = append(metric.WorkerPayload, message...)
	return metric, true, nil
}

// MarkQueued records a broker-confirmed publication.
func (repository *QueueMetrixRepository) MarkQueued(ctx context.Context, metric domain.QueueMetrix) error {
	query := `UPDATE ` + queueMetrixTable + ` SET queue_status=?, trip_code=?, travel_date=?,
		queued_at=?, failure_message=?, updated_at=? WHERE reference_id=?`
	if err := repository.session.Query(query, metric.QueueStatus, metric.TripCode, metric.TripDate,
		metric.QueuedAt, metric.FailureMessage, metric.UpdatedAt, metric.ReferenceID).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("mark queue metrix queued: %w", err)
	}
	return nil
}

// MarkCompleted records a successfully persisted Worker response.
func (repository *QueueMetrixRepository) MarkCompleted(ctx context.Context, metric domain.QueueMetrix) error {
	query := `UPDATE ` + queueMetrixTable + ` SET queue_status=?, completed_at=?, trip_codes=?,
		failure_message=?, updated_at=? WHERE reference_id=?`
	if err := repository.session.Query(query, metric.QueueStatus, metric.CompletedAt, metric.UpdatedTripCodes,
		metric.FailureMessage, metric.UpdatedAt, metric.ReferenceID).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("mark queue metrix completed: %w", err)
	}
	return nil
}

// MarkDead records a build, publish, or persistence failure.
func (repository *QueueMetrixRepository) MarkDead(ctx context.Context, metric domain.QueueMetrix) error {
	query := `UPDATE ` + queueMetrixTable + ` SET queue_status=?, dead_lettered_at=?,
		failure_message=?, updated_at=? WHERE reference_id=?`
	arguments := []any{metric.QueueStatus, metric.DeadLetteredAt, metric.FailureMessage, metric.UpdatedAt, metric.ReferenceID}
	if len(metric.UpdatedTripCodes) > 0 {
		query = `UPDATE ` + queueMetrixTable + ` SET queue_status=?, dead_lettered_at=?, trip_codes=?,
			failure_message=?, updated_at=? WHERE reference_id=?`
		arguments = []any{metric.QueueStatus, metric.DeadLetteredAt, metric.UpdatedTripCodes, metric.FailureMessage, metric.UpdatedAt, metric.ReferenceID}
	}
	if err := repository.session.Query(query, arguments...).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("mark queue metrix dead: %w", err)
	}
	return nil
}

// Close closes the Cassandra connection.
func (repository *QueueMetrixRepository) Close() {
	repository.session.Close()
}

// List returns a bounded page of queue lifecycle records for the report UI.
func (repository *QueueMetrixRepository) List(ctx context.Context, limit int) ([]domain.QueueMetrix, error) {
	query := `SELECT reference_id, activity_type, action_type, operator_code, schedule_code, trip_code,
		from_station, to_station, travel_date, zone, queue_status, queued_at, completed_at,
		dead_lettered_at, failure_message, updated_at FROM ` + queueMetrixTable + ` LIMIT ?`
	iter := repository.session.Query(query, limit).PageSize(limit).WithContext(ctx).Iter()
	jobs := make([]domain.QueueMetrix, 0, limit)
	for {
		var job domain.QueueMetrix
		if !iter.Scan(&job.ReferenceID, &job.ActivityType, &job.ActionType, &job.OperatorCode,
			&job.ScheduleCode, &job.TripCode, &job.SourceStationCode, &job.DestinationStationCode,
			&job.TripDate, &job.Zone, &job.QueueStatus, &job.QueuedAt, &job.CompletedAt,
			&job.DeadLetteredAt, &job.FailureMessage, &job.UpdatedAt) {
			break
		}
		jobs = append(jobs, job)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("list queue metrix records: %w", err)
	}
	return jobs, nil
}

// ScanTripHistory walks queue_metrix with driver paging and applies the trip or
// route match in Go. queue_metrix is keyed by reference_id only, so no
// server-side predicate exists for this lookup; ALLOW FILTERING is deliberately
// avoided because it lets one coordinator accumulate an unbounded result set.
// The scan reads at LOCAL_ONE and stops at the caller's row, match, or deadline
// limit, reporting how much work it performed.
func (repository *QueueMetrixRepository) ScanTripHistory(ctx context.Context, query master.TripHistoryQuery, options master.TripHistoryScanOptions) (master.TripHistoryScanResult, error) {
	statement := `SELECT reference_id, activity_type, action_type, operator_code, schedule_code, trip_code,
		from_station, to_station, travel_date, zone, message, trip_codes, queue_status, queued_at, completed_at,
		dead_lettered_at, failure_message, updated_at FROM ` + queueMetrixTable
	iter := repository.session.Query(statement).WithContext(ctx).Consistency(gocql.LocalOne).PageSize(options.PageSize).Iter()
	result := master.TripHistoryScanResult{Records: make([]domain.QueueMetrix, 0)}
	for {
		var record domain.QueueMetrix
		var message string
		if !iter.Scan(&record.ReferenceID, &record.ActivityType, &record.ActionType, &record.OperatorCode,
			&record.ScheduleCode, &record.TripCode, &record.SourceStationCode, &record.DestinationStationCode,
			&record.TripDate, &record.Zone, &message, &record.UpdatedTripCodes, &record.QueueStatus, &record.QueuedAt, &record.CompletedAt,
			&record.DeadLetteredAt, &record.FailureMessage, &record.UpdatedAt) {
			break
		}
		record.WorkerPayload = append(record.WorkerPayload, message...)
		result.RowsExamined++
		if query.Matches(record) {
			result.Records = append(result.Records, record)
			if len(result.Records) >= options.MaxMatches {
				result.Truncated = true
				break
			}
		}
		if result.RowsExamined >= options.MaxRowsExamined || ctx.Err() != nil {
			result.Truncated = true
			break
		}
	}
	if err := iter.Close(); err != nil {
		if ctx.Err() != nil {
			result.Truncated = true
			return result, nil
		}
		return master.TripHistoryScanResult{}, fmt.Errorf("scan queue metrix trip history: %w", err)
	}
	return result, nil
}
