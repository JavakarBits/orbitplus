package worker

import "context"

// RabbitMQDelivery is a durable delivery with Worker-controlled payload extraction
// and acknowledgement. RabbitMQ supplies bytes; the Worker validates the envelope.
type RabbitMQDelivery interface {
	Payload() []byte
	Ack(ctx context.Context) error
}

// RabbitMQConsumer supplies manually acknowledged deliveries.
type RabbitMQConsumer interface {
	Consume(ctx context.Context) (<-chan RabbitMQDelivery, error)
}
