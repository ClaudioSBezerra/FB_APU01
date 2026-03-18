package main

// Force rebuild: 2026-02-27 - Version 5.9.0 - NF-e Entradas + Creditos em Risco + Email Estruturado
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
// Force rebuild: 2026-03-18 - v5.9.13 - Separação simulador/apuracao + docker-compose Service
const (
	BackendVersion = "5.9.13"
	FeatureSet     = "Z.AI GLM Integration, AI Executive Reports, Email Estruturado, NF-e Entradas, Creditos em Risco, Tax Reform Projection, Simples Nacional Dashboard, Apelidos de Filiais, Filtro Multi-Filial Global"
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

	// Start Background Worker (only for Simulador — SPED worker not needed in Apuração)
	appModule := os.Getenv("APP_MODULE")
	if appModule != "apuracao" {
		worker.StartWorker(database)

		// Trigger async refresh of materialized views (Startup — Simulador only)
		go func() {
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
}

// Middleware to check if DB is ready
func DBMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		database := getDB()
		if database == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"service_unavailable","message":"Database initializing, please try again in a moment."}`))
			return
		}
		next(w, r)
	}
}

func main() {
	// Load .env file if it exists
	_ = godotenv.Load()

	// Validate JWT_SECRET — warns in dev, fatals in prod
	handlers.ValidateJWTSecret()

	// APP_MODULE controls which route groups are registered:
	//   "simulador" — SPED upload/jobs/reports/AI; no NF-e/CT-e/RFB routes
	//   "apuracao"  — NF-e/CT-e/RFB/apuração; no upload/jobs/reports/AI routes
	//   ""  / "all" — all routes (local dev)
	appModule := os.Getenv("APP_MODULE")
	log.Printf("APP_MODULE=%q", appModule)

	PrintVersion()

	// Start DB connection in background
	initDBAsync()
	// defer db.Close() // Cannot defer nil, handle in shutdown if needed

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
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
	jsonServiceUnavailable := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"service_unavailable","message":"Database initializing, please try again in a moment."}`))
	}

	withDB := func(handlerFactory func(*sql.DB) http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			database := getDB()
			if database == nil {
				jsonServiceUnavailable(w)
				return
			}
			handlerFactory(database)(w, r)
		}
	}

	withAuth := func(handlerFactory func(*sql.DB) http.HandlerFunc, role string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			database := getDB()
			if database == nil {
				jsonServiceUnavailable(w)
				return
			}
			h := handlerFactory(database)
			handlers.AuthMiddleware(h, role)(w, r)
		}
	}

	// Filiais Endpoint (global branch selector)
	http.HandleFunc("/api/filiais", withAuth(handlers.GetFiliaisHandler, ""))

	// ── Simulador da Reforma Tributária (SPED) — routes skipped in APP_MODULE=apuracao ──
	if appModule != "apuracao" {
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
		http.HandleFunc("/api/ai/query", withAuth(handlers.AIQueryHandler, ""))

		// Saved AI Reports
		http.HandleFunc("/api/reports", withAuth(handlers.ListSavedAIReportsHandler, ""))
		http.HandleFunc("/api/reports/", withAuth(handlers.GetSavedAIReportHandler, ""))

		// SPED Upload Handler
		http.HandleFunc("/api/upload", withAuth(handlers.UploadHandler, ""))

		// Check Duplicity Handler
		http.HandleFunc("/api/check-duplicity", withAuth(handlers.CheckDuplicityHandler, ""))

		// Job Status Handlers
		http.HandleFunc("/api/jobs", withAuth(handlers.ListJobsHandler, ""))

		// Custom wrapper for jobs/id (supports /participants and /cancel sub-routes)
		http.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
			database := getDB()
			if database == nil {
				jsonServiceUnavailable(w)
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

		http.HandleFunc("/api/mercadorias", withAuth(handlers.GetMercadoriasReportHandler, ""))
	}

	// Auth Routes
	http.HandleFunc("/api/auth/register", withDB(handlers.RegisterHandler))
	http.HandleFunc("/api/auth/login", withDB(handlers.LoginHandler))
	http.HandleFunc("/api/auth/me", withAuth(handlers.GetMeHandler, ""))
	http.HandleFunc("/api/auth/forgot-password", withDB(handlers.ForgotPasswordHandler))
	http.HandleFunc("/api/auth/reset-password", withDB(handlers.ResetPasswordHandler))
	http.HandleFunc("/api/auth/change-password", withAuth(handlers.ChangePasswordHandler, ""))
	http.HandleFunc("/api/auth/refresh", withDB(handlers.RefreshHandler))
	http.HandleFunc("/api/auth/logout", withDB(handlers.LogoutHandler))
	http.HandleFunc("/api/user/hierarchy", withAuth(handlers.GetUserHierarchyHandler, ""))
	http.HandleFunc("/api/user/companies", withAuth(handlers.GetUserCompaniesHandler, ""))

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
	http.HandleFunc("/api/config/aliquotas", withAuth(handlers.GetTaxRatesHandler, ""))
	http.HandleFunc("/api/config/cfop", withAuth(handlers.ListCFOPsHandler, ""))
	http.HandleFunc("/api/config/cfop/import", withAuth(handlers.ImportCFOPsHandler, ""))

	http.HandleFunc("/api/config/forn-simples", func(w http.ResponseWriter, r *http.Request) {
		database := getDB()
		if database == nil {
			jsonServiceUnavailable(w)
			return
		}
		switch r.Method {
		case http.MethodGet:
			handlers.AuthMiddleware(handlers.ListFornSimplesHandler(database), "")(w, r)
		case http.MethodPost:
			handlers.AuthMiddleware(handlers.CreateFornSimplesHandler(database), "")(w, r)
		case http.MethodDelete:
			handlers.AuthMiddleware(handlers.DeleteFornSimplesHandler(database), "")(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	http.HandleFunc("/api/config/forn-simples/import", withAuth(handlers.ImportFornSimplesHandler, ""))

	http.HandleFunc("/api/config/filial-apelidos", withAuth(handlers.FilialApelidosHandler, ""))
	http.HandleFunc("/api/config/filial-apelidos/import", withAuth(handlers.ImportFilialApelidosHandler, ""))

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

	// ── Apuração Assistida + Receita Federal — routes skipped in APP_MODULE=simulador ──
	if appModule != "simulador" {
		// RFB Credentials Endpoints (Conectar Receita Federal)
		http.HandleFunc("/api/rfb/credentials", func(w http.ResponseWriter, r *http.Request) {
			database := getDB()
			if database == nil {
				jsonServiceUnavailable(w)
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
		http.HandleFunc("/api/rfb/apuracao/reprocess", withAuth(handlers.ReprocessHandler, ""))
		http.HandleFunc("/api/rfb/apuracao/clear-errors", withAuth(handlers.ClearErrorsHandler, ""))
		http.HandleFunc("/api/rfb/apuracao/status", withAuth(handlers.StatusApuracaoHandler, ""))
		http.HandleFunc("/api/rfb/apuracao/", withAuth(handlers.DetalheApuracaoHandler, ""))

		// RFB Webhook (PUBLIC - no JWT auth, called by Receita Federal)
		http.HandleFunc("/api/rfb/webhook", withDB(handlers.RFBWebhookHandler))

		// Apuração Assistida — NF-e Saídas
		http.HandleFunc("/api/nfe-saidas/upload", withAuth(handlers.NfeSaidasUploadHandler, ""))
		http.HandleFunc("/api/nfe-saidas", withAuth(handlers.NfeSaidasListHandler, ""))

		// Apuração Assistida — NF-e Entradas
		http.HandleFunc("/api/nfe-entradas/upload", withAuth(handlers.NfeEntradasUploadHandler, ""))
		http.HandleFunc("/api/nfe-entradas", withAuth(handlers.NfeEntradasListHandler, ""))

		// Apuração Assistida — CT-e Entradas
		http.HandleFunc("/api/cte-entradas/upload", withAuth(handlers.CteEntradasUploadHandler, ""))
		http.HandleFunc("/api/cte-entradas", withAuth(handlers.CteEntradasListHandler, ""))

		// Apuração Assistida — Créditos IBS/CBS em Risco
		http.HandleFunc("/api/apuracao/creditos-perdidos", withAuth(handlers.CreditosPerdidosHandler, ""))

		// Painel Apuração IBS/CBS
		http.HandleFunc("/api/apuracao/painel", withAuth(handlers.ApuracaoPainelHandler, ""))
	}

	// Managers Endpoints (Gestores para relatorios IA)
	http.HandleFunc("/api/managers", withAuth(handlers.ListManagersHandler, ""))
	http.HandleFunc("/api/managers/create", withAuth(handlers.CreateManagerHandler, ""))
	http.HandleFunc("/api/managers/", func(w http.ResponseWriter, r *http.Request) {
		database := getDB()
		if database == nil {
			jsonServiceUnavailable(w)
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

	// Serve frontend static files (SPA — React Router)
	staticDir := "./static"
	if _, err := os.Stat(staticDir); err == nil {
		fs := http.FileServer(http.Dir(staticDir))
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// API routes are handled by their own handlers above
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			// Serve the file if it exists, otherwise fall back to index.html (SPA routing)
			filePath := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
				return
			}
			fs.ServeHTTP(w, r)
		})
		fmt.Println("Serving frontend from ./static")
	}

	fmt.Printf("FB_APU01 Fiscal Engine (Go) starting on port %s...\n", port)

	// Print Version
	fmt.Println("==================================================")
	fmt.Printf("   FB_APU01 BACKEND - %s\n", BackendVersion)
	fmt.Println("==================================================")

	// Use custom server with timeouts (Inspired by production best practices)
	server := &http.Server{
		Addr:    ":" + port,
		Handler: handlers.SecurityMiddleware(http.DefaultServeMux, handlers.GetAllowedOrigins()),
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
