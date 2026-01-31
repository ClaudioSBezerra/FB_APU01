package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type UploadResponse struct {
	JobID    string `json:"job_id"`
	Message  string `json:"message"`
	Filename string `json:"filename"`
}

func UploadHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// CORS Headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Limit upload size (1GB to allow large SPED files > 500MB)
		// Note: ParseMultipartForm limit is for memory; larger files spill to disk.
		// However, we increase this to be safe and ensure large headers/parts are handled.
		r.ParseMultipartForm(1024 << 20)

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Error retrieving the file: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Validate extension
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext != ".txt" && ext != ".xml" {
			http.Error(w, "Invalid file type. Only .txt and .xml are allowed.", http.StatusBadRequest)
			return
		}

		// Sanitize filename and create unique name
		originalName := filepath.Base(header.Filename)
		timestamp := time.Now().Format("20060102_150405")
		safeFilename := fmt.Sprintf("%s_%s", timestamp, originalName)
		uploadDir := "uploads"
		savePath := filepath.Join(uploadDir, safeFilename)

		// Create uploads directory if it doesn't exist
		if err := os.MkdirAll(uploadDir, 0755); err != nil {
			http.Error(w, "Failed to create upload directory", http.StatusInternalServerError)
			return
		}

		// Save file to disk
		dst, err := os.Create(savePath)
		if err != nil {
			http.Error(w, "Unable to create the file on server", http.StatusInternalServerError)
			return
		}
		defer dst.Close()

		written, err := io.Copy(dst, file)
		if err != nil {
			http.Error(w, "Unable to save the file content", http.StatusInternalServerError)
			return
		}

		// Verify size
		fmt.Printf("Upload Debug: Header Size: %d, Written: %d\n", header.Size, written)
		if written != header.Size {
			fmt.Printf("WARNING: Upload size mismatch! Header says %d but wrote %d bytes.\n", header.Size, written)
		}

		// Insert job into database
		var jobID string
		query := `INSERT INTO import_jobs (filename, status, message) VALUES ($1, $2, $3) RETURNING id`
		err = db.QueryRow(query, safeFilename, "pending", "File received and saved").Scan(&jobID)
		if err != nil {
			// Try to cleanup file if DB fails
			os.Remove(savePath)
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Printf("File saved: %s (Size: %d bytes) -> Job ID: %s\n", savePath, header.Size, jobID)

		response := UploadResponse{
			JobID:    jobID,
			Message:  "File uploaded and saved successfully",
			Filename: safeFilename,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}