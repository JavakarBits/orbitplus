package config

import (
	"log"

	"github.com/orbitcacheservices/search"
)

func StartApplication() {
	log.Println("Application Starting...")
	search.CreateIndex()
}
