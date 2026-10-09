package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"

	"moozo/internal/api/openapi/generated"
	"moozo/internal/app/query"
	"moozo/moozo"
)

var _ generated.SecurityHandler = (*Handler)(nil)

type sessionKey struct{}

// HandleBearerAuth authenticates operations that declare bearerAuth and puts
// the session in the context for the handler.
func (h *Handler) HandleBearerAuth(ctx context.Context, _ generated.OperationName, t generated.BearerAuth) (context.Context, error) {
	s, err := h.app.Queries.Authenticate.Get(ctx, query.Authenticate{Token: t.Token})
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, sessionKey{}, s), nil
}

// sessionFrom returns the session HandleBearerAuth stored. ogen runs it before
// every operation that declares bearerAuth, so those handlers can rely on it.
func sessionFrom(ctx context.Context) (*moozo.Session, bool) {
	s, ok := ctx.Value(sessionKey{}).(*moozo.Session)
	return s, ok
}

// statusCode is ogenerrors.ErrorCode, except that ogen reports every security
// failure as 401: only a missing or invalid token is the client's fault, so
// anything else, such as the database being down, becomes a 500.
func statusCode(err error) int {
	var se *ogenerrors.SecurityError
	if errors.As(err, &se) &&
		!errors.Is(err, query.ErrUnauthenticated) &&
		!errors.Is(err, ogenerrors.ErrSecurityRequirementIsNotSatisfied) {
		return http.StatusInternalServerError
	}
	return ogenerrors.ErrorCode(err)
}
