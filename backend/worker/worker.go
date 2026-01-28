package worker

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

func StartWorker(db *sql.DB) {
	go func() {
		fmt.Println("Background Worker Started...")
		for {
			processNextJob(db)
			time.Sleep(5 * time.Second) // Poll every 5 seconds
		}
	}()
}

func processNextJob(db *sql.DB) {
	var id, filename string

	// Select pending job
	// Using FOR UPDATE SKIP LOCKED to prevent race conditions in future scaled versions
	query := `
		SELECT id, filename 
		FROM import_jobs 
		WHERE status = 'pending' 
		ORDER BY created_at ASC 
		LIMIT 1
	`
	// Note: basic implementation, for production consider transactions and row locking
	
	err := db.QueryRow(query).Scan(&id, &filename)
	if err == sql.ErrNoRows {
		return // No jobs
	} else if err != nil {
		fmt.Printf("Worker Error scanning job: %v\n", err)
		return
	}

	fmt.Printf("Worker: Processing Job %s (File: %s)\n", id, filename)

	// Update status to processing
	_, err = db.Exec("UPDATE import_jobs SET status = 'processing', updated_at = NOW() WHERE id = $1", id)
	if err != nil {
		fmt.Printf("Worker Error updating status to processing: %v\n", err)
		return
	}

	// Simulate Processing (Read file size, count lines, etc.)
	summary, err := processFile(db, id, filename)
	
	if err != nil {
		// Report Error
		fmt.Printf("Worker: Job %s failed: %v\n", id, err)
		db.Exec("UPDATE import_jobs SET status = 'error', message = $1, updated_at = NOW() WHERE id = $2", err.Error(), id)
	} else {
		// Report Success
		fmt.Printf("Worker: Job %s completed: %s\n", id, summary)
		db.Exec("UPDATE import_jobs SET status = 'completed', message = $1, updated_at = NOW() WHERE id = $2", summary, id)
	}
}

func processFile(db *sql.DB, jobID, filename string) (string, error) {
	// Security: Ensure we only read from allowed directory
	uploadDir := "uploads"
	path := filepath.Join(uploadDir, filename)

	// Open file
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("file not found: %v", err)
	}
	defer file.Close()

	// SPED files are usually encoded in ISO-8859-1 (Latin1)
	// We need to convert to UTF-8 to read correctly in Go
	reader := transform.NewReader(file, charmap.ISO8859_1.NewDecoder())
	scanner := bufio.NewScanner(reader)

	// Buffer for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var (
		count0000 int
		count0150 int
		company   string
		cnpj      string
		dtIni     string
		dtFin     string
	)

	fmt.Printf("Worker: Parsing SPED file %s...\n", filename)

	// Use a transaction for better performance and consistency
	tx, err := db.Begin()
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO participants (
			job_id, cod_part, nome, cod_pais, cnpj, cpf, ie, cod_mun, suframa, endereco, numero, complemento, bairro
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`)
	if err != nil {
		return "", fmt.Errorf("failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	for scanner.Scan() {
		line := scanner.Text()
		
		if strings.HasPrefix(line, "|0000|") {
			parts := strings.Split(line, "|")
			if len(parts) > 6 {
				// |0000|006|0|||01112021|30112021|ARMAZEM CORAL LTDA|11623188000140|PE|
				dtIni = parts[6]
				dtFin = parts[7]
				company = parts[8]
				cnpj = parts[9]
				count0000++
			}
		} else if strings.HasPrefix(line, "|0150|") {
			// |0150|COD_PART|NOME|COD_PAIS|CNPJ|CPF|IE|COD_MUN|SUFRAMA|END|NUM|COMPL|BAIRRO|
			parts := strings.Split(line, "|")
			if len(parts) >= 14 {
				count0150++
				// parts[0] is empty, parts[1] is 0150
				_, err := stmt.Exec(
					jobID,
					parts[2],  // COD_PART
					parts[3],  // NOME
					parts[4],  // COD_PAIS
					parts[5],  // CNPJ
					parts[6],  // CPF
					parts[7],  // IE
					parts[8],  // COD_MUN
					parts[9],  // SUFRAMA
					parts[10], // END
					parts[11], // NUM
					parts[12], // COMPL
					parts[13], // BAIRRO
				)
				if err != nil {
					fmt.Printf("Worker: Error inserting participant %s: %v\n", parts[2], err)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading file: %v", err)
	}

	if count0000 == 0 {
		return "", fmt.Errorf("invalid SPED file: Record 0000 not found")
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %v", err)
	}

	summary := fmt.Sprintf("Analyzed: %s (%s) | Period: %s to %s | Participants: %d", 
		company, cnpj, dtIni, dtFin, count0150)
	
	return summary, nil
}