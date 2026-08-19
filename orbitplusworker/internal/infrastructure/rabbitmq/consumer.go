// Package rabbitmq implements durable, manually acknowledged delivery.
package rabbitmq

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"orbitplusworker/internal/application/worker"
)

// ConsumerConfig contains connection and queue settings. Passwords are
// populated from environment variables or mounted files by the composition root
// and must never be embedded in a RabbitMQ URL.
type ConsumerConfig struct {
	URL            string
	AppEnvironment worker.AppEnvironment
	Queue          string
	Exchange       string
	RoutingKey     string
	Username       string
	Password       string
	TLSConfig      *tls.Config
	// Prefetch is the AMQP unacknowledged-delivery limit. Process-local
	// operation concurrency belongs exclusively to the application Worker.
	Prefetch  int
	Readiness *worker.Readiness
}

// RabbitMQConsumer owns one AMQP connection and one manual-ack channel.
type RabbitMQConsumer struct {
	connection *amqp.Connection
	config     ConsumerConfig
	mu         sync.Mutex
	channel    *amqp.Channel
}

func ConnectRabbitMQConsumer(config ConsumerConfig) (*RabbitMQConsumer, error) {
	if err := worker.ValidateRabbitMQURL(config.URL, config.AppEnvironment); err != nil {
		return nil, err
	}
	parsed, _ := url.Parse(config.URL)
	if parsed.Scheme == "amqps" && config.TLSConfig == nil {
		return nil, fmt.Errorf("RabbitMQ TLS configuration is required for amqps")
	}
	if config.Queue == "" || config.Exchange == "" || config.RoutingKey == "" || config.Prefetch <= 0 || config.Readiness == nil || (config.Username == "") != (config.Password == "") {
		return nil, fmt.Errorf("RabbitMQ queue, exchange, routing key, prefetch, readiness, and authentication are invalid")
	}
	authentication := amqp.Authentication(&amqp.ExternalAuth{})
	if config.Username != "" {
		authentication = &amqp.PlainAuth{Username: config.Username, Password: config.Password}
	}
	connection, err := amqp.DialConfig(config.URL, amqp.Config{TLSClientConfig: config.TLSConfig, SASL: []amqp.Authentication{authentication}})
	if err != nil {
		return nil, fmt.Errorf("connect RabbitMQ: %w", err)
	}
	return &RabbitMQConsumer{connection: connection, config: config}, nil
}

// Consume uses manual acknowledgement and independently configured RabbitMQ
// prefetch. WorkerConcurrency bounds local operations; Prefetch bounds
// unacknowledged deliveries. The Worker decides whether to invoke
// RabbitMQDelivery.Ack after OrbitPlus reports terminal success.
func (consumer *RabbitMQConsumer) Consume(ctx context.Context) (<-chan worker.RabbitMQDelivery, error) {
	consumer.mu.Lock()
	defer consumer.mu.Unlock()
	if consumer.channel != nil {
		return nil, fmt.Errorf("RabbitMQ consumer is already active")
	}
	channel, err := consumer.connection.Channel()
	if err != nil {
		return nil, fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	closeOnError := func(message string, err error) (<-chan worker.RabbitMQDelivery, error) {
		_ = channel.Close()
		return nil, fmt.Errorf("%s: %w", message, err)
	}
	if err := channel.Qos(consumer.config.Prefetch, 0, false); err != nil {
		return closeOnError("configure RabbitMQ prefetch", err)
	}
	// RabbitMQ topology is provisioned outside this Worker. Passive declarations
	// verify that the configured resources exist without creating or modifying
	// queues, exchanges, bindings, or their arguments.
	if _, err := channel.QueueDeclarePassive(consumer.config.Queue, true, false, false, false, nil); err != nil {
		return closeOnError("verify configured RabbitMQ queue", err)
	}
	if err := channel.ExchangeDeclarePassive(consumer.config.Exchange, amqp.ExchangeDirect, true, false, false, false, nil); err != nil {
		return closeOnError("verify configured RabbitMQ exchange", err)
	}
	consumerTag := "trip-details-refresh-worker"
	deliveries, err := channel.Consume(consumer.config.Queue, consumerTag, false, false, false, false, nil)
	if err != nil {
		return closeOnError("consume RabbitMQ queue", err)
	}
	consumer.channel = channel
	consumer.config.Readiness.MarkReady()
	output := make(chan worker.RabbitMQDelivery)
	go forwardDeliveries(ctx, channel, consumer.config.Queue, consumerTag, deliveries, output, consumer.config.Readiness)
	return output, nil
}

func forwardDeliveries(ctx context.Context, channel *amqp.Channel, queue, tag string, input <-chan amqp.Delivery, output chan<- worker.RabbitMQDelivery, readiness *worker.Readiness) {
	defer close(output)
	defer readiness.MarkNotReady()
	defer channel.Cancel(tag, false)
	for {
		select {
		case <-ctx.Done():
			return
		case delivery, open := <-input:
			if !open {
				return
			}
			slog.Info("RabbitMQ delivery received",
				"messageId", delivery.MessageId,
				"correlationId", delivery.CorrelationId,
				"routingKey", delivery.RoutingKey,
				"queue", queue,
				"consumerTag", tag,
			)
			wrapped := newRabbitMQDelivery(delivery)
			select {
			case <-ctx.Done():
				return
			case output <- wrapped:
			}
		}
	}
}

func (consumer *RabbitMQConsumer) Close() error {
	consumer.config.Readiness.MarkNotReady()
	consumer.mu.Lock()
	defer consumer.mu.Unlock()
	var closeErr error
	if consumer.channel != nil {
		closeErr = consumer.channel.Close()
		consumer.channel = nil
	}
	if err := consumer.connection.Close(); err != nil && closeErr == nil {
		closeErr = err
	}
	return closeErr
}

var _ worker.RabbitMQConsumer = (*RabbitMQConsumer)(nil)
