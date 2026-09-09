package worker

import "context"

// RabbitMQDelivery is a durable delivery with Worker-controlled payload extraction
// and acknowledgement. RabbitMQ supplies bytes; the Worker validates the envelope.
type RabbitMQDelivery interface {
	Payload() []byte
	Ack(ctx context.Context) error
	// Requeue returns the delivery to the queue for later redelivery without
	// acknowledging it. The Worker uses it when a zone is rate-limited so the
	// task is retried later while the goroutine moves on to other deliveries.
	Requeue(ctx context.Context) error
}

// RabbitMQConsumer supplies manually acknowledged deliveries.
type RabbitMQConsumer interface {
	Consume(ctx context.Context) (<-chan RabbitMQDelivery, error)
}
