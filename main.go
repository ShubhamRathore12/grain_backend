package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"grain_backend/config"
	"grain_backend/database"
	"grain_backend/handlers"
	"grain_backend/middleware"
	"grain_backend/websocket"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

func main() {
	// Get the directory where the executable is located
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	envPath := filepath.Join(dir, ".env")
	
	// Try to load .env from executable directory
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("⚠️  Could not load .env from %s: %v", envPath, err)
		log.Println("Trying current directory...")
		
		// Fallback to current directory
		if err := godotenv.Load(".env"); err != nil {
			log.Println("No .env file found in current directory, using environment variables")
		} else {
			log.Println("✅ .env file loaded from current directory")
		}
	} else {
		log.Printf("✅ .env file loaded successfully from: %s", envPath)
	}

	// NOW get config (after .env is loaded)
	cfg := config.GetConfig()

	// Initialize database
	log.Println("Starting server initialization...")
	if err := database.InitDatabase(); err != nil {
		log.Printf("⚠️  Database connection failed: %v", err)
		log.Println("Starting server without database connection...")
	}

	// Create WebSocket hub
	wsHub := websocket.NewWebSocketHub()
	go wsHub.Run()

	// Create router
	r := mux.NewRouter()

	// Apply middleware
	r.Use(middleware.EnableCORS)

	// Health check endpoint (public)
	r.HandleFunc("/api/health", handlers.HandleHealthCheck).Methods("GET", "OPTIONS")

	// Authentication routes (public)
	r.HandleFunc("/api/login", handlers.HandleLogin).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/register", handlers.HandleRegister).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/auth/login", handlers.HandleLogin).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/auth/register", handlers.HandleRegister).Methods("POST", "OPTIONS")

	// Protected API routes - all require valid auth_token cookie or Bearer token
	protected := r.PathPrefix("/api").Subrouter()
	protected.Use(middleware.AuthenticateToken)

	// Data routes
	protected.HandleFunc("/data/update", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Data updated successfully"}`))
	}).Methods("POST", "OPTIONS")

	// All data routes
	protected.HandleFunc("/alldata/alldata", handlers.HandleGetAllData).Methods("GET", "OPTIONS")

	// Smart200 data routes
	protected.HandleFunc("/api/all700data/getAllDataSmart200", handlers.HandleGetAllData).Methods("GET", "OPTIONS")
	protected.HandleFunc("/api/all700data/getAllData", handlers.HandleGetAllData).Methods("GET", "OPTIONS")
	protected.HandleFunc("/api/all700data/paginatedSmart200", handlers.HandleGetPaginatedData).Methods("GET", "OPTIONS")
	protected.HandleFunc("/api/all700data/paginatedSmart1200", handlers.HandleGetPaginatedData).Methods("GET", "OPTIONS")

	// Get all data smart200 route
	protected.HandleFunc("/api/getAllDataSmart200/", handlers.HandleGetAllData).Methods("GET", "OPTIONS")

	// Machine status routes
	protected.HandleFunc("/machine/status", handlers.HandleMachineStatus).Methods("GET", "OPTIONS")
	protected.HandleFunc("/machine/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true, "message": "Test endpoint working"}`))
	}).Methods("GET", "OPTIONS")
	protected.HandleFunc("/machine/diagnose", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true, "message": "Diagnose endpoint working"}`))
	}).Methods("GET", "OPTIONS")

	// Table route (public - no auth required for dashboard)
	r.HandleFunc("/api/table", handlers.HandleGetAllData).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/table/", handlers.HandleGetAllData).Methods("GET", "OPTIONS")

	// Reports routes
	protected.HandleFunc("/api/reports/", handlers.HandleReports).Methods("GET", "OPTIONS")
	protected.HandleFunc("/reports/health", handlers.HandleHealthCheck).Methods("GET", "OPTIONS")

	// Status route (public - no auth required)
	r.HandleFunc("/api/status-public", handlers.HandleMachineStatus).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/status-public/", handlers.HandleMachineStatus).Methods("GET", "OPTIONS")

	// Fault logs route
	protected.HandleFunc("/api/faultLogs/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true, "data": [], "message": "Fault logs endpoint"}`))
	}).Methods("GET", "OPTIONS")

	// Get active fault route
	protected.HandleFunc("/api/getActiveFault/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true, "data": [], "message": "Active fault endpoint"}`))
	}).Methods("GET", "OPTIONS")

	// WebSocket endpoint
	r.HandleFunc("/ws", wsHub.HandleWebSocket)

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("✅ Server running on port %s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	// Close database connection
	database.CloseDatabase()

	log.Println("Server stopped gracefully")
}
