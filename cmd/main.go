package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"moozo/internal/api"
	"moozo/internal/api/generated"
	"moozo/internal/config"
)

const shutdownTimeout = 10 * time.Second

func main() {
	cfg, err := config.Load[Config]()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger, err := zap.NewProduction(zap.AddStacktrace(zapcore.FatalLevel))
	if err != nil {
		log.Fatalf("failed to create logger: %v", err)
	}

	err = run(cfg, logger)
	if err != nil {
		logger.Error("server stopped", zap.Error(err))
	}
	_ = logger.Sync()
	if err != nil {
		os.Exit(1)
	}
}

func run(cfg Config, logger *zap.Logger) error {
	handler := api.NewHandler(logger, cfg.Production)

	srv, err := generated.NewServer(handler,
		generated.WithErrorHandler(api.ErrorHandler(logger, cfg.Production)),
		generated.WithMiddleware(api.LoggingMiddleware(logger)),
	)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	if !cfg.Production {
		mux.HandleFunc("GET /docs", handler.ServeDocs)
		mux.HandleFunc("GET /docs/openapi.yaml", handler.ServeSpec)
	}
	mux.Handle("/", srv)

	httpSrv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           api.Recover(logger, cfg.Production, mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting server",
			zap.String("addr", httpSrv.Addr),
			zap.Bool("docs", !cfg.Production),
		)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return httpSrv.Shutdown(shutdownCtx)
}

type Config struct {
	config.Environment
	MongoURI string `envconfig:"MONGODB_URI" default:"mongodb://localhost:27017"`
	Port     int    `envconfig:"PORT" default:"8080"`
}
