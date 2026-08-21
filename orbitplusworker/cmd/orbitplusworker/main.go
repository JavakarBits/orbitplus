// orbitplusworker runs the standalone direct TripDetails refresh flow.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"orbitplusworker/internal/application/worker"
	"orbitplusworker/internal/infrastructure/bits"
	"orbitplusworker/internal/infrastructure/orbit"
	"orbitplusworker/internal/infrastructure/orbitplus"
	"orbitplusworker/internal/infrastructure/rabbitmq"
)

type authenticatedTransport struct {
	base        http.RoundTripper
	bearerToken string
}

func (transport authenticatedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.bearerToken == "" {
		return transport.base.RoundTrip(request)
	}
	requestCopy := request.Clone(request.Context())
	requestCopy.Header.Set("Authorization", "Bearer "+transport.bearerToken)
	return transport.base.RoundTrip(requestCopy)
}

func newHTTPClient(tlsConfig worker.TLSFileConfig, timeout time.Duration, auth worker.HTTPAuthConfig) (*http.Client, error) {
	transportTLS, err := tlsConfig.BuildTLSConfig()
	if err != nil {
		return nil, err
	}
	return &http.Client{Timeout: timeout, Transport: authenticatedTransport{base: &http.Transport{TLSClientConfig: transportTLS}, bearerToken: auth.BearerToken}}, nil
}

func startHealthServer(config worker.HealthAPIConfig, readiness *worker.Readiness) (func(), error) {
	server := &http.Server{
		Addr:              config.Address(),
		Handler:           worker.NewHealthHandler(readiness),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return nil, fmt.Errorf("listen on health API address %s: %w", server.Addr, err)
	}

	var shutdownOnce sync.Once
	shutdown := func() {
		shutdownOnce.Do(func() {
			shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownContext); err != nil {
				log.Printf("health API server shutdown error: %v", err)
				return
			}
			log.Print("health API server shutdown complete")
		})
	}
	go func() {
		log.Printf("health API server listening on %s", listener.Addr())
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("health API server stopped unexpectedly: %v", err)
		}
	}()
	return shutdown, nil
}

func main() {
	log.Print("beginning tripdetails refresh worker startup")
	config, err := worker.LoadRuntimeConfig()
	if err != nil {
		log.Fatalf("invalid tripdetails refresh worker configuration: %v", err)
	}
	log.Printf("tripdetails refresh worker configuration loaded: APP_ENV=%s", config.AppEnvironment)

	readiness := worker.NewReadiness()
	shutdownHealthServer, err := startHealthServer(config.HealthAPI, readiness)
	if err != nil {
		log.Fatalf("cannot start health API server: %v", err)
	}

	runContext, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	shutdownComplete := make(chan struct{})
	defer func() {
		close(shutdownComplete)
		signal.Stop(signals)
		readiness.MarkNotReady()
		shutdownHealthServer()
	}()
	go func() {
		select {
		case signalValue := <-signals:
			log.Printf("received %s; beginning controlled shutdown", signalValue)
			readiness.MarkNotReady()
			shutdownHealthServer()
			cancelRun()
		case <-shutdownComplete:
		}
	}()

	log.Print("beginning RabbitMQ connection")
	rabbitTLS, err := config.RabbitMQ.TLS.BuildTLSConfig()
	if err != nil {
		log.Printf("invalid RabbitMQ TLS configuration: %v", err)
		return
	}
	consumer, err := rabbitmq.ConnectRabbitMQConsumer(rabbitmq.ConsumerConfig{
		URL: config.RabbitMQ.URL, AppEnvironment: config.AppEnvironment, Queue: config.RabbitMQ.Queue, Exchange: config.RabbitMQ.Exchange, RoutingKey: config.RabbitMQ.RoutingKey,
		Username: config.RabbitMQ.Username, Password: config.RabbitMQ.Password, TLSConfig: rabbitTLS,
		Prefetch: config.RabbitMQ.Prefetch, Readiness: readiness,
	})
	if err != nil {
		log.Printf("cannot connect RabbitMQ: %v", err)
		return
	}
	defer consumer.Close()
	log.Print("RabbitMQ connection established")

	log.Print("beginning Bits client setup")
	bitsHTTP, err := newHTTPClient(config.Bits.TLS, config.HTTPTimeout, worker.HTTPAuthConfig{})
	if err != nil {
		log.Printf("invalid Bits TLS configuration: %v", err)
		return
	}
	bitsClient, err := bits.NewBitsTripDetailsClient(bitsHTTP, config.AppEnvironment)
	if err != nil {
		log.Printf("invalid Bits client configuration: %v", err)
		return
	}
	log.Print("Bits client setup complete")

	log.Print("beginning Orbit credential client setup")
	orbitHTTP, err := newHTTPClient(config.Orbit.TLS, config.HTTPTimeout, worker.HTTPAuthConfig{})
	if err != nil {
		log.Printf("invalid Orbit TLS configuration: %v", err)
		return
	}
	credentialClient, err := orbit.NewClient(config.Orbit.Endpoint, config.Orbit.NamespaceCode, config.Orbit.AccessToken, config.OrbitPlusResponseSize, orbitHTTP, config.AppEnvironment)
	if err != nil {
		log.Printf("invalid Orbit credential client configuration: %v", err)
		return
	}
	log.Print("Orbit credential client setup complete")

	log.Print("beginning OrbitPlus client setup")
	orbitPlusHTTP, err := newHTTPClient(config.OrbitPlus.TLS, config.HTTPTimeout, config.OrbitPlus.Auth)
	if err != nil {
		log.Printf("invalid OrbitPlus TLS configuration: %v", err)
		return
	}
	orbitPlusClient, err := orbitplus.NewClient(config.OrbitPlus.Endpoint, config.OrbitPlusResponseSize, orbitPlusHTTP, config.AppEnvironment)
	if err != nil {
		log.Printf("invalid OrbitPlus configuration: %v", err)
		return
	}
	log.Print("OrbitPlus client setup complete")

	refreshWorker, err := worker.NewTripDetailsRefreshWorker(config.Worker, consumer, bitsClient, credentialClient, orbitPlusClient)
	if err != nil {
		log.Printf("cannot construct TripDetailsRefreshWorker: %v", err)
		return
	}
	log.Print("TripDetailsRefreshWorker starting")
	runErr := refreshWorker.Run(runContext)
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		log.Printf("TripDetailsRefreshWorker stopped unexpectedly: %v", runErr)
		return
	}
	if runContext.Err() != nil || errors.Is(runErr, context.Canceled) {
		log.Print("TripDetailsRefreshWorker controlled shutdown")
		return
	}
	log.Print("TripDetailsRefreshWorker stopped unexpectedly")
}
