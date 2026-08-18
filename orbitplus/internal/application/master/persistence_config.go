package master

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PersistenceConfig contains the explicit Phase 2 datastore settings.
type PersistenceConfig struct {
	Dragonfly DragonflyConfig
	Cassandra CassandraConfig
}

type DragonflyConfig struct {
	Address     string
	Password    string
	Database    int
	DialTimeout time.Duration
}

type CassandraConfig struct {
	Hosts    []string
	Port     int
	Keyspace string
	Username string
	Password string
	Timeout  time.Duration
}

func loadPersistenceConfig() (*PersistenceConfig, error) {
	dragonflyAddress := os.Getenv("DRAGONFLY_ADDRESS")
	cassandraHosts := os.Getenv("CASSANDRA_HOSTS")
	if dragonflyAddress == "" && cassandraHosts == "" {
		return nil, nil
	}
	if dragonflyAddress == "" || cassandraHosts == "" {
		return nil, fmt.Errorf("DRAGONFLY_ADDRESS and CASSANDRA_HOSTS must both be set for Phase 2 persistence")
	}
	dragonflyTimeout, err := requiredDuration("DRAGONFLY_CONNECTION_TIMEOUT")
	if err != nil {
		return nil, err
	}
	cassandraTimeout, err := requiredDuration("CASSANDRA_CONNECTION_TIMEOUT")
	if err != nil {
		return nil, err
	}
	cassandraPort, err := requiredPort("CASSANDRA_PORT")
	if err != nil {
		return nil, err
	}
	keyspace := os.Getenv("CASSANDRA_KEYSPACE")
	if keyspace == "" {
		return nil, fmt.Errorf("CASSANDRA_KEYSPACE must be set for Phase 2 persistence")
	}
	database, err := optionalNonNegativeInt("DRAGONFLY_DATABASE")
	if err != nil {
		return nil, err
	}
	hosts := splitHosts(cassandraHosts)
	if len(hosts) == 0 {
		return nil, fmt.Errorf("CASSANDRA_HOSTS must include at least one host")
	}
	return &PersistenceConfig{
		Dragonfly: DragonflyConfig{Address: dragonflyAddress, Password: os.Getenv("DRAGONFLY_PASSWORD"), Database: database, DialTimeout: dragonflyTimeout},
		Cassandra: CassandraConfig{Hosts: hosts, Port: cassandraPort, Keyspace: keyspace, Username: os.Getenv("CASSANDRA_USERNAME"), Password: os.Getenv("CASSANDRA_PASSWORD"), Timeout: cassandraTimeout},
	}, nil
}

func requiredDuration(name string) (time.Duration, error) {
	value := os.Getenv(name)
	duration, err := time.ParseDuration(value)
	if value == "" || err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}

func requiredPort(name string) (int, error) {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 1 || value > 65535 {
		return 0, fmt.Errorf("%s must be an integer between 1 and 65535", name)
	}
	return value, nil
}

func optionalNonNegativeInt(name string) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return parsed, nil
}

func splitHosts(value string) []string {
	var hosts []string
	for _, host := range strings.Split(value, ",") {
		if host = strings.TrimSpace(host); host != "" {
			hosts = append(hosts, host)
		}
	}
	return hosts
}
