package config

import (
	"os"
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
			JWTSecret:  getEnv("JWT_SECRET", "your-secret-key"),
			Port:       getEnv("PORT", "8080"),
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
