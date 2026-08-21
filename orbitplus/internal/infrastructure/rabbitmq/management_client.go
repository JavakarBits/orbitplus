package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"orbitplusmaster/internal/application/master"
)

const managementResponseLimit = 8 << 20

// ManagementReader reads selected RabbitMQ Management API operational data.
type ManagementReader interface {
	Snapshot(context.Context) (ManagementSnapshot, error)
}

// ManagementClient calls selected read-only RabbitMQ Management API endpoints.
type ManagementClient struct {
	baseURL  string
	username string
	password string
	vhost    string
	client   *http.Client
}

// ManagementSnapshot contains the safe operational data shown in the admin portal.
type ManagementSnapshot struct {
	ClusterName string                 `json:"clusterName"`
	Version     string                 `json:"version"`
	Totals      ManagementTotals       `json:"totals"`
	Queues      []ManagementQueue      `json:"queues"`
	Exchanges   []ManagementExchange   `json:"exchanges"`
	Consumers   []ManagementConsumer   `json:"consumers"`
	Connections []ManagementConnection `json:"connections"`
}

type ManagementTotals struct {
	Connections int `json:"connections"`
	Channels    int `json:"channels"`
	Queues      int `json:"queues"`
	Exchanges   int `json:"exchanges"`
	Consumers   int `json:"consumers"`
}
type ManagementQueue struct {
	Name           string `json:"name"`
	VHost          string `json:"vHost"`
	State          string `json:"state"`
	Messages       int    `json:"messages"`
	Ready          int    `json:"ready"`
	Unacknowledged int    `json:"unacknowledged"`
	Consumers      int    `json:"consumers"`
	Durable        bool   `json:"durable"`
}
type ManagementExchange struct {
	Name    string `json:"name"`
	VHost   string `json:"vHost"`
	Type    string `json:"type"`
	Durable bool   `json:"durable"`
}
type ManagementConsumer struct {
	Tag           string `json:"tag"`
	Queue         string `json:"queue"`
	VHost         string `json:"vHost"`
	User          string `json:"user"`
	Connection    string `json:"connection"`
	Status        string `json:"status"`
	AckRequired   bool   `json:"ackRequired"`
	Exclusive     bool   `json:"exclusive"`
	PrefetchCount int    `json:"prefetchCount"`
}
type ManagementConnection struct {
	Name     string `json:"name"`
	User     string `json:"user"`
	VHost    string `json:"vHost"`
	State    string `json:"state"`
	Channels int    `json:"channels"`
}

// NewManagementClient constructs a Management API client without performing a network call.
func NewManagementClient(config master.RabbitMQManagementConfig) *ManagementClient {
	return &ManagementClient{baseURL: config.URL, username: config.Username, password: config.Password, vhost: config.VHost, client: &http.Client{Timeout: config.Timeout}}
}

// Snapshot reads the overview, queues, exchanges, consumers, and connections endpoints.
func (client *ManagementClient) Snapshot(ctx context.Context) (ManagementSnapshot, error) {
	var overview managementOverview
	var queues []managementQueue
	var exchanges []managementExchange
	var consumers []managementConsumer
	var connections []managementConnection
	vhost := url.PathEscape(client.vhost)
	if err := client.get(ctx, "/api/overview", &overview); err != nil {
		return ManagementSnapshot{}, err
	}
	if err := client.get(ctx, "/api/queues/"+vhost, &queues); err != nil {
		return ManagementSnapshot{}, err
	}
	if err := client.get(ctx, "/api/exchanges/"+vhost, &exchanges); err != nil {
		return ManagementSnapshot{}, err
	}
	if err := client.get(ctx, "/api/consumers", &consumers); err != nil {
		return ManagementSnapshot{}, err
	}
	if err := client.get(ctx, "/api/connections", &connections); err != nil {
		return ManagementSnapshot{}, err
	}
	snapshot := ManagementSnapshot{ClusterName: overview.ClusterName, Version: overview.RabbitMQVersion, Totals: ManagementTotals{Connections: overview.ObjectTotals.Connections, Channels: overview.ObjectTotals.Channels, Queues: overview.ObjectTotals.Queues, Exchanges: overview.ObjectTotals.Exchanges, Consumers: overview.ObjectTotals.Consumers}, Queues: make([]ManagementQueue, 0, len(queues)), Exchanges: make([]ManagementExchange, 0, len(exchanges)), Consumers: make([]ManagementConsumer, 0, len(consumers)), Connections: make([]ManagementConnection, 0, len(connections))}
	for _, queue := range queues {
		snapshot.Queues = append(snapshot.Queues, ManagementQueue{Name: queue.Name, VHost: queue.VHost, State: queue.State, Messages: queue.Messages, Ready: queue.MessagesReady, Unacknowledged: queue.MessagesUnacknowledged, Consumers: queue.Consumers, Durable: queue.Durable})
	}
	for _, exchange := range exchanges {
		snapshot.Exchanges = append(snapshot.Exchanges, ManagementExchange{Name: exchange.Name, VHost: exchange.VHost, Type: exchange.Type, Durable: exchange.Durable})
	}
	for _, consumer := range consumers {
		if consumer.Queue.VHost != client.vhost {
			continue
		}
		snapshot.Consumers = append(snapshot.Consumers, ManagementConsumer{Tag: consumer.ConsumerTag, Queue: consumer.Queue.Name, VHost: consumer.Queue.VHost, User: consumer.ChannelDetails.User, Connection: consumer.ChannelDetails.ConnectionName, Status: consumer.ActivityStatus, AckRequired: consumer.AckRequired, Exclusive: consumer.Exclusive, PrefetchCount: consumer.PrefetchCount})
	}
	snapshot.Totals.Consumers = len(snapshot.Consumers)
	for _, connection := range connections {
		snapshot.Connections = append(snapshot.Connections, ManagementConnection{Name: connection.Name, User: connection.User, VHost: connection.VHost, State: connection.State, Channels: connection.Channels})
	}
	return snapshot, nil
}

func (client *ManagementClient) get(ctx context.Context, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create RabbitMQ management request: %w", err)
	}
	request.SetBasicAuth(client.username, client.password)
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("call RabbitMQ management API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("RabbitMQ management API returned %s", response.Status)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, managementResponseLimit)).Decode(target); err != nil {
		return fmt.Errorf("decode RabbitMQ management response: %w", err)
	}
	return nil
}

type managementOverview struct {
	ClusterName     string `json:"cluster_name"`
	RabbitMQVersion string `json:"rabbitmq_version"`
	ObjectTotals    struct {
		Connections int `json:"connections"`
		Channels    int `json:"channels"`
		Queues      int `json:"queues"`
		Exchanges   int `json:"exchanges"`
		Consumers   int `json:"consumers"`
	} `json:"object_totals"`
}
type managementQueue struct {
	Name                   string `json:"name"`
	VHost                  string `json:"vhost"`
	State                  string `json:"state"`
	Messages               int    `json:"messages"`
	MessagesReady          int    `json:"messages_ready"`
	MessagesUnacknowledged int    `json:"messages_unacknowledged"`
	Consumers              int    `json:"consumers"`
	Durable                bool   `json:"durable"`
}
type managementExchange struct {
	Name    string `json:"name"`
	VHost   string `json:"vhost"`
	Type    string `json:"type"`
	Durable bool   `json:"durable"`
}
type managementConnection struct {
	Name     string `json:"name"`
	User     string `json:"user"`
	VHost    string `json:"vhost"`
	State    string `json:"state"`
	Channels int    `json:"channels"`
}
type managementConsumer struct {
	ConsumerTag string `json:"consumer_tag"`
	Queue       struct {
		Name  string `json:"name"`
		VHost string `json:"vhost"`
	} `json:"queue"`
	ChannelDetails struct {
		User           string `json:"user"`
		ConnectionName string `json:"connection_name"`
	} `json:"channel_details"`
	AckRequired    bool   `json:"ack_required"`
	Exclusive      bool   `json:"exclusive"`
	PrefetchCount  int    `json:"prefetch_count"`
	ActivityStatus string `json:"activity_status"`
}

func (client *ManagementClient) String() string { return strings.TrimSpace(client.baseURL) }
