package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	JWTSecret  string
	Port       string

	// Session / cookie hardening (S-01, S-05).
	// The known deployment serves the UI from a different site than the API
	// (vercel.app -> primeosys.com), so the auth cookie must be SameSite=None
	// with Secure set or the browser will drop it on API and WebSocket calls.
	CookieSecure   bool
	CookieSameSite string // "none" | "lax" | "strict"
	CookieDomain   string
	AccessTokenTTL time.Duration

	// EnforceMachineScope gates per-user machine authorization (S-02, S-06).
	// Keep it on. It exists only as a break-glass switch for the rollout window
	// in which monitorAccess is still being populated for existing accounts.
	EnforceMachineScope bool

	// AllowPublicRegister keeps /api/register reachable without an admin session.
	// Off by default: self-service account creation on an industrial control
	// surface is an access-control hole, not a feature.
	AllowPublicRegister bool
}

var (
	config *Config
	once   sync.Once
)

func GetConfig() *Config {
	once.Do(func() {
		config = &Config{
			DBHost:     getEnv("DB_HOST", "localhost"),
			DBPort:     getEnv("DB_PORT", "3306"),
			DBUser:     getEnv("DB_USER", "root"),
			DBPassword: getEnv("DB_PASSWORD", ""),
			DBName:     getEnv("DB_NAME", "grain_db"),
			JWTSecret:  loadJWTSecret(),
			Port:       getEnv("PORT", "8080"),

			CookieSecure:   getEnvBool("COOKIE_SECURE", true),
			CookieSameSite: strings.ToLower(getEnv("COOKIE_SAMESITE", "none")),
			CookieDomain:   getEnv("COOKIE_DOMAIN", ""),
			AccessTokenTTL: getEnvDuration("ACCESS_TOKEN_TTL", 30*time.Minute),

			EnforceMachineScope: getEnvBool("MACHINE_SCOPE_ENFORCE", true),
			AllowPublicRegister: getEnvBool("ALLOW_PUBLIC_REGISTER", false),
		}

		if !config.CookieSecure {
			log.Printf("⚠️  COOKIE_SECURE=false — auth cookies will be sent over plain HTTP. Local development only.")
		}
		if config.CookieSameSite == "none" && !config.CookieSecure {
			// Browsers reject SameSite=None without Secure, which silently breaks login.
			log.Printf("⚠️  COOKIE_SAMESITE=none requires COOKIE_SECURE=true; falling back to lax.")
			config.CookieSameSite = "lax"
		}
		if !config.EnforceMachineScope {
			log.Printf("⚠️  MACHINE_SCOPE_ENFORCE=false — per-user machine authorization is DISABLED. Do not run this way in production.")
		}
		if config.AllowPublicRegister {
			log.Printf("⚠️  ALLOW_PUBLIC_REGISTER=true — anyone can create an account.")
		}
	})
	return config
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvBool(key string, defaultValue bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		log.Printf("⚠️  %s=%q is not a boolean; using %v", key, raw, defaultValue)
		return defaultValue
	}
	return parsed
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		log.Printf("⚠️  %s=%q is not a valid duration; using %s", key, raw, defaultValue)
		return defaultValue
	}
	return parsed
}

// Database connection pool configuration.
// MySQL server allows max 25 connections total. Keep the app pool below that so
// admin/root and any other client always have connections available — a pool at
// or above the server limit causes "Too many connections" errors under load.
// Internal concurrency caps (status refresh: 8, exports: 3) fit inside this pool.
const (
	MaxOpenConns    = 15
	MaxIdleConns    = 5
	ConnMaxLifetime = 5 * time.Minute
	ConnMaxIdleTime = 3 * time.Minute
)

// -----------------------------------------------------------------------------
// JWT signing secret
//
// Every session in the system is only as strong as this value: anyone holding it
// can mint a token for any user, which makes it the single credential that
// bypasses both the authentication and the machine-authorization checks.
//
// So it is validated at startup rather than trusted:
//   - it can be supplied out-of-band via JWT_SECRET_FILE (Docker/K8s secret,
//     mounted file) instead of an environment variable that shows up in
//     `docker inspect`, crash dumps and process listings,
//   - placeholder and previously-leaked values are rejected by name,
//   - a short or low-entropy value is rejected,
//   - and failure is fatal. Booting with a guessable key would look healthy
//     while leaving the API effectively unauthenticated.
// -----------------------------------------------------------------------------

// rejectedJWTSecrets are values that must never sign a token again: framework
// placeholders, and the hand-typed secret that shipped in this repository's
// committed .env (and therefore exists in git history and every clone).
var rejectedJWTSecrets = map[string]bool{
	"your-secret-key":                                  true,
	"your-secret-key-change-this":                      true,
	"secret":                                           true,
	"changeme":                                         true,
	"jwt-secret":                                       true,
	"mysecret":                                         true,
	"test":                                             true,
	"21321esaendjnasjdasjdnjasbndjqwbeijn2jebjbjbjnjnj": true,
}

const minJWTSecretLength = 32

// loadJWTSecret resolves the secret from JWT_SECRET_FILE, then JWT_SECRET, and
// validates it. It terminates the process rather than returning an unusable key.
func loadJWTSecret() string {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))

	// A mounted file wins: it keeps the secret out of the environment block.
	if path := strings.TrimSpace(os.Getenv("JWT_SECRET_FILE")); path != "" {
		contents, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("❌ JWT_SECRET_FILE=%s could not be read: %v", path, err)
		}
		secret = strings.TrimSpace(string(contents))
		if secret == "" {
			log.Fatalf("❌ JWT_SECRET_FILE=%s is empty.", path)
		}
	}

	switch {
	case secret == "":
		log.Fatal("❌ JWT_SECRET is not set. Generate one with `openssl rand -base64 48` " +
			"and set JWT_SECRET (or JWT_SECRET_FILE). Refusing to start without a signing key.")

	case rejectedJWTSecrets[strings.ToLower(secret)]:
		log.Fatal("❌ JWT_SECRET is a known placeholder or a previously leaked value. " +
			"Generate a fresh one with `openssl rand -base64 48`. Refusing to start.")

	case len(secret) < minJWTSecretLength:
		log.Fatalf("❌ JWT_SECRET is %d characters; at least %d are required. "+
			"Generate one with `openssl rand -base64 48`. Refusing to start.",
			len(secret), minJWTSecretLength)

	case distinctRunes(secret) < 12:
		// Catches padded junk like "aaaa...a" that passes the length check but
		// carries almost no entropy.
		log.Fatal("❌ JWT_SECRET has too little variation to be random. " +
			"Generate one with `openssl rand -base64 48`. Refusing to start.")
	}

	return secret
}

func distinctRunes(s string) int {
	seen := make(map[rune]struct{}, len(s))
	for _, c := range s {
		seen[c] = struct{}{}
	}
	return len(seen)
}
