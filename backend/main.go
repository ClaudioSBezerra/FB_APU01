package main

// Force rebuild
import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fb_apu01/handlers"
	"fb_apu01/worker"

	_ "github.com/lib/pq"
)

// Version information for backend deployment validation
const (
	BackendVersion = "4.9.2"
	FeatureSet     = "Auto-Refresh View & Chart Fix"
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
}

var db *sql.DB

func initDB() {
	var err error
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		// Fallback for local development
		connStr = "postgres://postgres:postgres@localhost:5432/fiscal_db?sslmode=disable"
		fmt.Println("DATABASE_URL not set, using default local connection:", connStr)
	}

	// Retry logic for database connection
	for i := 0; i < 10; i++ {
		db, err = sql.Open("postgres", connStr)
		if err == nil {
			err = db.Ping()
			if err == nil {
				// Configure Connection Pool
				db.SetMaxOpenConns(25)
				db.SetMaxIdleConns(5)
				db.SetConnMaxLifetime(5 * time.Minute)
				fmt.Println("Successfully connected to the database!")
				return
			}
		}
		fmt.Printf("Failed to connect to database (attempt %d/10): %v. Retrying in 2s...\n", i+1, err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("Could not connect to database after retries: %v", err)
}

func resetStuckJobs(db *sql.DB) {
	res, err := db.Exec("UPDATE import_jobs SET status='failed', message=message || ' [Interrupted by server restart]' WHERE status='processing'")
	if err != nil {
		log.Printf("Error resetting stuck jobs: %v", err)
		return
	}
	count, _ := res.RowsAffected()
	if count > 0 {
		fmt.Printf("Startup: Reset %d stuck jobs to 'failed' status.\n", count)
	}
}

func main() {
	PrintVersion()
	initDB()
	defer db.Close()

	// DEBUG: Emergency route to delete Iolanda
	http.HandleFunc("/api/debug/nuke-iolanda", func(w http.ResponseWriter, r *http.Request) {
		email := "iolanda_fortes@hotmail.com"
		// Delete related data first if cascades aren't set up (assuming cascades work for simplicity, but let's be safe)
		// Actually, let's rely on CASCADE or manual cleanup if needed.
		// For now, just delete user.
		_, err := db.Exec("DELETE FROM users WHERE email = $1", email)
		if err != nil {
			http.Error(w, "Error deleting user: "+err.Error(), 500)
			return
		}
		w.Write([]byte("User " + email + " deleted successfully. Please register again."))
	})

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
		if len(files) == 0 {
			log.Println("Warning: No migration files found!")
		}
		for _, file := range files {
			fmt.Printf("Executing migration: %s\n", file)
			migration, err := os.ReadFile(file)
			if err != nil {
				log.Printf("Could not read migration file %s: %v", file, err)
				continue
			}
			_, err = db.Exec(string(migration))
			if err != nil {
				log.Printf("Migration %s warning: %v", file, err)
			} else {
				fmt.Printf("Migration %s executed successfully.\n", file)
			}
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		// CORS headers para desenvolvimento
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")

		dbStatus := "connected"
		stats := db.Stats()
		if err := db.Ping(); err != nil {
			dbStatus = "error: " + err.Error()
		}

		response := HealthResponse{
			Status:    "running",
			Timestamp: time.Now().Format(time.RFC3339),
			Service:   "FB_APU01 Fiscal Engine",
			Version:   BackendVersion,
			Features:  FeatureSet,
			Database:  fmt.Sprintf("%s (Open: %d, InUse: %d, Idle: %d, Wait: %v)", dbStatus, stats.OpenConnections, stats.InUse, stats.Idle, stats.WaitDuration),
		}

		json.NewEncoder(w).Encode(response)
	})

	// Report Endpoints
	http.HandleFunc("/api/reports/mercadorias", handlers.GetMercadoriasReportHandler(db))
	http.HandleFunc("/api/reports/energia", handlers.GetEnergiaReportHandler(db))
	http.HandleFunc("/api/reports/transporte", handlers.GetTransporteReportHandler(db))
	http.HandleFunc("/api/reports/comunicacoes", handlers.GetComunicacoesReportHandler(db))

	// Start Background Worker
	worker.StartWorker(db)

	// Trigger async refresh of views (Startup)
	// Added manual trigger comment to force git sync
	go func() {
		// Wait for server to start serving requests
		time.Sleep(5 * time.Second)
		log.Println("Background: Triggering initial view refresh (mv_mercadorias_agregada)...")
		_, err := db.Exec("REFRESH MATERIALIZED VIEW mv_mercadorias_agregada")
		if err != nil {
			log.Printf("Background: Initial view refresh failed: %v", err)
		} else {
			log.Println("Background: Initial view refresh completed successfully.")
		}
	}()

	// Register Upload Handler
	http.HandleFunc("/api/upload", handlers.AuthMiddleware(handlers.UploadHandler(db), ""))

	// Register Job Status Handler
	http.HandleFunc("/api/jobs", handlers.ListJobsHandler(db))
	http.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
		if strings.HasSuffix(id, "/participants") {
			handlers.GetJobParticipantsHandler(db)(w, r)
			return
		}
		handlers.GetJobStatusHandler(db)(w, r)
	})

	// Auth Routes
	http.HandleFunc("/api/auth/register", handlers.RegisterHandler(db))
	http.HandleFunc("/api/auth/login", handlers.LoginHandler(db))
	http.HandleFunc("/api/auth/forgot-password", handlers.ForgotPasswordHandler(db))
	http.HandleFunc("/api/user/hierarchy", handlers.AuthMiddleware(handlers.GetUserHierarchyHandler(db), ""))

	http.HandleFunc("/api/mercadorias", handlers.GetMercadoriasReportHandler(db))

	// Admin Endpoints
	http.HandleFunc("/api/admin/reset-db", handlers.AuthMiddleware(handlers.ResetDatabaseHandler(db), "admin"))
	http.HandleFunc("/api/company/reset-data", handlers.AuthMiddleware(handlers.ResetCompanyDataHandler(db), "")) // Authenticated users can reset their own data
	http.HandleFunc("/api/admin/refresh-views", handlers.AuthMiddleware(handlers.RefreshViewsHandler(db), ""))    // Authenticated users can refresh views
	http.HandleFunc("/api/admin/users", handlers.AuthMiddleware(handlers.ListUsersHandler(db), "admin"))
	http.HandleFunc("/api/admin/users/promote", handlers.AuthMiddleware(handlers.PromoteUserHandler(db), "admin"))
	http.HandleFunc("/api/admin/users/delete", handlers.AuthMiddleware(handlers.DeleteUserHandler(db), "admin"))

	// Configuration Endpoints
	http.HandleFunc("/api/config/aliquotas", handlers.GetTaxRatesHandler(db))
	http.HandleFunc("/api/config/cfop", handlers.ListCFOPsHandler(db))
	http.HandleFunc("/api/config/cfop/import", handlers.ImportCFOPsHandler(db))

	// Environment & Groups Endpoints
	http.HandleFunc("/api/config/environments", func(w http.ResponseWriter, r *http.Request) {
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
	})

	http.HandleFunc("/api/config/groups", func(w http.ResponseWriter, r *http.Request) {
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
	})

	http.HandleFunc("/api/config/companies", func(w http.ResponseWriter, r *http.Request) {
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

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
