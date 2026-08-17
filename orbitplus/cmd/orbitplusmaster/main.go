// orbitplusmaster runs the Phase 1 HTTP service that receives TripDetails
// payloads from the Worker.
package main

import (
	"log"
	"net/http"

	"orbitplusmaster/internal/application/master"
	masterhttp "orbitplusmaster/internal/infrastructure/http"
)

func main() {
	log.Print("beginning orbitplusmaster startup")

	config, err := master.LoadRuntimeConfig()
	if err != nil {
		log.Fatalf("invalid orbitplusmaster configuration: %v", err)
	}
	log.Printf("orbitplusmaster configuration loaded: APP_ENV=%s MASTER_API_PORT=%d", config.AppEnvironment, config.APIPort)

	tripDetailsService := master.NewTripDetailsService()
	router := masterhttp.NewRouter(tripDetailsService)

	server := &http.Server{
		Addr:    config.Address(),
		Handler: router,
	}

	log.Printf("orbitplusmaster listening on %s", config.Address())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("orbitplusmaster server stopped unexpectedly: %v", err)
	}
}
