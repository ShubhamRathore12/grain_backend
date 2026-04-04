package database

import (
	"database/sql"
	"fmt"
	"log"
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
		
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&timeout=30s",
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
		if err := db.Ping(); err != nil {
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
func IsDatabaseConnected() bool {
	if db == nil {
		return false
	}
	return db.Ping() == nil
}

// ensureConnection pings the database and reconnects if the connection is stale
func ensureConnection() error {
	if db == nil {
		return fmt.Errorf("database not connected")
	}
	if err := db.Ping(); err != nil {
		log.Printf("⚠️ Database ping failed: %v, connection will be refreshed by pool", err)
	}
	return nil
}

// SafeQuery executes a query with proper error handling and connection management
func SafeQuery(query string, args ...interface{}) (*sql.Rows, error) {
	if db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	startTime := time.Now()
	rows, err := db.Query(query, args...)
	duration := time.Since(startTime)

	if err != nil {
		// Retry once on stale connection errors
		if isConnectionError(err) {
			log.Printf("⚠️ Stale connection detected, retrying query...")
			ensureConnection()
			rows, err = db.Query(query, args...)
			duration = time.Since(startTime)
			if err != nil {
				log.Printf("❌ Query error after retry (%v): %s", duration, err.Error())
				return nil, err
			}
			log.Printf("✓ Query executed in %v (after retry)", duration)
			return rows, nil
		}
		log.Printf("❌ Query error (%v): %s", duration, err.Error())
		return nil, err
	}

	log.Printf("✓ Query executed in %v", duration)
	return rows, nil
}

// SafeQueryRow executes a query that returns a single row
func SafeQueryRow(query string, args ...interface{}) *sql.Row {
	if db == nil {
		return nil
	}
	ensureConnection()
	return db.QueryRow(query, args...)
}

// SafeExec executes an insert/update/delete query
func SafeExec(query string, args ...interface{}) (sql.Result, error) {
	if db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	startTime := time.Now()
	result, err := db.Exec(query, args...)
	duration := time.Since(startTime)

	if err != nil {
		// Retry once on stale connection errors
		if isConnectionError(err) {
			log.Printf("⚠️ Stale connection detected, retrying exec...")
			ensureConnection()
			result, err = db.Exec(query, args...)
			duration = time.Since(startTime)
			if err != nil {
				log.Printf("❌ Exec error after retry (%v): %s", duration, err.Error())
				return nil, err
			}
			log.Printf("✓ Exec executed in %v (after retry)", duration)
			return result, nil
		}
		log.Printf("❌ Exec error (%v): %s", duration, err.Error())
		return nil, err
	}

	log.Printf("✓ Exec executed in %v", duration)
	return result, nil
}

// isConnectionError checks if the error is a stale/invalid connection error
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return errMsg == "invalid connection" ||
		errMsg == "driver: bad connection" ||
		errMsg == "unexpected EOF" ||
		errMsg == "connection refused"
}
