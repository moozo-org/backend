package app

import "context"

// CommandHandler executes a use case that changes state. R is the result the
// caller needs to respond, such as the created entity; use struct{} for none.
type CommandHandler[C, R any] interface {
	Execute(ctx context.Context, cmd C) (R, error)
}

// QueryHandler runs a use case that only reads state.
type QueryHandler[Q, R any] interface {
	Get(ctx context.Context, q Q) (R, error)
}
