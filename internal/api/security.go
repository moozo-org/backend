package api

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/ogen-go/ogen/ogenerrors"

	"moozo/internal/api/openapi/generated"
	"moozo/internal/app/query"
	"moozo/moozo"
)

var _ generated.SecurityHandler = (*Handler)(nil)

// ErrForbidden is returned when the caller is authenticated but their role is
// not one the operation allows.
var ErrForbidden = errors.New("role not allowed for this operation")

type sessionKey struct{}

// HandleBearerAuth authenticates operations that declare bearerAuth, checks
// the session's role against the roles the operation lists in the spec
// (`bearerAuth: [admin]`), and puts the session in the context for the
// handler.
func (h *Handler) HandleBearerAuth(ctx context.Context, _ generated.OperationName, t generated.BearerAuth) (context.Context, error) {
	s, err := h.app.Queries.Authenticate.Get(ctx, query.Authenticate{Token: t.Token})
	if err != nil {
		return ctx, err
	}
	if !allowed(s.Role, t.Roles) {
		return ctx, ErrForbidden
	}
	return context.WithValue(ctx, sessionKey{}, s), nil
}

// allowed reports whether role may call an operation restricted to roles. An
// operation that lists no roles is open to every authenticated user. There is
// no implicit superuser: admins need to be listed like any other role.
func allowed(role moozo.Role, roles []string) bool {
	return len(roles) == 0 || slices.Contains(roles, string(role))
}

// sessionFrom returns the session HandleBearerAuth stored. ogen runs it before
// every operation that declares bearerAuth, so those handlers can rely on it.
func sessionFrom(ctx context.Context) (*moozo.Session, bool) {
	s, ok := ctx.Value(sessionKey{}).(*moozo.Session)
	return s, ok
}

// statusCode is ogenerrors.ErrorCode, except for security failures, which ogen
// reports as 401 across the board: a missing or invalid token is a 401, a
// role the operation does not allow is a 403, and anything else, such as the
// database being down, is a 500.
func statusCode(err error) int {
	var se *ogenerrors.SecurityError
	if !errors.As(err, &se) {
		return ogenerrors.ErrorCode(err)
	}
	switch {
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, query.ErrUnauthenticated),
		errors.Is(err, ogenerrors.ErrSecurityRequirementIsNotSatisfied):
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}
