package middleware

import (
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

// EnableCORS enables Cross-Origin Resource Sharing
func EnableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Vary matters as soon as the allowed origin is computed per request:
		// without it a shared cache can hand one site's CORS headers to another.
		w.Header().Add("Vary", "Origin")

		// Only reflect origins that are actually on the allowlist. The previous
		// "allow it anyway" fallback reflected every origin alongside
		// Allow-Credentials: true, which let any website read authenticated
		// responses from a logged-in operator's browser.
		if origin != "" && IsOriginAllowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin, Cache-Control")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, Content-Type, X-Total-Count")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Handle preflight requests immediately
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// IsOriginAllowed checks if the given origin is in the allowed list. Exported so
// the WebSocket upgrade enforces the same allowlist as HTTP requests instead of
// accepting every origin.
func IsOriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}

	// Hardcoded allowed origins (always permitted)
	hardcodedOrigins := []string{
		"https://new-plc-software-5xyc.vercel.app",
		"http://localhost:3000",
		"http://localhost:3001",
		"http://localhost:5173",
	}

	for _, allowed := range hardcodedOrigins {
		if origin == allowed {
			return true
		}
	}

	// Env-configured origins. "*" is intentionally NOT honoured: this API always
	// answers with Access-Control-Allow-Credentials, and a credentialed
	// wildcard is both rejected by browsers and a cross-site read primitive.
	for _, key := range []string{"CORS_ORIGIN", "CORS_ALLOWED_ORIGINS"} {
		raw := os.Getenv(key)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		for _, allowed := range strings.Split(raw, ",") {
			allowed = strings.TrimSpace(allowed)
			if allowed == "" {
				continue
			}
			if allowed == "*" {
				warnWildcardOnce(key)
				continue
			}
			if allowed == origin {
				return true
			}
		}
	}

	return false
}

var wildcardWarned sync.Map

func warnWildcardOnce(key string) {
	if _, seen := wildcardWarned.LoadOrStore(key, true); !seen {
		log.Printf("⚠️  %s contains \"*\", which is ignored: a wildcard origin cannot be combined with credentialed requests. List the exact origins instead.", key)
	}
}
