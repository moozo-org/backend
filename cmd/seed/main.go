// Command seed creates test accounts for every role in a local database. It
// is idempotent, and refuses to run when PRODUCTION is true.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"moozo/internal/app/command"
	"moozo/internal/config"
	"moozo/internal/mongodb"
	"moozo/moozo"
)

const (
	// Password is shared by every seeded account; the README lists it.
	Password = "password123"

	usersPerRole = 3
	seedTimeout  = 30 * time.Second
)

type Config struct {
	config.Environment
	MongoURI      string `envconfig:"MONGODB_URI" default:"mongodb://localhost:27017"`
	MongoDatabase string `envconfig:"MONGODB_DATABASE" default:"moozo"`
}

func main() {
	cfg, err := config.Load[Config]()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if err := run(cfg); err != nil {
		log.Fatalf("seed failed: %v", err)
	}
}

func run(cfg Config) error {
	if cfg.Production {
		return errors.New("refusing to seed with PRODUCTION=true")
	}

	db, disconnect, err := mongodb.Connect(cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		return err
	}
	defer disconnect()

	ctx, cancel := context.WithTimeout(context.Background(), seedTimeout)
	defer cancel()

	repo, err := mongodb.NewRepository(ctx, db)
	if err != nil {
		return err
	}
	create := command.NewCreateUserHandler(repo)

	for _, role := range moozo.Roles {
		for i := 1; i <= usersPerRole; i++ {
			email := fmt.Sprintf("%s%d@moozo.test", role, i)
			_, err := create.Execute(ctx, command.CreateUser{Email: email, Password: Password, Role: role})
			switch {
			case errors.Is(err, moozo.ErrEmailTaken):
				log.Printf("exists   %-8s %s", role, email)
			case err != nil:
				return fmt.Errorf("create %s: %w", email, err)
			default:
				log.Printf("created  %-8s %s", role, email)
			}
		}
	}
	return nil
}
