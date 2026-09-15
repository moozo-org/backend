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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /docs", handler.ServeDocs)
	mux.HandleFunc("GET /docs/openapi.yaml", handler.ServeSpec)
	mux.Handle("/", srv)

	log.Println("starting server on http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal("failed to start server:", err)
	}
}
