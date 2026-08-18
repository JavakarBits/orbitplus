// orbitplusmaster runs the HTTP service that receives and serves TripDetails.
package main

import (
	"context"
	"log"
	"net/http"

	"orbitplusmaster/internal/application/master"
	"orbitplusmaster/internal/infrastructure/cassandra"
	"orbitplusmaster/internal/infrastructure/dragonfly"
	masterhttp "orbitplusmaster/internal/infrastructure/http"
)

func main() {
	log.Print("beginning orbitplusmaster startup")
	config, err := master.LoadRuntimeConfig()
	if err != nil {
		log.Fatalf("invalid orbitplusmaster configuration: %v", err)
	}
	log.Printf("orbitplusmaster configuration loaded: APP_ENV=%s MASTER_API_PORT=%d", config.AppEnvironment, config.APIPort)

	tripDetailsService, readService, closePersistence, err := newMasterServices(config)
	if err != nil {
		log.Fatalf("initialize TripDetails persistence: %v", err)
	}
	defer closePersistence()
	router := masterhttp.NewRouter(tripDetailsService, readService)
	server := &http.Server{Addr: config.Address(), Handler: router}
	log.Printf("orbitplusmaster listening on %s", config.Address())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("orbitplusmaster server stopped unexpectedly: %v", err)
	}
}

func newMasterServices(config master.RuntimeConfig) (*master.TripDetailsService, *master.TripDetailsReadService, func(), error) {
	if config.Persistence == nil {
		log.Print("TripDetails persistence disabled: ingestion is log-only and persisted reads are unavailable")
		return master.NewTripDetailsService(), nil, func() {}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.Persistence.Cassandra.Timeout)
	defer cancel()
	cache, err := dragonfly.NewTripDetailsCacheRepository(ctx, dragonfly.Config{
		Address: config.Persistence.Dragonfly.Address, Password: config.Persistence.Dragonfly.Password,
		Database: config.Persistence.Dragonfly.Database, DialTimeout: config.Persistence.Dragonfly.DialTimeout,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	metadata, err := cassandra.NewTripDetailsMetadataRepository(ctx, cassandra.Config{
		Hosts: config.Persistence.Cassandra.Hosts, Port: config.Persistence.Cassandra.Port,
		Keyspace: config.Persistence.Cassandra.Keyspace, Username: config.Persistence.Cassandra.Username,
		Password: config.Persistence.Cassandra.Password, Timeout: config.Persistence.Cassandra.Timeout,
	})
	if err != nil {
		_ = cache.Close()
		return nil, nil, nil, err
	}
	persistence := master.NewTripDetailsPersistenceWithLogger(cache, metadata, log.Default())
	readService := master.NewTripDetailsReadService(cache, metadata, log.Default())
	closePersistence := func() { metadata.Close(); _ = cache.Close() }
	return master.NewTripDetailsServiceWithPersistence(log.Default(), persistence), readService, closePersistence, nil
}
