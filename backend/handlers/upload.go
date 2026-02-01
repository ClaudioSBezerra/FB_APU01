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
	"strconv"
)

type UploadResponse struct {
	JobID    string `json:"job_id"`
	Message  string `json:"message"`
	Filename string `json:"filename"`
}

func castToInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
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

		// Check for Chunked Upload
		chunkIndex := r.FormValue("chunk_index")
		totalChunks := r.FormValue("total_chunks")
		uploadID := r.FormValue("upload_id")

		if chunkIndex != "" && totalChunks != "" && uploadID != "" {
			// CHUNKED UPLOAD LOGIC
			uploadDir := "uploads"
			tempDir := filepath.Join(uploadDir, "temp_chunks", uploadID)
			
			// Create temp dir
			if err := os.MkdirAll(tempDir, 0755); err != nil {
				http.Error(w, "Failed to create temp directory", http.StatusInternalServerError)
				return
			}

			// Save individual chunk
			file, _, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "Error retrieving chunk: "+err.Error(), http.StatusBadRequest)
				return
			}
			defer file.Close()

			chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%s", chunkIndex))
			dst, err := os.Create(chunkPath)
			if err != nil {
				http.Error(w, "Failed to save chunk", http.StatusInternalServerError)
				return
			}
			defer dst.Close()
			io.Copy(dst, file)

			// If last chunk, merge everything
			if chunkIndex == fmt.Sprintf("%d", castToInt(totalChunks)-1) {
				// Sanitize filename and create unique name
				// Get filename from first chunk request or current one (assuming it's passed)
				originalName := r.FormValue("filename")
				if originalName == "" {
					originalName = "upload.txt"
				}
				
				timestamp := time.Now().Format("20060102_150405")
				safeFilename := fmt.Sprintf("%s_%s", timestamp, filepath.Base(originalName))
				finalPath := filepath.Join(uploadDir, safeFilename)

				finalFile, err := os.Create(finalPath)
				if err != nil {
					http.Error(w, "Failed to create final file", http.StatusInternalServerError)
					return
				}
				// Merge chunks
				totalC := castToInt(totalChunks)
				for i := 0; i < totalC; i++ {
					cPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%d", i))
					cFile, err := os.Open(cPath)
					if err != nil {
						// Missing chunk?
						log.Printf("Error missing chunk %d: %v", i, err)
						http.Error(w, "Missing chunk during merge", http.StatusInternalServerError)
						return
					}
					io.Copy(finalFile, cFile)
					cFile.Close()
				}
				finalFile.Close() // Explicitly close to ensure flush before integrity check

				// --- Integrity Check (Chunked) ---
				expectedLines := r.FormValue("expected_lines")
				expectedSize := r.FormValue("expected_size")
				
				if fi, err := os.Stat(finalPath); err == nil {
					written := fi.Size()
					tailBuf := make([]byte, 4096)
					startPos := int64(0)
					if written > 4096 { startPos = written - 4096 }
					
					if fCheck, err := os.Open(finalPath); err == nil {
						fCheck.ReadAt(tailBuf, startPos)
						fCheck.Close()
						
						tailStr := string(tailBuf)
						actualLines := "not_found"
						lines := strings.Split(tailStr, "\n")
						for i := len(lines) - 1; i >= 0; i-- {
							if strings.Contains(lines[i], "|9999|") {
								parts := strings.Split(lines[i], "|")
								if len(parts) >= 3 && parts[1] == "9999" {
									actualLines = parts[2]
									break
								}
							}
						}
						fmt.Printf("LOG API Integrity (Chunked): File=%s Lines=%s Size=%s ActualEnd=%s\n", safeFilename, expectedLines, expectedSize, actualLines)
					}
				}
				// ---------------------------------

				// Cleanup temp dir
				os.RemoveAll(tempDir)

				// Create Job
				var jobID string
				query := `INSERT INTO import_jobs (filename, status, message) VALUES ($1, $2, $3) RETURNING id`
				err = db.QueryRow(query, safeFilename, "pending", "File uploaded via chunks").Scan(&jobID)
				if err != nil {
					http.Error(w, "Database error", http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(UploadResponse{
					JobID:    jobID,
					Message:  "Upload completed successfully (Chunked)",
					Filename: safeFilename,
				})
				return
			}

			w.WriteHeader(http.StatusOK)
			return
		}

		// STANDARD UPLOAD LOGIC (Legacy / Small Files)
		// Limit upload size:
		// ParseMultipartForm maxMemory is set to 64MB.

		// Files larger than 64MB will be stored in temporary files on disk.
		// This prevents Out-Of-Memory (OOM) errors on the VPS while allowing files of ANY size (e.g. 2GB+).
		// The actual rejection limit is handled by Nginx (client_max_body_size).
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
		defer dst.Close()

		written, err := io.Copy(dst, file)
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

		// Read last 4KB to find |9999|
		tailBuf := make([]byte, 4096)
		startPos := int64(0)
		if written > 4096 {
			startPos = written - 4096
		}
		
		// Re-open for reading
		fCheck, err := os.Open(savePath)
		if err == nil {
			defer fCheck.Close()
			_, err = fCheck.ReadAt(tailBuf, startPos)
			if err == nil || err == io.EOF {
				tailStr := string(tailBuf)
				var actualLines string = "not_found"
				
				// Simple parsing for |9999|
				lines := strings.Split(tailStr, "\n")
				for i := len(lines) - 1; i >= 0; i-- {
					line := strings.TrimSpace(lines[i])
					if strings.Contains(line, "|9999|") {
						parts := strings.Split(line, "|")
						// |9999|COUNT| -> index 0 is empty, 1 is 9999, 2 is COUNT
						if len(parts) >= 3 && parts[1] == "9999" {
							actualLines = parts[2]
							break
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