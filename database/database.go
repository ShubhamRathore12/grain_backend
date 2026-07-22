package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"grain_backend/config"

	_ "github.com/go-sql-driver/mysql"
)

var (
	db   *sql.DB
	once sync.Once
)

// InitDatabase initializes the database connection pool
func InitDatabase() error {
	var initErr error
	once.Do(func() {
		cfg := config.GetConfig()

		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&timeout=5s&readTimeout=15s&writeTimeout=15s",
			cfg.DBUser,
			cfg.DBPassword,
			cfg.DBHost,
			cfg.DBPort,
			cfg.DBName,
		)

		var err error
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			initErr = fmt.Errorf("failed to open database: %w", err)
			return
		}

		// Configure connection pool
		db.SetMaxOpenConns(config.MaxOpenConns)
		db.SetMaxIdleConns(config.MaxIdleConns)
		db.SetConnMaxLifetime(config.ConnMaxLifetime)
		db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

		// Test connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			initErr = fmt.Errorf("failed to ping database: %w", err)
			return
		}

		log.Println("✅ Database connection established")
	})

	return initErr
}

// GetDB returns the database instance
func GetDB() *sql.DB {
	if db == nil {
		log.Fatal("Database not initialized. Call InitDatabase() first.")
	}
	return db
}

// CloseDatabase closes the database connection
func CloseDatabase() {
	if db != nil {
		db.Close()
		log.Println("Database connection closed")
	}
}

// IsDatabaseConnected checks if the database is reachable
func IsDatabaseConnected(ctx context.Context) bool {
	if db == nil {
		return false
	}
	return db.PingContext(ctx) == nil
}

// Stats returns a snapshot of the database pool for health monitoring.
func Stats() sql.DBStats {
	if db == nil {
		return sql.DBStats{}
	}
	return db.Stats()
}

// SafeQuery executes a query with proper error handling and connection management
func SafeQuery(query string, args ...interface{}) (*sql.Rows, error) {
	return SafeQueryContext(context.Background(), query, args...)
}

// SafeQueryContext ties connection acquisition and query execution to the request.
func SafeQueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	startTime := time.Now()
	rows, err := db.QueryContext(ctx, query, args...)
	duration := time.Since(startTime)

	if err != nil {
		// Retry once on stale connection errors
		if isConnectionError(err) && ctx.Err() == nil {
			log.Printf("Stale database connection detected; retrying query")
			rows, err = db.QueryContext(ctx, query, args...)
			duration = time.Since(startTime)
			if err != nil {
				log.Printf("Query failed after retry (%v): %s", duration, err.Error())
				return nil, err
			}
			return rows, nil
		}
		log.Printf("Query failed (%v): %s", duration, err.Error())
		return nil, err
	}

	if duration >= time.Second {
		log.Printf("Slow query completed in %v", duration)
	}
	return rows, nil
}

// SafeQueryRow executes a query that returns a single row
func SafeQueryRow(query string, args ...interface{}) *sql.Row {
	return SafeQueryRowContext(context.Background(), query, args...)
}

// SafeQueryRowContext executes a single-row query tied to the request context.
func SafeQueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	if db == nil {
		return nil
	}
	return db.QueryRowContext(ctx, query, args...)
}

// SafeExec executes an insert/update/delete query
func SafeExec(query string, args ...interface{}) (sql.Result, error) {
	return SafeExecContext(context.Background(), query, args...)
}

// SafeExecContext executes a statement tied to the request context.
func SafeExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	startTime := time.Now()
	result, err := db.ExecContext(ctx, query, args...)
	duration := time.Since(startTime)

	if err != nil {
		// Retry once on stale connection errors
		if isConnectionError(err) && ctx.Err() == nil {
			log.Printf("Stale database connection detected; retrying exec")
			result, err = db.ExecContext(ctx, query, args...)
			duration = time.Since(startTime)
			if err != nil {
				log.Printf("Exec failed after retry (%v): %s", duration, err.Error())
				return nil, err
			}
			return result, nil
		}
		log.Printf("Exec failed (%v): %s", duration, err.Error())
		return nil, err
	}

	if duration >= time.Second {
		log.Printf("Slow exec completed in %v", duration)
	}
	return result, nil
}

// isConnectionError checks if the error is a stale/invalid connection error
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "invalid connection") ||
		strings.Contains(errMsg, "driver: bad connection") ||
		strings.Contains(errMsg, "unexpected eof") ||
		strings.Contains(errMsg, "connection refused")
}
