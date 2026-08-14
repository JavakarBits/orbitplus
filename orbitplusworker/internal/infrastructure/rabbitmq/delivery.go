package rabbitmq

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"orbitplusworker/internal/application/worker"
)

// RabbitMQDelivery retains broker payload bytes and has no implicit
// acknowledgement path. The application Worker extracts and validates the
// refresh envelope, then decides whether to invoke Ack.
type RabbitMQDelivery struct {
	payload []byte
	ack     func() error
}

func newRabbitMQDelivery(delivery amqp.Delivery) *RabbitMQDelivery {
	return &RabbitMQDelivery{payload: append([]byte(nil), delivery.Body...), ack: func() error { return delivery.Ack(false) }}
}

func (delivery *RabbitMQDelivery) Payload() []byte { return append([]byte(nil), delivery.payload...) }

func (delivery *RabbitMQDelivery) Ack(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delivery.ack == nil {
		return fmt.Errorf("RabbitMQ delivery acknowledgement is unavailable")
	}
	return delivery.ack()
}

var _ worker.RabbitMQDelivery = (*RabbitMQDelivery)(nil)
