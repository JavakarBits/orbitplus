package master

import (
	"context"
	"errors"
	"sort"

	"orbitplusmaster/internal/domain"
)

const queueJobsReportLimit = 100

// QueueJobsService provides recent queue lifecycle records for the report UI.
type QueueJobsService struct {
	reader QueueMetrixReader
}

// NewQueueJobsService constructs the queue report reader.
func NewQueueJobsService(reader QueueMetrixReader) *QueueJobsService {
	return &QueueJobsService{reader: reader}
}

// List returns the current bounded report result, newest update first.
func (service *QueueJobsService) List(ctx context.Context) ([]domain.QueueMetrix, error) {
	if service == nil || service.reader == nil {
		return nil, errors.New("queue jobs reporting is not configured")
	}
	jobs, err := service.reader.List(ctx, queueJobsReportLimit)
	if err != nil {
		return nil, err
	}
	sort.Slice(jobs, func(left, right int) bool { return jobs[left].UpdatedAt.After(jobs[right].UpdatedAt) })
	return jobs, nil
}
