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
	"moozo/internal/api/middleware"
	"moozo/internal/api/openapi/generated"
	"moozo/internal/app"
	"moozo/internal/config"
	"moozo/internal/mail"
	"moozo/internal/mongodb"
	"moozo/moozo"
	"moozo/pkg/brevo"
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
	mailer, err := newMailer(cfg, logger)
	if err != nil {
		return err
	}

	db, disconnect, err := mongodb.Connect(cfg.MongoURI, cfg.MongoDatabase)
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

	handler := api.NewHandler(logger, cfg.Production, app.New(repo, repo, repo, mailer))

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

// newMailer sends through Brevo when BREVO_API_KEY is set. Without it, codes
// are logged instead, which production refuses.
func newMailer(cfg Config, logger *zap.Logger) (moozo.Mailer, error) {
	switch {
	case cfg.BrevoAPIKey != "":
		if cfg.MailFromEmail == "" {
			return nil, errors.New("MAIL_FROM_EMAIL is required with BREVO_API_KEY")
		}
		return mail.NewBrevo(brevo.NewClient(cfg.BrevoAPIKey), cfg.MailFromEmail, cfg.MailFromName), nil
	case cfg.Production:
		return nil, errors.New("BREVO_API_KEY is required when PRODUCTION is true")
	default:
		logger.Warn("BREVO_API_KEY is unset: verification codes are logged, not emailed")
		return mail.NewLog(logger), nil
	}
}

type Config struct {
	config.Environment
	MongoURI      string `envconfig:"MONGODB_URI" default:"mongodb://localhost:27017"`
	MongoDatabase string `envconfig:"MONGODB_DATABASE" default:"moozo"`
	Port          int    `envconfig:"PORT" default:"8080"`

	BrevoAPIKey   string `envconfig:"BREVO_API_KEY"`
	MailFromEmail string `envconfig:"MAIL_FROM_EMAIL"`
	MailFromName  string `envconfig:"MAIL_FROM_NAME" default:"Moozo"`
}
