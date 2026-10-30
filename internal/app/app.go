// Package app is the application layer. Transport handlers call a command
// (changes state) or a query (reads state); each one holds the use case's
// logic and calls one or more repositories.
package app

import (
	"moozo/internal/app/command"
	"moozo/moozo"
)

type Application struct {
	Commands Commands
	Queries  Queries
}

type Commands struct {
	RegisterUser CommandHandler[command.RegisterUser, *moozo.User]
}

type Queries struct{}

func New(users moozo.UserRepository) Application {
	return Application{
		Commands: Commands{
			RegisterUser: command.NewRegisterUserHandler(users),
		},
	}
}
