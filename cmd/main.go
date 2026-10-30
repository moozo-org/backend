package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"moozo/internal/api"
	"moozo/internal/api/middleware"
	"moozo/internal/api/openapi/generated"
	"moozo/internal/app"
	"moozo/internal/config"
	"moozo/internal/mongodb"
)

const (
	shutdownTimeout = 10 * time.Second
	mongoTimeout    = 10 * time.Second
)

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
	db, disconnect, err := connectMongo(cfg)
	if err != nil {
		return err
	}
	defer disconnect()

	initCtx, cancel := context.WithTimeout(context.Background(), mongoTimeout)
	repo, err := mongodb.NewRepository(initCtx, db)
	cancel()
	if err != nil {
		return err
	}

	handler := api.NewHandler(logger, cfg.Production, app.New(repo, repo))

	srv, err := generated.NewServer(handler, handler,
		generated.WithErrorHandler(api.ErrorHandler(logger, cfg.Production)),
		generated.WithMiddleware(middleware.LoggingMiddleware(logger)),
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

func connectMongo(cfg Config) (*mongo.Database, func(), error) {
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		return nil, nil, fmt.Errorf("connect to mongo: %w", err)
	}
	disconnect := func() {
		ctx, cancel := context.WithTimeout(context.Background(), mongoTimeout)
		defer cancel()
		_ = client.Disconnect(ctx)
	}

	ctx, cancel := context.WithTimeout(context.Background(), mongoTimeout)
	defer cancel()

	if err := client.Ping(ctx, nil); err != nil {
		disconnect()
		return nil, nil, fmt.Errorf("ping mongo: %w", err)
	}

	return client.Database(cfg.MongoDatabase), disconnect, nil
}

type Config struct {
	config.Environment
	MongoURI      string `envconfig:"MONGODB_URI" default:"mongodb://localhost:27017"`
	MongoDatabase string `envconfig:"MONGODB_DATABASE" default:"moozo"`
	Port          int    `envconfig:"PORT" default:"8080"`
}
