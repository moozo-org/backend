package main

import (
	"moozo/internal/api"
	"moozo/internal/api/generated"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func main() {
	e := echo.New()
	e.Use(middleware.RequestLogger())

	// Instantiate the handler
	handler := api.NewHandler()

	// Create ogen server with the handler
	srv, err := generated.NewServer(handler)
	if err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}

	// This registers all the routes defined in your OpenAPI spec
	e.Any("/*", echo.WrapHandler(srv))

	// Start server
	if err := e.Start(":8080"); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}

type Config struct {
	Port int `default:"8080"`
}
