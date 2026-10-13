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
	url           string
	connection    *amqp.Connection
	channel       *amqp.Channel
	confirmations <-chan amqp.Confirmation
	channelErrors <-chan *amqp.Error
	exchange      string
	mutex         sync.Mutex
}

// NewInventoryEventPublisher connects to the configured RabbitMQ broker.
func NewInventoryEventPublisher(url, exchange string) (*InventoryEventPublisher, error) {
	publisher := &InventoryEventPublisher{url: url, exchange: exchange}
	if err := publisher.connect(); err != nil {
		return nil, err
	}
	return publisher, nil
}

// connect opens a fresh connection and channel with publisher confirms enabled.
// It replaces any existing connection/channel on the publisher.
func (publisher *InventoryEventPublisher) connect() error {
	connection, err := amqp.Dial(publisher.url)
	if err != nil {
		return fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return fmt.Errorf("enable RabbitMQ publisher confirms: %w", err)
	}
	publisher.connection = connection
	publisher.channel = channel
	publisher.confirmations = channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	publisher.channelErrors = channel.NotifyClose(make(chan *amqp.Error, 1))
	return nil
}

// ensureChannel reconnects if the current connection or channel is closed, so a
// broker restart or idle timeout does not permanently break publishing.
func (publisher *InventoryEventPublisher) ensureChannel() error {
	if publisher.connection != nil && !publisher.connection.IsClosed() &&
		publisher.channel != nil && !publisher.channel.IsClosed() {
		return nil
	}
	return publisher.connect()
}

// PublishInventoryEvent publishes an Orionmax event at its fixed highest priority.
func (publisher *InventoryEventPublisher) PublishInventoryEvent(ctx context.Context, referenceID string, payload []byte) error {
	return publisher.PublishInventoryEventWithPriority(ctx, referenceID, payload, 10)
}

// PublishInventoryEventWithPriority publishes a Worker job with the supplied AMQP priority.
func (publisher *InventoryEventPublisher) PublishInventoryEventWithPriority(ctx context.Context, referenceID string, payload []byte, priority uint8) error {
	return publisher.publish(ctx, referenceID, payload, priority)
}

func (publisher *InventoryEventPublisher) publish(ctx context.Context, referenceID string, payload []byte, priority uint8) error {
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()

	message := amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		Timestamp:     time.Now().UTC(),
		Priority:      priority,
		MessageId:     referenceID,
		CorrelationId: referenceID,
		Body:          payload,
	}

	// Reconnect if the channel/connection died (broker restart, idle timeout, or
	// a prior "channel/connection is not open" failure), then publish. One retry
	// covers the case where the channel dies between the check and the publish.
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := publisher.ensureChannel(); err != nil {
			lastErr = err
			continue
		}
		if err := publisher.channel.PublishWithContext(ctx, publisher.exchange, master.InventoryRefreshRoutingKey, false, false, message); err != nil {
			lastErr = fmt.Errorf("publish inventory event: %w", err)
			// Force a reconnect on the next attempt.
			if publisher.channel != nil {
				_ = publisher.channel.Close()
			}
			continue
		}
		return publisher.awaitConfirmation(ctx)
	}
	return lastErr
}

// awaitConfirmation waits for the broker to confirm the publish.
func (publisher *InventoryEventPublisher) awaitConfirmation(ctx context.Context) error {
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
	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()

	var channelErr, connectionErr error
	if publisher.channel != nil {
		channelErr = publisher.channel.Close()
	}
	if publisher.connection != nil {
		connectionErr = publisher.connection.Close()
	}
	if channelErr != nil {
		return channelErr
	}
	return connectionErr
}
