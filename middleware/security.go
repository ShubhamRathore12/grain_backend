package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SecurityHeaders applies baseline response hardening to every request.
//
// The API serves JSON and file downloads only, so a restrictive policy costs
// nothing here. no-store on /api paths keeps authenticated payloads out of the
// browser's back/forward cache, which is part of making logout actually stick
// (S-05).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Resource-Policy", "same-site")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}

		next.ServeHTTP(w, r)
	})
}

// -----------------------------------------------------------------------------
// Rate limiting
//
// A fixed-window counter keyed by client IP plus a scope label. Enough to blunt
// credential stuffing and identifier enumeration (S-03, S-06, S-07) without
// pulling in a dependency. Single-process only: behind more than one instance,
// move this to Redis or the load balancer.
// -----------------------------------------------------------------------------

type rateWindow struct {
	count int
	reset time.Time
}

var (
	rateBuckets   = make(map[string]*rateWindow)
	rateBucketsMu sync.Mutex
	rateSweptAt   time.Time
)

// ClientIP resolves the caller address, honouring X-Forwarded-For only for the
// left-most entry. Behind nginx or Render this is the real client; with no proxy
// it falls back to the socket address.
func ClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first := strings.TrimSpace(strings.Split(fwd, ",")[0]); first != "" {
			return first
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// AllowRequest records a hit against "scope:key" and reports whether it stays
// inside the limit. retryAfter is the seconds remaining in the current window.
func AllowRequest(scope, key string, limit int, window time.Duration) (allowed bool, retryAfter int) {
	bucketKey := scope + ":" + key
	now := time.Now()

	rateBucketsMu.Lock()
	defer rateBucketsMu.Unlock()

	// Opportunistic sweep so abandoned keys cannot grow the map without bound.
	if now.Sub(rateSweptAt) > 10*time.Minute {
		for k, v := range rateBuckets {
			if now.After(v.reset) {
				delete(rateBuckets, k)
			}
		}
		rateSweptAt = now
	}

	bucket, found := rateBuckets[bucketKey]
	if !found || now.After(bucket.reset) {
		rateBuckets[bucketKey] = &rateWindow{count: 1, reset: now.Add(window)}
		return true, 0
	}

	bucket.count++
	if bucket.count > limit {
		return false, int(time.Until(bucket.reset).Seconds()) + 1
	}
	return true, 0
}

// WriteRateLimited emits a 429 with Retry-After and no detail about the limit
// that was hit.
func WriteRateLimited(w http.ResponseWriter, retryAfter int) {
	if retryAfter < 1 {
		retryAfter = 1
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": "Too many requests. Please try again later.",
	})
}
