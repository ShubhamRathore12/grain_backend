package middleware

import (
	"net/http"
	"os"
)

// EnableCORS enables Cross-Origin Resource Sharing
func EnableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get CORS origin from environment or use wildcard
		corsOrigin := os.Getenv("CORS_ORIGIN")
		origin := r.Header.Get("Origin")

		if corsOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", corsOrigin)
		} else if origin != "" {
			// Reflect the requesting origin to support credentials
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
