package rabbitmq

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"orbitplusmaster/internal/application/master"
)

// InventoryEventPublisher publishes raw Orionmax events to RabbitMQ.
type InventoryEventPublisher struct {
	connection    *amqp.Connection
	channel       *amqp.Channel
	confirmations <-chan amqp.Confirmation
	channelErrors <-chan *amqp.Error
	exchange      string
	mutex         sync.Mutex
}

// NewInventoryEventPublisher connects to the configured RabbitMQ broker.
func NewInventoryEventPublisher(url, exchange string) (*InventoryEventPublisher, error) {
	connection, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("enable RabbitMQ publisher confirms: %w", err)
	}
	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	channelErrors := channel.NotifyClose(make(chan *amqp.Error, 1))
	return &InventoryEventPublisher{connection: connection, channel: channel, confirmations: confirmations, channelErrors: channelErrors, exchange: exchange}, nil
}

// PublishInventoryEvent publishes an accepted event with the refresh routing key.
func (publisher *InventoryEventPublisher) PublishInventoryEvent(ctx context.Context, referenceID string, payload []byte) error {
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()

	message := amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		Timestamp:     time.Now().UTC(),
		Priority:      10,
		MessageId:     referenceID,
		CorrelationId: referenceID,
		Body:          payload,
	}
	if err := publisher.channel.PublishWithContext(ctx, publisher.exchange, master.InventoryRefreshRoutingKey, false, false, message); err != nil {
		return fmt.Errorf("publish inventory event: %w", err)
	}
	select {
	case confirmation, open := <-publisher.confirmations:
		if !open || !confirmation.Ack {
			return fmt.Errorf("RabbitMQ did not confirm inventory event")
		}
		return nil
	case channelError, open := <-publisher.channelErrors:
		if open && channelError != nil {
			return fmt.Errorf("RabbitMQ channel closed: %w", channelError)
		}
		return fmt.Errorf("RabbitMQ channel closed before confirmation")
	case <-ctx.Done():
		return fmt.Errorf("wait for RabbitMQ publish confirmation: %w", ctx.Err())
	}
}

// Close releases the RabbitMQ channel and connection.
func (publisher *InventoryEventPublisher) Close() error {
	channelErr := publisher.channel.Close()
	connectionErr := publisher.connection.Close()
	if channelErr != nil {
		return channelErr
	}
	return connectionErr
}
