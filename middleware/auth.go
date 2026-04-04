package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"grain_backend/config"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserContextKey contextKey = "user"

type UserClaims struct {
	Username    string `json:"username"`
	AccountType string `json:"accountType"`
	UserID      int    `json:"userId"`
	jwt.RegisteredClaims
}

// AuthenticateToken validates JWT token from Authorization header or cookie
func AuthenticateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for CORS preflight requests
		if r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		var tokenString string

		// Try to get token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			}
		}

		// If not in header, try cookie
		if tokenString == "" {
			cookie, err := r.Cookie("auth_token")
			if err == nil {
				tokenString = cookie.Value
			}
		}

		if tokenString == "" {
			http.Error(w, `{"message": "Access token is required"}`, http.StatusUnauthorized)
			return
		}

		// Parse and validate token
		claims := &UserClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(config.GetConfig().JWTSecret), nil
		})

		if err != nil || !token.Valid {
			http.Error(w, `{"message": "Invalid or expired token"}`, http.StatusForbidden)
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), UserContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GenerateToken creates a new JWT token for a user
func GenerateToken(username, accountType string, userID int) (string, error) {
	claims := &UserClaims{
		Username:    username,
		AccountType: accountType,
		UserID:      userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.GetConfig().JWTSecret))
}

// SetAuthCookie sets the JWT token in an HTTP-only cookie
func SetAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		MaxAge:   900, // 15 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}

// GetUserFromContext extracts user claims from context
func GetUserFromContext(r *http.Request) (*UserClaims, bool) {
	user, ok := r.Context().Value(UserContextKey).(*UserClaims)
	return user, ok
}
