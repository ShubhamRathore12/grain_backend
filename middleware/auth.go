package middleware

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"grain_backend/config"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserContextKey contextKey = "user"

// authCookieName is the only cookie the API treats as a credential.
const authCookieName = "auth_token"

type UserClaims struct {
	Username    string `json:"username"`
	AccountType string `json:"accountType"`
	UserID      int    `json:"userId"`
	jwt.RegisteredClaims
}

// -----------------------------------------------------------------------------
// Session revocation (S-05)
//
// A stateless JWT cannot be "logged out" — it stays valid until it expires. Two
// small in-memory registries close that gap:
//
//	revokedTokens : single token IDs killed by an explicit logout
//	userCutoffs   : per-user "no token issued before this instant is valid"
//	                markers, used by password change and log-out-everywhere
//
// Both are bounded by token TTL, so memory stays flat. A multi-instance
// deployment needs this state in Redis or the database instead; see
// SECURITY_REMEDIATION_BACKEND.md.
// -----------------------------------------------------------------------------

var (
	revokedTokens sync.Map // jti (string) -> expiry (time.Time)
	userCutoffs   sync.Map // userID (int)  -> cutoff (time.Time)
	janitorOnce   sync.Once
)

func startRevocationJanitor() {
	janitorOnce.Do(func() {
		go func() {
			for {
				time.Sleep(5 * time.Minute)
				now := time.Now()
				revokedTokens.Range(func(k, v any) bool {
					if exp, ok := v.(time.Time); ok && now.After(exp) {
						revokedTokens.Delete(k)
					}
					return true
				})
				userCutoffs.Range(func(k, v any) bool {
					// A cutoff is only meaningful while tokens older than it can
					// still be presented.
					if cut, ok := v.(time.Time); ok && now.After(cut.Add(24*time.Hour)) {
						userCutoffs.Delete(k)
					}
					return true
				})
			}
		}()
	})
}

// RevokeToken invalidates one specific token, identified by its jti claim.
func RevokeToken(tokenID string, expiresAt time.Time) {
	if tokenID == "" {
		return
	}
	startRevocationJanitor()
	revokedTokens.Store(tokenID, expiresAt)
}

// RevokeAllSessionsForUser invalidates every token already issued to a user.
// Called on password change and on an explicit "log out of all sessions".
func RevokeAllSessionsForUser(userID int) {
	if userID <= 0 {
		return
	}
	startRevocationJanitor()
	userCutoffs.Store(userID, time.Now())
}

func isTokenRevoked(claims *UserClaims) bool {
	if claims.ID != "" {
		if _, found := revokedTokens.Load(claims.ID); found {
			return true
		}
	}
	if cutoff, found := userCutoffs.Load(claims.UserID); found {
		cut, ok := cutoff.(time.Time)
		if ok && claims.IssuedAt != nil && !claims.IssuedAt.Time.After(cut) {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Token verification
// -----------------------------------------------------------------------------

// writeAuthError returns a stable, non-descriptive body. The client only needs
// to know it must authenticate again; anything more helps an attacker tell
// "no token" from "expired token" from "revoked token" (S-07).
func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": message,
	})
}

// extractToken pulls the bearer token from, in order: the Authorization header,
// the auth cookie, and the WebSocket subprotocol handshake. Browsers cannot set
// headers on a WebSocket upgrade, so /ws is allowed to carry the token in
// Sec-WebSocket-Protocol ("bearer, <token>"). Query-string tokens are
// deliberately not accepted: they leak into access logs, referrers and history.
func extractToken(r *http.Request) string {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			if token := strings.TrimSpace(parts[1]); token != "" {
				return token
			}
		}
	}

	if cookie, err := r.Cookie(authCookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	if proto := r.Header.Get("Sec-WebSocket-Protocol"); proto != "" {
		parts := strings.Split(proto, ",")
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	return ""
}

// ParseToken verifies a token's signature, algorithm, expiry and revocation
// state. Exported so non-middleware entry points (the WebSocket upgrade) can
// apply exactly the same rules.
func ParseToken(tokenString string) (*UserClaims, error) {
	claims := &UserClaims{}
	// Pinning the algorithm blocks alg-confusion attacks ("alg": "none" and
	// HS256/RS256 substitution).
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(t *jwt.Token) (interface{}, error) {
			return []byte(config.GetConfig().JWTSecret), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	if isTokenRevoked(claims) {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

// AuthenticateToken rejects every request that does not carry a valid session.
// It is the default for all data, export, command and user routes: deny first,
// allow only what is explicitly public (S-01).
func AuthenticateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Preflight carries no credentials by design. The CORS layer answers
		// OPTIONS before this point; this is belt-and-braces.
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		// Authenticated responses must never be replayable from the browser
		// cache after logout (S-05).
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
		w.Header().Set("Pragma", "no-cache")

		tokenString := extractToken(r)
		if tokenString == "" {
			writeAuthError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		claims, err := ParseToken(tokenString)
		if err != nil {
			// 401, not 403: the credential is missing or no longer valid, so the
			// client should re-authenticate. The old 403 made clients treat an
			// expired session as a permanent permission error.
			writeAuthError(w, http.StatusUnauthorized, "Session expired or invalid. Please sign in again.")
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// -----------------------------------------------------------------------------
// Token issuing
// -----------------------------------------------------------------------------

func newTokenID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// Fall back to a time-based value rather than issuing an empty jti,
		// which would make the token non-revocable.
		log.Printf("⚠️  crypto/rand unavailable for token id: %v", err)
		return "t" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// IssuedToken is everything the caller needs to set a cookie and, later, revoke.
type IssuedToken struct {
	Token     string
	TokenID   string
	ExpiresAt time.Time
}

// GenerateToken creates a signed, revocable access token for a user.
func GenerateToken(username, accountType string, userID int) (IssuedToken, error) {
	cfg := config.GetConfig()
	now := time.Now()
	expiresAt := now.Add(cfg.AccessTokenTTL)
	tokenID := newTokenID()

	claims := &UserClaims{
		Username:    username,
		AccountType: accountType,
		UserID:      userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID,
			Subject:   username,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return IssuedToken{}, err
	}
	return IssuedToken{Token: signed, TokenID: tokenID, ExpiresAt: expiresAt}, nil
}

// -----------------------------------------------------------------------------
// Cookies
// -----------------------------------------------------------------------------

func sameSiteMode(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteLaxMode
	}
}

func baseAuthCookie() *http.Cookie {
	cfg := config.GetConfig()
	return &http.Cookie{
		Name:     authCookieName,
		Path:     "/",
		Domain:   cfg.CookieDomain,
		HttpOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: sameSiteMode(cfg.CookieSameSite),
	}
}

// SetAuthCookie stores the token in an HttpOnly cookie whose lifetime matches
// the token's, so the browser stops sending a credential the server would
// reject anyway.
func SetAuthCookie(w http.ResponseWriter, issued IssuedToken) {
	cookie := baseAuthCookie()
	cookie.Value = issued.Token
	cookie.Expires = issued.ExpiresAt
	cookie.MaxAge = int(time.Until(issued.ExpiresAt).Seconds())
	http.SetCookie(w, cookie)
}

// ClearAuthCookie expires the cookie using the same Domain, Path, Secure and
// SameSite attributes it was created with. A mismatch on any of them leaves the
// original cookie in place and logout silently fails (S-05).
func ClearAuthCookie(w http.ResponseWriter) {
	cookie := baseAuthCookie()
	cookie.Value = ""
	cookie.Expires = time.Unix(0, 0)
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
}

// GetUserFromContext extracts user claims from context.
func GetUserFromContext(r *http.Request) (*UserClaims, bool) {
	user, ok := r.Context().Value(UserContextKey).(*UserClaims)
	return user, ok
}

// OptionalAuth attaches claims when a valid token is present but never rejects
// the request.
//
// Logout needs this: a client whose token already expired must still be able to
// clear its cookie. Requiring auth there would answer 401 and leave the browser
// holding a stale credential.
func OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")

		if tokenString := extractToken(r); tokenString != "" {
			if claims, err := ParseToken(tokenString); err == nil {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), UserContextKey, claims)))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
