package api

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"go.uber.org/zap"
)

// trackingWriter records whether the response has started, so writeError can
// decline to append to a half-sent body.
type trackingWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *trackingWriter) WriteHeader(code int) {
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *trackingWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}

func (w *trackingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Recover turns a panic into a 500. It wraps the whole mux because ogen's
// middleware surrounds only the handler call.
func Recover(logger *zap.Logger, production bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tw := &trackingWriter{ResponseWriter: w}

		defer func() {
			rec := recover()
			if rec == nil {
				return
			}

			if rec == http.ErrAbortHandler {
				panic(rec)
			}

			withTrace(r.Context(), logger).Error("panic recovered",
				zap.Any("panic", rec),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Bool("response_started", tw.wrote),
				zap.ByteString("stack", debug.Stack()),
			)

			// Too late to send a 500; drop the connection rather than append
			// an error object to a partial body.
			if tw.wrote {
				panic(http.ErrAbortHandler)
			}

			code := http.StatusInternalServerError
			writeError(tw, code, errorBody(code, fmt.Errorf("panic: %v", rec), production))
		}()

		next.ServeHTTP(tw, r)
	})
}
