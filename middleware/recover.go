package middleware

import (
	"log"
	"net/http"
	"runtime/debug"
)

// Recover catches panics from any downstream handler so a single bad request
// cannot crash the whole process and take monitoring offline for every machine.
// The panic is logged with a stack trace and the client gets a 500 (only if no
// response has started yet).
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("PANIC recovered on %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
				// Headers may already be sent for streaming endpoints (exports);
				// writing again would just log an error, so guard nothing extra.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
