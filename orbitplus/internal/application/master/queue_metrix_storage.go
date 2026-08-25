package master

import (
	"context"

	"orbitplusmaster/internal/domain"
)

// QueueMetrixStorage records the lifecycle of one Orionmax queue job.
type QueueMetrixStorage interface {
	SaveReceived(context.Context, domain.QueueMetrix) error
	MarkQueued(context.Context, domain.QueueMetrix) error
	MarkCompleted(context.Context, domain.QueueMetrix) error
	MarkDead(context.Context, domain.QueueMetrix) error
}

// QueueMetrixReader lists queue lifecycle records for reports and analytics.
type QueueMetrixReader interface {
	List(context.Context, int) ([]domain.QueueMetrix, error)
}

// QueueMetrixReferenceReader reads one queue lifecycle record by its exact reference ID.
type QueueMetrixReferenceReader interface {
	FindByReferenceID(context.Context, string) (domain.QueueMetrix, bool, error)
}
