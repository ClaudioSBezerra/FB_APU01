package main

// Force rebuild: 2026-02-19 - Version 5.6.0 - RFB Integration + Stability Fixes
import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"fb_apu01/handlers"
	"fb_apu01/worker"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// Version information for backend deployment validation
const (
	BackendVersion = "5.5.0"
	FeatureSet     = "Z.AI GLM Integration, AI Executive Reports, Multipart Email, Tax Reform Projection, Simples Nacional Dashboard"
)

func GetVersionInfo() string {
	return fmt.Sprintf("Backend Version: %s | Features: %s", BackendVersion, FeatureSet)
}

func PrintVersion() {
	fmt.Println(GetVersionInfo())
}

type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Service   string `json:"service"`
	Version   string `json:"version"`
	Features  string `json:"features"`
	Database  string `json:"database"`
	DBError   string `json:"db_error,omitempty"`
}

var (
	db      *sql.DB
	dbMutex sync.RWMutex
	dbErr   error
)

func getDB() *sql.DB {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	return db
}

func initDBAsync() {
	go func() {
		var conn *sql.DB
		var err error
		connStr := os.Getenv("DATABASE_URL")
		if connStr == "" {
			// Fallback for local development
			connStr = "postgres://postgres:postgres@localhost:5432/fiscal_db?sslmode=disable"
			fmt.Println("DATABASE_URL not set, using default local connection:", connStr)
		}

		// Retry logic for database connection (Infinite loop until success)
		attempt := 0
		for {
			attempt++
			conn, err = sql.Open("postgres", connStr)
			if err == nil {
				err = conn.Ping()
				if err == nil {
					// Configure Connection Pool
					// MaxIdleConns=10 keeps connections warm for workers
					// ConnMaxLifetime=30min prevents overnight stale connection kills
					conn.SetMaxOpenConns(25)
					conn.SetMaxIdleConns(10)
					conn.SetConnMaxLifetime(30 * time.Minute)

					dbMutex.Lock()
					db = conn
					dbErr = nil
					dbMutex.Unlock()

					fmt.Println("Successfully connected to the database!")

					// Initialize components that depend on DB
					onDBConnected()
					return
				}
			}

			dbMutex.Lock()
			dbErr = fmt.Errorf("attempt %d: %v", attempt, err)
			dbMutex.Unlock()

			fmt.Printf("Failed to connect to database (attempt %d): %v. Retrying in 5s...\n", attempt, err)
			time.Sleep(5 * time.Second)
		}
	}()
}

func onDBConnected() {
	// Execute migrations and other initialization tasks here
	// This function is called once DB is connected
	database := getDB()

	// Execute migrations
	migrationDir := "migrations"
	if _, err := os.Stat(migrationDir); os.IsNotExist(err) {
		// Try backend/migrations if running from root
		if _, err := os.Stat("backend/migrations"); err == nil {
			migrationDir = "backend/migrations"
		}
	}

	fmt.Printf("Looking for migrations in: %s\n", migrationDir)
	files, err := filepath.Glob(filepath.Join(migrationDir, "*.sql"))
	if err != nil {
		log.Printf("Error finding migration files: %v", err)
	} else {
		// Ensure schema_migrations table exists with correct column name
		var tableExists bool
		_ = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='schema_migrations')`).Scan(&tableExists)

		if !tableExists {
			_, err = database.Exec(`CREATE TABLE schema_migrations (
				filename VARCHAR(255) PRIMARY KEY,
				executed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
			)`)
			if err != nil {
				log.Printf("Warning: Failed to create schema_migrations table: %v", err)
			}
		} else {
			// Table exists — ensure 'filename' column exists with correct type
			var hasFilename bool
			_ = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='schema_migrations' AND column_name='filename')`).Scan(&hasFilename)
			if !hasFilename {
				// Find the first column and rename it to 'filename'
				var oldCol string
				_ = database.QueryRow(`SELECT column_name FROM information_schema.columns WHERE table_name='schema_migrations' ORDER BY ordinal_position LIMIT 1`).Scan(&oldCol)
				if oldCol != "" {
					log.Printf("Renaming schema_migrations column '%s' → 'filename'", oldCol)
					_, renameErr := database.Exec(fmt.Sprintf(`ALTER TABLE schema_migrations RENAME COLUMN %s TO filename`, oldCol))
					if renameErr != nil {
						log.Printf("ERROR: Failed to rename column: %v. Recreating table.", renameErr)
					}
				}
			}

			// Ensure 'filename' column is VARCHAR (legacy DBs may have integer type)
			var colType string
			_ = database.QueryRow(`SELECT data_type FROM information_schema.columns WHERE table_name='schema_migrations' AND column_name='filename'`).Scan(&colType)
			if colType != "" && colType != "character varying" && colType != "text" {
				log.Printf("schema_migrations.filename is type '%s', converting to VARCHAR(255)", colType)
				// Drop and recreate — old integer data is not useful
				_, _ = database.Exec(`DROP TABLE schema_migrations`)
				_, _ = database.Exec(`CREATE TABLE schema_migrations (
					filename VARCHAR(255) PRIMARY KEY,
					executed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
				)`)
				log.Println("schema_migrations table recreated with correct schema")
			} else {
				// Ensure executed_at exists
				var hasExec bool
				_ = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='schema_migrations' AND column_name='executed_at')`).Scan(&hasExec)
				if !hasExec {
					log.Println("Adding executed_at column to schema_migrations")
					_, _ = database.Exec(`ALTER TABLE schema_migrations ADD COLUMN executed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP`)
				}
			}
		}

		if len(files) == 0 {
			log.Println("Warning: No migration files found!")
		}
		for _, file := range files {
			baseName := filepath.Base(file)
			var alreadyExecuted bool
			// Check if migration was already executed
			errCheck := database.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename=$1)", baseName).Scan(&alreadyExecuted)
			if errCheck != nil {
				log.Printf("Warning: Could not check migration status for %s: %v", baseName, errCheck)
				continue
			}
			if alreadyExecuted {
				continue
			}

			fmt.Printf("Executing migration: %s\n", file)
			migration, err := os.ReadFile(file)
			if err != nil {
				log.Printf("Could not read migration file %s: %v", file, err)
				continue
			}
			_, err = database.Exec(string(migration))
			if err != nil {
				log.Printf("Migration %s warning: %v", file, err)
				// Still record it — "already exists" errors mean the migration was applied before
				if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
					_, _ = database.Exec("INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING", baseName)
				}
			} else {
				fmt.Printf("Migration %s executed successfully.\n", file)
			}
			// Record successful or partially-successful migration
			_, insertErr := database.Exec("INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING", baseName)
			if insertErr != nil {
				log.Printf("Warning: Could not record migration %s: %v", baseName, insertErr)
			}
		}
	}

	// Start Background Worker
	worker.StartWorker(database)

	// Trigger async refresh of views (Startup)
	go func() {
		// Wait for server to start serving requests
		time.Sleep(5 * time.Second)
		log.Println("Background: Triggering initial view refresh (mv_mercadorias_agregada)...")
		_, err := database.Exec("REFRESH MATERIALIZED VIEW mv_mercadorias_agregada")
		if err != nil {
			log.Printf("Background: Initial view refresh failed: %v", err)
		} else {
			log.Println("Background: Initial view refresh completed successfully.")
		}
	}()
}

// Middleware to check if DB is ready
func DBMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		database := getDB()
		if database == nil {
			dbMutex.RLock()
			err := dbErr
			dbMutex.RUnlock()
			errMsg := "Database not initialized yet"
			if err != nil {
				errMsg += ": " + err.Error()
			}
			http.Error(w, errMsg, http.StatusServiceUnavailable)
			return
		}
		next(w, r)
	}
}

func main() {
	// Load .env file if it exists
	_ = godotenv.Load()

	PrintVersion()

	// Start DB connection in background
	initDBAsync()
	// defer db.Close() // Cannot defer nil, handle in shutdown if needed

	// DEBUG: Emergency route to delete Iolanda
	http.HandleFunc("/api/debug/nuke-iolanda", func(w http.ResponseWriter, r *http.Request) {
		database := getDB()
		if database == nil {
			http.Error(w, "Database not ready", http.StatusServiceUnavailable)
			return
		}
		email := "iolanda_fortes@hotmail.com"
		_, err := database.Exec("DELETE FROM users WHERE email = $1", email)
		if err != nil {
			http.Error(w, "Error deleting user: "+err.Error(), 500)
			return
		}
		w.Write([]byte("User " + email + " deleted successfully. Please register again."))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		// CORS headers para desenvolvimento
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")

		dbStatus := "connecting..."
		database := getDB()
		var dbStats string
		var lastErr string

		if database != nil {
			stats := database.Stats()
			if err := database.Ping(); err != nil {
				dbStatus = "error: " + err.Error()
			} else {
				dbStatus = "connected"
			}
			dbStats = fmt.Sprintf("Open: %d, InUse: %d, Idle: %d, Wait: %v", stats.OpenConnections, stats.InUse, stats.Idle, stats.WaitDuration)
		} else {
			dbMutex.RLock()
			if dbErr != nil {
				dbStatus = "error"
				lastErr = dbErr.Error()
			}
			dbMutex.RUnlock()
		}

		response := HealthResponse{
			Status:    "running",
			Timestamp: time.Now().Format(time.RFC3339),
			Service:   "FB_APU01 Fiscal Engine",
			Version:   BackendVersion,
			Features:  FeatureSet,
			Database:  fmt.Sprintf("%s (%s)", dbStatus, dbStats),
			DBError:   lastErr,
		}

		json.NewEncoder(w).Encode(response)
	})

	// Wrap handlers with DBMiddleware where db is required, but we need to inject the db instance safely.
	// Since existing handlers expect *sql.DB, we need a wrapper that gets the current DB instance.
	// A better approach for this refactor without rewriting all handlers is to make handlers accept a getter or check for nil.
	// However, most handlers in 'handlers' package likely accept *sql.DB.
	// If we pass 'db' variable (which is nil initially) to handlers factories, they will have nil.
	// We need to delay the DB access inside handlers.

	// CRITICAL: The current architecture passes 'db' (pointer) to handler factories.
	// If 'db' is nil at startup, handlers get nil.
	// We need a proxy DB object or change how handlers are registered.
	// Since we can't easily change all handlers signatures now, we will rely on the fact that
	// we are passing the global 'db' variable. BUT, in Go, arguments are passed by value.
	// Passing a nil pointer means the handler has a nil pointer forever.

	// FIX: We will create a proxy handler that gets the DB on request.
	// But handlers.GetMeHandler(db) returns a func.
	// We have to wrap the FACTORY calls.

	// Helper to wrap DB dependency
	withDB := func(handlerFactory func(*sql.DB) http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			database := getDB()
			if database == nil {
				http.Error(w, "Database initializing, please wait...", http.StatusServiceUnavailable)
				return
			}
			// Create handler with ready DB and serve
			handlerFactory(database)(w, r)
		}
	}

	// Auth AuthMiddleware wrapper needs special care as it takes a handler.
	withAuth := func(handlerFactory func(*sql.DB) http.HandlerFunc, role string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			database := getDB()
			if database == nil {
				http.Error(w, "Database initializing...", http.StatusServiceUnavailable)
				return
			}
			// Create handler, then wrap in Auth
			h := handlerFactory(database)
			handlers.AuthMiddleware(h, role)(w, r)
		}
	}

	// Report Endpoints
	http.HandleFunc("/api/reports/mercadorias", withAuth(handlers.GetMercadoriasReportHandler, ""))
	http.HandleFunc("/api/reports/energia", withAuth(handlers.GetEnergiaReportHandler, ""))
	http.HandleFunc("/api/reports/transporte", withAuth(handlers.GetTransporteReportHandler, ""))
	http.HandleFunc("/api/reports/comunicacoes", withAuth(handlers.GetComunicacoesReportHandler, ""))
	http.HandleFunc("/api/dashboard/projection", withAuth(handlers.GetDashboardProjectionHandler, ""))
	http.HandleFunc("/api/dashboard/simples-nacional", withAuth(handlers.GetSimplesDashboardHandler, ""))

	// AI-Powered Report Endpoints
	http.HandleFunc("/api/reports/available-periods", withAuth(handlers.GetAvailablePeriodsHandler, ""))
	http.HandleFunc("/api/reports/executive-summary", withAuth(handlers.GetExecutiveSummaryHandler, ""))
	http.HandleFunc("/api/insights/daily", withAuth(handlers.GetDailyInsightHandler, ""))

	// Saved AI Reports
	http.HandleFunc("/api/reports", withAuth(handlers.ListSavedAIReportsHandler, ""))
	http.HandleFunc("/api/reports/", withAuth(handlers.GetSavedAIReportHandler, ""))

	// Register Upload Handler
	http.HandleFunc("/api/upload", withAuth(handlers.UploadHandler, ""))

	// Register Check Duplicity Handler
	http.HandleFunc("/api/check-duplicity", withAuth(handlers.CheckDuplicityHandler, ""))

	// Register Job Status Handler
	http.HandleFunc("/api/jobs", withAuth(handlers.ListJobsHandler, ""))

	// Custom wrapper for jobs/id (supports /participants and /cancel sub-routes)
	http.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		database := getDB()
		if database == nil {
			http.Error(w, "Database initializing...", http.StatusServiceUnavailable)
			return
		}
		handlers.AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
			if strings.HasSuffix(path, "/participants") {
				handlers.GetJobParticipantsHandler(database)(w, r)
				return
			}
			if strings.HasSuffix(path, "/cancel") {
				handlers.CancelJobHandler(database)(w, r)
				return
			}
			handlers.GetJobStatusHandler(database)(w, r)
		}, "")(w, r)
	})

	// Auth Routes
	http.HandleFunc("/api/auth/register", withDB(handlers.RegisterHandler))
	http.HandleFunc("/api/auth/login", withDB(handlers.LoginHandler))
	http.HandleFunc("/api/auth/me", withAuth(handlers.GetMeHandler, ""))
	http.HandleFunc("/api/auth/forgot-password", withDB(handlers.ForgotPasswordHandler))
	http.HandleFunc("/api/auth/reset-password", withDB(handlers.ResetPasswordHandler))
	http.HandleFunc("/api/user/hierarchy", withAuth(handlers.GetUserHierarchyHandler, ""))
	http.HandleFunc("/api/user/companies", withAuth(handlers.GetUserCompaniesHandler, ""))

	http.HandleFunc("/api/mercadorias", withAuth(handlers.GetMercadoriasReportHandler, ""))

	// Admin Endpoints
	http.HandleFunc("/api/admin/reset-db", withAuth(handlers.ResetDatabaseHandler, "admin"))
	http.HandleFunc("/api/company/reset-data", withAuth(handlers.ResetCompanyDataHandler, ""))
	http.HandleFunc("/api/admin/refresh-views", withAuth(handlers.RefreshViewsHandler, ""))
	http.HandleFunc("/api/admin/users", withAuth(handlers.ListUsersHandler, "admin"))
	http.HandleFunc("/api/admin/users/create", withAuth(handlers.CreateUserHandler, "admin"))
	http.HandleFunc("/api/admin/users/promote", withAuth(handlers.PromoteUserHandler, "admin"))
	http.HandleFunc("/api/admin/users/delete", withAuth(handlers.DeleteUserHandler, "admin"))
	http.HandleFunc("/api/admin/users/reassign", withAuth(handlers.ReassignUserHandler, "admin"))

	// Configuration Endpoints
	http.HandleFunc("/api/config/aliquotas", withDB(handlers.GetTaxRatesHandler))
	http.HandleFunc("/api/config/cfop", withDB(handlers.ListCFOPsHandler))
	http.HandleFunc("/api/config/cfop/import", withDB(handlers.ImportCFOPsHandler))

	http.HandleFunc("/api/config/forn-simples", func(w http.ResponseWriter, r *http.Request) {
		database := getDB()
		if database == nil {
			http.Error(w, "Database initializing...", http.StatusServiceUnavailable)
			return
		}
		switch r.Method {
		case http.MethodGet:
			handlers.ListFornSimplesHandler(database)(w, r)
		case http.MethodPost:
			handlers.CreateFornSimplesHandler(database)(w, r)
		case http.MethodDelete:
			handlers.DeleteFornSimplesHandler(database)(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	http.HandleFunc("/api/config/forn-simples/import", withDB(handlers.ImportFornSimplesHandler))

	// Environment & Groups Endpoints
	http.HandleFunc("/api/config/environments", withAuth(func(db *sql.DB) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handlers.GetEnvironmentsHandler(db)(w, r)
			case http.MethodPost:
				handlers.CreateEnvironmentHandler(db)(w, r)
			case http.MethodPut:
				handlers.UpdateEnvironmentHandler(db)(w, r)
			case http.MethodDelete:
				handlers.DeleteEnvironmentHandler(db)(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}
	}, ""))

	http.HandleFunc("/api/config/groups", withAuth(func(db *sql.DB) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handlers.GetGroupsHandler(db)(w, r)
			case http.MethodPost:
				handlers.CreateGroupHandler(db)(w, r)
			case http.MethodDelete:
				handlers.DeleteGroupHandler(db)(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}
	}, ""))

	http.HandleFunc("/api/config/companies", withAuth(func(db *sql.DB) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handlers.GetCompaniesHandler(db)(w, r)
			case http.MethodPost:
				handlers.CreateCompanyHandler(db)(w, r)
			case http.MethodDelete:
				handlers.DeleteCompanyHandler(db)(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}
	}, ""))

	// RFB Credentials Endpoints (Conectar Receita Federal)
	http.HandleFunc("/api/rfb/credentials", func(w http.ResponseWriter, r *http.Request) {
		database := getDB()
		if database == nil {
			http.Error(w, "Database initializing...", http.StatusServiceUnavailable)
			return
		}
		switch r.Method {
		case http.MethodGet:
			handlers.AuthMiddleware(handlers.GetRFBCredentialHandler(database), "")(w, r)
		case http.MethodPost:
			handlers.AuthMiddleware(handlers.SaveRFBCredentialHandler(database), "")(w, r)
		case http.MethodDelete:
			handlers.AuthMiddleware(handlers.DeleteRFBCredentialHandler(database), "")(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// RFB Apuração Endpoints
	http.HandleFunc("/api/rfb/apuracao/solicitar", withAuth(handlers.SolicitarApuracaoHandler, ""))
	http.HandleFunc("/api/rfb/apuracao/download", withAuth(handlers.DownloadManualHandler, ""))
	http.HandleFunc("/api/rfb/apuracao/status", withAuth(handlers.StatusApuracaoHandler, ""))
	http.HandleFunc("/api/rfb/apuracao/", withAuth(handlers.DetalheApuracaoHandler, ""))

	// RFB Webhook (PUBLIC - no JWT auth, called by Receita Federal)
	http.HandleFunc("/api/rfb/webhook", withDB(handlers.RFBWebhookHandler))

	// Managers Endpoints (Gestores para relatorios IA)
	http.HandleFunc("/api/managers", withAuth(handlers.ListManagersHandler, ""))
	http.HandleFunc("/api/managers/create", withAuth(handlers.CreateManagerHandler, ""))
	http.HandleFunc("/api/managers/", func(w http.ResponseWriter, r *http.Request) {
		database := getDB()
		if database == nil {
			http.Error(w, "Database initializing...", http.StatusServiceUnavailable)
			return
		}
		// Route to update or delete based on HTTP method
		switch r.Method {
		case http.MethodPut, http.MethodPatch:
			handlers.AuthMiddleware(handlers.UpdateManagerHandler(database), "")(w, r)
		case http.MethodDelete:
			handlers.AuthMiddleware(handlers.DeleteManagerHandler(database), "")(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	fmt.Printf("FB_APU01 Fiscal Engine (Go) starting on port %s...\n", port)

	// Print Version
	fmt.Println("==================================================")
	fmt.Printf("   FB_APU01 BACKEND - %s\n", BackendVersion)
	fmt.Println("==================================================")

	// Use custom server with timeouts (Inspired by production best practices)
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      nil,               // Use DefaultServeMux
		ReadTimeout:  300 * time.Second, // 5 minutes for Uploads
		WriteTimeout: 300 * time.Second, // 5 minutes for Long Responses
		IdleTimeout:  60 * time.Second,
	}

	// Graceful Shutdown: close DB connections on SIGTERM/SIGINT
	// Prevents stale connections accumulating in PostgreSQL overnight
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down gracefully...", sig)

		// Give active requests 10 seconds to finish
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}

		database := getDB()
		if database != nil {
			log.Println("Closing database connections...")
			database.Close()
		}

		log.Println("Shutdown complete.")
		os.Exit(0)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
