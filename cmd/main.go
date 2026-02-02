package main

import (
	"log"
	"moozo/internal/api"
	"moozo/internal/api/generated"
	"net/http"
)

func main() {
	handler := api.NewHandler()

	srv, err := generated.NewServer(handler)
	if err != nil {
		log.Fatal("failed to create server:", err)
	}

	log.Println("starting server on http://localhost:8080")
	if err := http.ListenAndServe(":8080", srv); err != nil {
		log.Fatal("failed to start server:", err)
	}
}
