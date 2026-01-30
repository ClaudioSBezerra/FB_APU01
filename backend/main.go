package main

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

type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Service   string `json:"service"`
	Version   string `json:"version"`
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
				fmt.Println("Successfully connected to the database!")
				return
			}
		}
		fmt.Printf("Failed to connect to database (attempt %d/10): %v. Retrying in 2s...\n", i+1, err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("Could not connect to database after retries: %v", err)
}

func main() {
	initDB()
	defer db.Close()

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

		response := HealthResponse{
			Status:    "running",
			Timestamp: time.Now().Format(time.RFC3339),
			Service:   "FB_APU01 Fiscal Engine",
			Version:   "0.1.0",
			Database:  "connected",
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

	// Register Upload Handler
	http.HandleFunc("/api/upload", handlers.UploadHandler(db))

	// Register Job Status Handler
	http.HandleFunc("/api/jobs", handlers.ListJobsHandler(db))
	http.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/participants") {
			handlers.GetJobParticipantsHandler(db)(w, r)
		} else {
			handlers.GetJobStatusHandler(db)(w, r)
		}
	})

	// Configuration Endpoints
	http.HandleFunc("/api/config/aliquotas", handlers.GetTaxRatesHandler(db))
	http.HandleFunc("/api/config/cfop", handlers.ListCFOPsHandler(db))
	http.HandleFunc("/api/config/cfop/import", handlers.ImportCFOPsHandler(db))

	fmt.Printf("FB_APU01 Fiscal Engine (Go) starting on port %s...\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}