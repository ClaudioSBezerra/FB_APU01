package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

type JobStatusResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Participant struct {
	ID      string `json:"id"`
	CodPart string `json:"cod_part"`
	Nome    string `json:"nome"`
	CNPJ    string `json:"cnpj"`
	CPF     string `json:"cpf"`
	UF      string `json:"uf"` // Using cod_mun prefix or lookup ideally, but simple for now
	IE      string `json:"ie"`
}

func GetJobParticipantsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")

		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		pathParts := strings.Split(r.URL.Path, "/")
		// Expected: /api/jobs/{id}/participants
		if len(pathParts) < 5 {
			http.Error(w, "Invalid URL", http.StatusBadRequest)
			return
		}
		jobID := pathParts[3]

		rows, err := db.Query(`
			SELECT id, cod_part, nome, cnpj, cpf, ie 
			FROM participants 
			WHERE job_id = $1 
			ORDER BY nome ASC
		`, jobID)
		if err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var participants []Participant
		for rows.Next() {
			var p Participant
			if err := rows.Scan(&p.ID, &p.CodPart, &p.Nome, &p.CNPJ, &p.CPF, &p.IE); err != nil {
				continue
			}
			participants = append(participants, p)
		}

		json.NewEncoder(w).Encode(participants)
	}
}

func GetJobStatusHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// CORS Headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract ID from URL (simple path parsing)
		// Expected path: /api/jobs/{id}
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 4 {
			http.Error(w, "Invalid job ID", http.StatusBadRequest)
			return
		}
		jobID := pathParts[3]

		var job JobStatusResponse
		query := `SELECT id, status, message, created_at, updated_at FROM import_jobs WHERE id = $1`
		
		err := db.QueryRow(query, jobID).Scan(&job.ID, &job.Status, &job.Message, &job.CreatedAt, &job.UpdatedAt)
		if err == sql.ErrNoRows {
			http.Error(w, "Job not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job)
	}
}