// Package app is the application layer. Transport handlers call a command
// (changes state) or a query (reads state); each one holds the use case's
// logic and calls one or more repositories.
package app

import (
	"moozo/internal/app/command"
	"moozo/internal/app/query"
	"moozo/moozo"
)

type Application struct {
	Commands Commands
	Queries  Queries
}

type Commands struct {
	RegisterUser CommandHandler[command.RegisterUser, *moozo.User]
	Login        CommandHandler[command.Login, *command.LoginResult]
	Logout       CommandHandler[command.Logout, struct{}]
}

type Queries struct {
	Authenticate QueryHandler[query.Authenticate, *moozo.Session]
}

func New(users moozo.UserRepository, sessions moozo.SessionRepository) Application {
	return Application{
		Commands: Commands{
			RegisterUser: command.NewRegisterUserHandler(users),
			Login:        command.NewLoginHandler(users, sessions),
			Logout:       command.NewLogoutHandler(sessions),
		},
		Queries: Queries{
			Authenticate: query.NewAuthenticateHandler(sessions),
		},
	}
}
