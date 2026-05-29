package middleware

import (
	"net/http"
	"os"
	"strings"
)

// EnableCORS enables Cross-Origin Resource Sharing
func EnableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Determine if origin is allowed
		allowed := isOriginAllowed(origin)

		// Set CORS headers
		if allowed && origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else if origin != "" {
			// Fallback: allow the origin anyway for now (remove this in production hardening)
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin, Cache-Control")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, Content-Type, X-Total-Count")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Handle preflight requests immediately
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isOriginAllowed checks if the given origin is in the allowed list
func isOriginAllowed(origin string) bool {
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

	// Check CORS_ORIGIN env var
	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin != "" {
		if corsOrigin == "*" {
			return true
		}
		if strings.TrimSpace(corsOrigin) == origin {
			return true
		}
	}

	// Check CORS_ALLOWED_ORIGINS env var (comma-separated)
	allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	if allowedOrigins != "" {
		if allowedOrigins == "*" {
			return true
		}
		origins := strings.Split(allowedOrigins, ",")
		for _, allowed := range origins {
			if strings.TrimSpace(allowed) == origin {
				return true
			}
		}
	}

	return false
}
