package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

		// STANDARD STREAMING UPLOAD LOGIC
		// Uses ParseMultipartForm with 64MB memory limit, spilling to disk for larger files.
		// Nginx handles the absolute max body size (2GB+).
		r.ParseMultipartForm(64 << 20)

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
		log.Printf("Upload Debug: Starting to write file %s to storage...\n", safeFilename)
		dst, err := os.Create(savePath)
		if err != nil {
			log.Printf("Upload Error: Failed to create file on storage: %v\n", err)
			http.Error(w, "Unable to create the file on server", http.StatusInternalServerError)
			return
		}
		
		// Use io.Copy for efficient streaming from request to disk
		written, err := io.Copy(dst, file)
		// Close file immediately after writing to ensure flush
		dst.Close() 

		if err != nil {
			log.Printf("Upload Error: Failed to write content to storage: %v\n", err)
			http.Error(w, "Unable to save the file content", http.StatusInternalServerError)
			return
		}
		log.Printf("Upload Debug: Successfully wrote %d bytes to %s\n", written, safeFilename)

		// Verify size
		log.Printf("Upload Debug: Header Size: %d, Written: %d\n", header.Size, written)
		if written != header.Size {
			log.Printf("WARNING: Upload size mismatch! Header says %d but wrote %d bytes.\n", header.Size, written)
		}

		// --- Integrity Check (Storage Verification) ---
		expectedLines := r.FormValue("expected_lines")
		expectedSize := r.FormValue("expected_size")

		// Read last 16KB to find |9999| (Handling Digital Signatures)
		if fi, err := os.Stat(savePath); err == nil {
			written := fi.Size()
			tailBuf := make([]byte, 16384) // 16KB buffer
			startPos := int64(0)
			if written > 16384 { startPos = written - 16384 }
			
			if fCheck, err := os.Open(savePath); err == nil {
				fCheck.ReadAt(tailBuf, startPos)
				fCheck.Close()
				
				tailStr := string(tailBuf)
				actualLines := "not_found"
				
				// Look for last valid |9999| occurrence
				// Regex to find |9999|COUNT|
				// We search from end manually or iterate
				lines := strings.Split(tailStr, "\n")
				for i := len(lines) - 1; i >= 0; i-- {
					if strings.Contains(lines[i], "|9999|") {
						parts := strings.Split(lines[i], "|")
						// |9999|COUNT| -> index 0 is empty, 1 is 9999, 2 is COUNT
						if len(parts) >= 3 && parts[1] == "9999" {
							actualLines = parts[2]
							break // Found the trailer, ignore subsequent garbage/signatures
						}
					}
				}

				fmt.Printf("LOG API Integrity: File=%s\n", safeFilename)
				fmt.Printf("LOG API Frontend: Expected Lines=%s, Expected Size=%s\n", expectedLines, expectedSize)
				fmt.Printf("LOG API Storage:  Registro Final %s, tamanho recebido final %d\n", actualLines, written)
				
				if expectedLines != "unknown" && expectedLines != actualLines {
					fmt.Printf("LOG API WARNING: Line count mismatch! Frontend says %s, Storage found %s\n", expectedLines, actualLines)
				}
			}
		}
		// ---------------------------------------------

		// Insert job into database
		var jobID string
		query := `INSERT INTO import_jobs (filename, status, message) VALUES ($1, $2, $3) RETURNING id`
		err = db.QueryRow(query, safeFilename, "pending", "File received and saved").Scan(&jobID)
		if err != nil {
			// Try to cleanup file if DB fails
			os.Remove(savePath)
			log.Printf("Database Error: %v\n", err)
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("Upload Success: Job %s created for file %s\n", jobID, safeFilename)

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