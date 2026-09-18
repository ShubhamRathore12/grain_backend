package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
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

	// =========================================================================
	// PUBLIC ROUTES
	//
	// Deny by default: this list is the entire public surface of the API. Every
	// other route lives on the authenticated subrouter below.
	//
	// Machine telemetry, reports, fault history, exports and the live socket used
	// to be reachable with no credential at all, which is what made a direct URL
	// or a hand-written /api/table request enough to read another customer's
	// chiller (S-01, S-06). They are all authenticated now.
	// =========================================================================

	// Liveness probe. Reports process/database health only, never machine data.
	r.HandleFunc("/api/health", handlers.HandleHealthCheck).Methods("GET", "OPTIONS")
	r.HandleFunc("/api/reports/health", handlers.HandleHealthCheck).Methods("GET", "OPTIONS")

	// Credential endpoints. Rate limited inside the handler.
	r.HandleFunc("/api/login", handlers.HandleLogin).Methods("POST", "OPTIONS")
	r.HandleFunc("/api/auth/login", handlers.HandleLogin).Methods("POST", "OPTIONS")

	// Logout is public so a request carrying an already-expired token still
	// clears the cookie instead of returning 401. The handler revokes the session
	// server-side when a valid token is present.
	r.Handle("/api/logout", middleware.OptionalAuth(http.HandlerFunc(handlers.HandleLogout))).Methods("POST", "OPTIONS")
	r.Handle("/api/auth/logout", middleware.OptionalAuth(http.HandlerFunc(handlers.HandleLogout))).Methods("POST", "OPTIONS")

	// Account creation is admin-only unless explicitly opened up. Registered
	// here so the closed case can answer 404 rather than advertising the route.
	if cfg.AllowPublicRegister {
		r.HandleFunc("/api/register", handlers.HandleRegister).Methods("POST", "OPTIONS")
		r.HandleFunc("/api/auth/register", handlers.HandleRegister).Methods("POST", "OPTIONS")
	}

	// =========================================================================
	// PROTECTED ROUTES (valid session required)
	//
	// AuthenticateToken answers 401 for a missing, expired or revoked token.
	// Handlers that take a `table` parameter then run it through resolveTable,
	// which confirms the caller's monitorAccess grant covers that machine and
	// answers 403 for anything else — unknown and unauthorized look identical
	// (S-02, S-07).
	// =========================================================================
	protected := r.PathPrefix("/api").Subrouter()
	protected.Use(middleware.AuthenticateToken)

	// Session introspection and self-service credential management (S-03, S-04).
	protected.HandleFunc("/auth/session", handlers.HandleSession).Methods("GET", "OPTIONS")
	protected.HandleFunc("/auth/me", handlers.HandleSession).Methods("GET", "OPTIONS")
	protected.HandleFunc("/auth/change-password", handlers.HandleChangePassword).Methods("POST", "PUT", "OPTIONS")
	protected.HandleFunc("/profile/password", handlers.HandleChangePassword).Methods("POST", "PUT", "OPTIONS")

	// Account creation for administrators. HandleRegister stores the issued
	// credential as an INIT: hash, so the new owner must replace it.
	if !cfg.AllowPublicRegister {
		protected.HandleFunc("/register", handlers.RequireAdmin(handlers.HandleRegister)).Methods("POST", "OPTIONS")
		protected.HandleFunc("/auth/register", handlers.RequireAdmin(handlers.HandleRegister)).Methods("POST", "OPTIONS")
	}

	// Machine status. The response is filtered to the caller's assigned machines,
	// so the device list can no longer be used to enumerate the fleet (S-02).
	// "status-public" keeps its path for client compatibility but is not public.
	protected.HandleFunc("/status-public", handlers.HandleMachineStatus).Methods("GET", "OPTIONS")
	protected.HandleFunc("/status-public/", handlers.HandleMachineStatus).Methods("GET", "OPTIONS")
	protected.HandleFunc("/machine/status", handlers.HandleMachineStatus).Methods("GET", "OPTIONS")
	protected.HandleFunc("/machine/status-public", handlers.HandleMachineStatus).Methods("GET", "OPTIONS")

	// Latest-row table data.
	protected.HandleFunc("/table", handlers.HandleGetAllData).Methods("GET", "OPTIONS")
	protected.HandleFunc("/table/", handlers.HandleGetAllData).Methods("GET", "OPTIONS")

	// Reports.
	protected.HandleFunc("/reports", handlers.HandleReports).Methods("GET", "OPTIONS")
	protected.HandleFunc("/reports/", handlers.HandleReports).Methods("GET", "OPTIONS")

	// Exports. Specific Excel paths must be registered before the generic ones.
	protected.HandleFunc("/export/excel", handlers.HandleExportExcel).Methods("GET", "OPTIONS")
	protected.HandleFunc("/export/excel/", handlers.HandleExportExcel).Methods("GET", "OPTIONS")
	protected.HandleFunc("/export", handlers.HandleExportCSV).Methods("GET", "OPTIONS")
	protected.HandleFunc("/export/", handlers.HandleExportCSV).Methods("GET", "OPTIONS")

	// Bulk/paginated data.
	protected.HandleFunc("/alldata/alldata", handlers.HandleGetAllData).Methods("GET", "OPTIONS")
	protected.HandleFunc("/alldata/alldata/", handlers.HandleGetAllData).Methods("GET", "OPTIONS")
	protected.HandleFunc("/all700data/getAllDataSmart200", handlers.HandleGetAllData).Methods("GET", "OPTIONS")
	protected.HandleFunc("/all700data/getAllData", handlers.HandleGetAllData).Methods("GET", "OPTIONS")
	protected.HandleFunc("/all700data/paginatedSmart200", handlers.HandleGetPaginatedData).Methods("GET", "OPTIONS")
	protected.HandleFunc("/all700data/paginatedSmart1200", handlers.HandleGetPaginatedData).Methods("GET", "OPTIONS")
	protected.HandleFunc("/getAllDataSmart200", handlers.HandleGetAllData).Methods("GET", "OPTIONS")
	protected.HandleFunc("/getAllDataSmart200/", handlers.HandleGetAllData).Methods("GET", "OPTIONS")

	// Fault history (today back 2 months by default).
	protected.HandleFunc("/faultLogs", handlers.HandleGetFaultHistory).Methods("GET", "OPTIONS")
	protected.HandleFunc("/faultLogs/", handlers.HandleGetFaultHistory).Methods("GET", "OPTIONS")
	protected.HandleFunc("/fault/history", handlers.HandleGetFaultHistory).Methods("GET", "OPTIONS")
	protected.HandleFunc("/fault/history/", handlers.HandleGetFaultHistory).Methods("GET", "OPTIONS")

	// Today's faults only.
	protected.HandleFunc("/fault/today", handlers.HandleGetTodaysFaults).Methods("GET", "OPTIONS")
	protected.HandleFunc("/fault/today/", handlers.HandleGetTodaysFaults).Methods("GET", "OPTIONS")
	protected.HandleFunc("/todaysFaults", handlers.HandleGetTodaysFaults).Methods("GET", "OPTIONS")
	protected.HandleFunc("/todaysFaults/", handlers.HandleGetTodaysFaults).Methods("GET", "OPTIONS")

	// Active fault placeholder. Still authorization-checked so it cannot become a
	// probe for which machine identifiers are valid once it returns real data.
	activeFault := func(w http.ResponseWriter, req *http.Request) {
		if _, ok := handlers.AuthorizeTableRequest(w, req); !ok {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true, "data": [], "message": "Active fault endpoint"}`))
	}
	protected.HandleFunc("/getActiveFault", activeFault).Methods("GET", "OPTIONS")
	protected.HandleFunc("/getActiveFault/", activeFault).Methods("GET", "OPTIONS")

	// Data write.
	protected.HandleFunc("/data/update", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Data updated successfully"}`))
	}).Methods("POST", "OPTIONS")

	// Machine test/diagnose. Admin-gated: these are diagnostic surfaces, not
	// operator features.
	protected.HandleFunc("/machine/test", handlers.RequireAdmin(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true, "message": "Test endpoint working"}`))
	})).Methods("GET", "OPTIONS")
	protected.HandleFunc("/machine/diagnose", handlers.RequireAdmin(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true, "message": "Diagnose endpoint working"}`))
	})).Methods("GET", "OPTIONS")

	// User management. Handlers additionally restrict access to the usernames in
	// defaultUserAdmins / USER_ADMIN_USERNAMES.
	protected.HandleFunc("/users", handlers.HandleListUsers).Methods("GET", "OPTIONS")
	protected.HandleFunc("/users/", handlers.HandleListUsers).Methods("GET", "OPTIONS")
	protected.HandleFunc("/users/{id:[0-9]+}", handlers.HandleUpdateUser).Methods("PUT", "PATCH", "OPTIONS")
	protected.HandleFunc("/users/{id:[0-9]+}", handlers.HandleDeleteUser).Methods("DELETE", "OPTIONS")

	// Live telemetry socket. Authenticated like every other route: the upgrade
	// itself is refused without a session, and the origin is checked against the
	// CORS allowlist inside the hub.
	r.Handle("/ws", middleware.AuthenticateToken(http.HandlerFunc(wsHub.HandleWebSocket)))

	// Wrap the entire router so headers are set on EVERY response, including
	// 404/405 and preflight OPTIONS that never match a route (gorilla/mux's
	// r.Use() middleware does NOT run on unmatched routes).
	// Recover is outermost so a panic in ANY handler or middleware is caught and
	// logged instead of crashing the process.
	handler := middleware.Recover(
		middleware.SecurityHeaders(
			requestDeadlineMiddleware(
				middleware.EnableCORS(r))))

	// Create HTTP server
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
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

// requestDeadlineMiddleware ensures abandoned or overloaded requests release
// their database work. Exports receive a longer deadline because they stream
// table data in batches.
func requestDeadlineMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timeout := 30 * time.Second
		if strings.HasPrefix(r.URL.Path, "/api/export") {
			timeout = 4 * time.Minute
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
