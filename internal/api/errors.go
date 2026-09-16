package api

import (
	"context"
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"
	"go.uber.org/zap"

	"moozo/internal/api/generated"
)

// ErrorHandler covers request-decode and response-encode failures. Errors
// returned by a handler go through Handler.NewError instead.
func ErrorHandler(logger *zap.Logger, production bool) ogenerrors.ErrorHandler {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
		code := ogenerrors.ErrorCode(err)

		logAt(withTrace(ctx, logger), code, "request failed",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status_code", code),
			zap.Error(err),
		)

		writeError(w, code, errorBody(code, err, production))
	}
}

// errorBody attaches err.Error() as debug detail outside production only.
func errorBody(code int, err error, production bool) generated.Error {
	body := generated.Error{ErrorMessage: http.StatusText(code)}
	if !production {
		body.Debug = generated.NewOptString(err.Error())
	}
	return body
}

func writeError(w http.ResponseWriter, code int, body generated.Error) {
	// Appending to a response that already started would corrupt it.
	if tw, ok := w.(*trackingWriter); ok && tw.wrote {
		return
	}

	payload, err := body.MarshalJSON()
	if err != nil {
		code, payload = http.StatusInternalServerError, []byte(`{"error_message":"Internal Server Error"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(payload)
}
