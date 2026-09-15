// Command api runs the ledger HTTP API.
package main

import (
	"errors"
	"log"
	"net/http"

	"ledger-api/internal/config"
	"ledger-api/internal/database"
	"ledger-api/internal/server"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	srv := server.New(cfg, db)

	log.Printf("listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}
