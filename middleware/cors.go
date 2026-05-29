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
		corsOrigin := os.Getenv("CORS_ORIGIN")
		allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")

		// Check if origin is allowed
		isAllowed := false

		// Check single CORS_ORIGIN
		if corsOrigin != "" && origin == corsOrigin {
			isAllowed = true
		}

		// Check multiple CORS_ALLOWED_ORIGINS (comma-separated)
		if !isAllowed && allowedOrigins != "" {
			origins := strings.Split(allowedOrigins, ",")
			for _, allowed := range origins {
				if strings.TrimSpace(allowed) == origin {
					isAllowed = true
					break
				}
			}
		}

		// Set CORS headers
		if isAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else if corsOrigin == "*" || allowedOrigins == "*" {
			// Only use wildcard if explicitly set (not recommended with credentials)
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition, Content-Type, X-Total-Count")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
