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

func Atoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

type UploadResponse struct {
	JobID         string `json:"job_id"`
	Message       string `json:"message"`
	Filename      string `json:"filename"`
	DetectedLines string `json:"detected_lines"`
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

		// CHUNKED UPLOAD LOGIC
		// Check if this is a chunked upload
		isChunked := r.FormValue("is_chunked") == "true"
		uploadID := r.FormValue("upload_id")
		chunkIndex := r.FormValue("chunk_index")
		totalChunks := r.FormValue("total_chunks")

		var safeFilename string
		var savePath string
		var written int64

		if isChunked {
			// Chunked: We use a temporary filename based on upload_id
			if uploadID == "" {
				http.Error(w, "Missing upload_id for chunked upload", http.StatusBadRequest)
				return
			}
			safeFilename = uploadID + "_" + filepath.Base(header.Filename)
			savePath = filepath.Join("uploads", safeFilename)
			
			// Open in Append mode or Create if it's the first chunk
			flags := os.O_APPEND | os.O_CREATE | os.O_WRONLY
			if chunkIndex == "0" {
				flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
			}

			dst, err := os.OpenFile(savePath, flags, 0644)
			if err != nil {
				log.Printf("Chunk Upload Error: %v\n", err)
				http.Error(w, "Failed to open chunk file", http.StatusInternalServerError)
				return
			}
			
			// Copy chunk
			wBytes, err := io.Copy(dst, file)
			dst.Close()
			if err != nil {
				http.Error(w, "Failed to write chunk", http.StatusInternalServerError)
				return
			}
			written = wBytes

			// If this is NOT the last chunk, return success immediately (don't create job yet)
			if chunkIndex != fmt.Sprintf("%d", Atoi(totalChunks)-1) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"status": "chunk_received", "chunk": chunkIndex})
				return
			}
			
			// If it IS the last chunk, proceed to Job Creation logic below...
			log.Printf("Chunked Upload Complete: %s\n", safeFilename)

		} else {
			// STANDARD UPLOAD (Legacy / Small files)
			originalName := filepath.Base(header.Filename)
			timestamp := time.Now().Format("20060102_150405")
			safeFilename = fmt.Sprintf("%s_%s", timestamp, originalName)
			savePath = filepath.Join("uploads", safeFilename)

			// Create uploads directory if it doesn't exist
			if err := os.MkdirAll("uploads", 0755); err != nil {
				http.Error(w, "Failed to create upload directory", http.StatusInternalServerError)
				return
			}

			// Save file to disk
			dst, err := os.Create(savePath)
			if err != nil {
				http.Error(w, "Unable to create the file on server", http.StatusInternalServerError)
				return
			}
			
			wBytes, err := io.Copy(dst, file)
			dst.Close() 
			if err != nil {
				http.Error(w, "Unable to save the file content", http.StatusInternalServerError)
				return
			}
			written = wBytes
		}

		// Verify size
		log.Printf("Upload Debug: Header Size: %d, Written: %d\n", header.Size, written)
		if written != header.Size {
			log.Printf("WARNING: Upload size mismatch! Header says %d but wrote %d bytes.\n", header.Size, written)
		}

		// --- Integrity Check (Storage Verification) ---
		expectedLines := r.FormValue("expected_lines")
		expectedSize := r.FormValue("expected_size")
		actualLines := "not_found"

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
				
				// Look for last valid |9999| occurrence
				// Regex to find |9999|COUNT|
				// We search from end manually or iterate
				lines := strings.Split(tailStr, "\n")
				for i := len(lines) - 1; i >= 0; i-- {
					// STRICT CHECK: Must START with |9999| (LPAD style) to avoid false positives in descriptions
					trimmed := strings.TrimSpace(lines[i])
					if strings.HasPrefix(trimmed, "|9999|") {
						parts := strings.Split(trimmed, "|")
						// |9999|COUNT| -> index 0 is empty, 1 is 9999, 2 is COUNT
						// Check if count > 100 to ensure it's a valid SPED trailer and not a random occurrence
						if len(parts) >= 3 && parts[1] == "9999" {
							// Parse count to ensure it's numeric and reasonably large
							if countVal, err := strconv.Atoi(parts[2]); err == nil && countVal > 100 {
								actualLines = parts[2]
								break // Found the trailer, ignore subsequent garbage/signatures
							}
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
			JobID:         jobID,
			Message:       "File uploaded and saved successfully",
			Filename:      safeFilename,
			DetectedLines: actualLines,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}