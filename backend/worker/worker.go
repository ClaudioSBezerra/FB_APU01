package worker

import (
	// Force Git Update - Worker V4.2.2 (Chunked Upload Fix)
	"bufio"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

func StartWorker(db *sql.DB) {
	// Worker Pool Size: 2 concurrent workers (Optimized for 2 vCPU VPS)
	// Prevents CPU starvation and 504 Gateway Timeouts
	const WorkerPoolSize = 2

	fmt.Printf("Starting Background Worker Pool (%d workers)...\n", WorkerPoolSize)

	// CRASH RECOVERY: Reset any 'processing' jobs to 'pending' on startup
	// This ensures that if the server crashed, interrupted jobs are resumed automatically
	res, err := db.Exec("UPDATE import_jobs SET status = 'pending', message = message || ' [Recovered]' WHERE status = 'processing'")
	if err == nil {
		count, _ := res.RowsAffected()
		if count > 0 {
			fmt.Printf("Worker Recovery: Reset %d stuck jobs from 'processing' to 'pending'\n", count)
		}
	} else {
		fmt.Printf("Worker Recovery Error: %v\n", err)
	}

	for i := 0; i < WorkerPoolSize; i++ {
		workerID := i + 1
		go func(id int) {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Worker #%d PANIC RECOVERED: %v\n", id, r)
				}
			}()
			fmt.Printf("Worker #%d started\n", id)
			for {
				processNextJob(db, id)
				time.Sleep(2 * time.Second)
			}
		}(workerID)
	}
}

func processNextJob(db *sql.DB, workerID int) {
	var id, filename string

	// Select pending job with SKIP LOCKED to avoid race conditions
	// This allows multiple workers to pick different jobs concurrently
	query := `
		SELECT id, filename 
		FROM import_jobs 
		WHERE status = 'pending' 
		ORDER BY created_at ASC 
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`

	// We must use a transaction for FOR UPDATE SKIP LOCKED to hold the lock until update
	tx, err := db.Begin()
	if err != nil {
		fmt.Printf("Worker #%d: Error starting transaction: %v\n", workerID, err)
		return
	}
	defer tx.Rollback() // Rollback if not committed

	err = tx.QueryRow(query).Scan(&id, &filename)
	if err == sql.ErrNoRows {
		return // No jobs
	} else if err != nil {
		fmt.Printf("Worker #%d: Error scanning job: %v\n", workerID, err)
		return
	}

	fmt.Printf("Worker #%d: Processing Job %s (File: %s)\n", workerID, id, filename)

	// Update status to processing within the same transaction
	_, err = tx.Exec("UPDATE import_jobs SET status = 'processing', updated_at = NOW() WHERE id = $1", id)
	if err != nil {
		fmt.Printf("Worker #%d: Error updating status to processing: %v\n", workerID, err)
		return
	}

	// Commit the transaction to release the lock and save the status change
	if err := tx.Commit(); err != nil {
		fmt.Printf("Worker #%d: Error committing job pickup: %v\n", workerID, err)
		return
	}

	// Simulate Processing
	if err := db.Ping(); err != nil {
		fmt.Printf("Worker #%d: Database connection lost, retrying... %v\n", workerID, err)
		time.Sleep(1 * time.Second)
		if err := db.Ping(); err != nil {
			fmt.Printf("Worker #%d: Database unreachable: %v\n", workerID, err)
			return // Retry next loop
		}
	}

	summary, err := processFile(db, id, filename)

	if err != nil {
		// Report Error
		fmt.Printf("Worker #%d: Job %s failed: %v\n", workerID, id, err)
		db.Exec("UPDATE import_jobs SET status = 'error', message = $1, updated_at = NOW() WHERE id = $2", err.Error(), id)
	} else {
		// Report Success
		fmt.Printf("Worker #%d: Job %s completed: %s\n", workerID, id, summary)
		db.Exec("UPDATE import_jobs SET status = 'completed', message = $1, updated_at = NOW() WHERE id = $2", summary, id)
	}
}

func countLines(filename string) (int, error) {
	file, err := os.Open(filename)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := file.Read(buf)
		count += bytes.Count(buf[:c], lineSep)

		switch {
		case err == io.EOF:
			return count, nil

		case err != nil:
			return count, err
		}
	}
}

// validateFileIntegrity checks if the file has a valid SPED header and footer
func validateFileIntegrity(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if stat.Size() < 100 {
		return errors.New("file too small")
	}

	// 1. HEADER CHECK (Must start with |0000|) - CRITICAL
	headerBuf := make([]byte, 100)
	if _, err := f.ReadAt(headerBuf, 0); err != nil {
		return err
	}
	// Check for standard SPED header |0000|
	if !bytes.HasPrefix(headerBuf, []byte("|0000|")) {
		// Show what we found (first 20 chars)
		safeHeader := string(bytes.Map(func(r rune) rune {
			if r >= 32 && r <= 126 { return r }
			return '.'
		}, headerBuf[:20]))
		return fmt.Errorf("invalid SPED format: missing '|0000|' header. Found: [%s]...", safeHeader)
	}

	// 2. TAIL CHECK (Should end with |9999|) - WARNING ONLY
	// Read last 16KB (to skip Digital Signatures)
	bufSize := int64(16384)
	if stat.Size() < bufSize {
		bufSize = stat.Size()
	}
	buf := make([]byte, bufSize)
	_, err = f.ReadAt(buf, stat.Size()-bufSize)
	if err != nil && err != io.EOF {
		return err
	}

	// Check for |9999| (ANSI/UTF-8) OR |D990| (Phase 2 DEV - Client Side Filter End)
	// Note: It might be |9999| or |9999|CRLF or |9999|LF
	if !bytes.Contains(buf, []byte("|9999|")) && !bytes.Contains(buf, []byte("|D990|")) {
		// Try checking for UTF-16LE pattern (common in Windows "Unicode" files)
		// | (7C 00) 9 (39 00) 9 (39 00) 9 (39 00) 9 (39 00) | (7C 00)
		utf16le := []byte{0x7C, 0x00, 0x39, 0x00, 0x39, 0x00, 0x39, 0x00, 0x39, 0x00, 0x7C, 0x00}
		if bytes.Contains(buf, utf16le) {
			fmt.Println("Worker: Detected UTF-16LE encoding with valid footer.")
			return nil
		}

		// Downgrade to WARNING to allow processing "partial" files or weird encodings
		tailLen := 100
		if len(buf) < tailLen {
			tailLen = len(buf)
		}
		actualTail := string(buf[len(buf)-tailLen:])
		fmt.Printf("Worker WARNING: Missing trailing '|9999|' or '|D990|' record. Tail: [%q]. Continuing anyway...\n", actualTail)
		
		// return nil to allow processing
		return nil
	}
	return nil
}

func parseDate(s string) interface{} {
	if len(s) != 8 {
		return nil
	}
	// DDMMYYYY -> YYYY-MM-DD
	return s[4:8] + "-" + s[2:4] + "-" + s[0:2]
}

func parseDecimal(s string) float64 {
	if s == "" {
		return 0.0
	}
	s = strings.ReplaceAll(s, ",", ".")
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func processFile(db *sql.DB, jobID, filename string) (string, error) {
	// Verify DB connection before starting
	if err := db.Ping(); err != nil {
		return "", fmt.Errorf("database connection lost before start: %v", err)
	}

	// Auto-Migrate: Ensure last_line_processed column exists
	// This allows us to resume processing if it crashes
	_, err := db.Exec(`ALTER TABLE import_jobs ADD COLUMN IF NOT EXISTS last_line_processed INT DEFAULT 0`)
	if err != nil {
		fmt.Printf("Worker: Warning checking checkpoint column: %v\n", err)
	}

	// CHECKPOINT: Check if we are resuming a job
	var lastLineProcessed int
	err = db.QueryRow("SELECT COALESCE(last_line_processed, 0) FROM import_jobs WHERE id = $1", jobID).Scan(&lastLineProcessed)
	if err == nil && lastLineProcessed > 0 {
		fmt.Printf("Worker: RESUMING job %s from line %d\n", jobID, lastLineProcessed)
	} else {
		lastLineProcessed = 0
	}

	// Security: Ensure we only read from allowed directory
	uploadDir := "uploads"
	path := filepath.Join(uploadDir, filename)

	// 1. PRE-VALIDATION: Check if file is truncated (must end with |9999|)
	if err := validateFileIntegrity(path); err != nil {
		fmt.Printf("Worker: File integrity check failed: %v\n", err)
		db.Exec("UPDATE import_jobs SET status='error', message=$1, updated_at=NOW() WHERE id=$2", "File Corrupted/Truncated: "+err.Error(), jobID)
		return "File Integrity Failed", err
	}

	// Open file
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("file not found: %v", err)
	}
	defer file.Close()

	// Get file size for progress calculation
	if fi, err := file.Stat(); err == nil {
		fmt.Printf("Worker: Processing %s (Size: %.2f MB)\n", path, float64(fi.Size())/1024/1024)
	}

	// SPED files are usually encoded in ISO-8859-1 (Latin1)
	reader := transform.NewReader(file, charmap.ISO8859_1.NewDecoder())
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var lineCount int

	// SKIP LINES logic for Resume
	if lastLineProcessed > 0 {
		fmt.Printf("Worker: Skipping first %d lines to resume...\n", lastLineProcessed)
		linesSkipped := 0
		for linesSkipped < lastLineProcessed && scanner.Scan() {
			linesSkipped++
		}
		lineCount = lastLineProcessed
	}

	var (
		count0000, count0150, countC100, countC190, countC500, countC600, countD100, countD500 int
		company, filialCNPJ, dtIni, dtFin, currentC100ID                                       string
		rates                                                                                  TaxRates
		debugLog                                                                               strings.Builder
		foundEOF                                                                               bool
	)

	fmt.Printf("Worker: Parsing SPED file %s (EFD ICMS Logic - Fixed Indices)...\n", filename)
	fmt.Println("Worker: VERSION 4.2.2 - CHUNKED UPLOAD SUPPORT")

	// Get file info for size
	fileInfo, err := file.Stat()
	if err == nil {
		fmt.Printf("Worker: File Size: %d bytes (%.2f MB)\n", fileInfo.Size(), float64(fileInfo.Size())/1024/1024)
	}

	// Count total lines for progress tracking
	totalLines, err := countLines(path)
	if err != nil {
		fmt.Printf("Worker: Warning counting lines: %v\n", err)
		totalLines = 0
	}
	fmt.Printf("Worker: Total lines to process: %d\n", totalLines)

	// Warning check for empty CFOP table
	var cfopCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM cfop").Scan(&cfopCount); err == nil && cfopCount == 0 {
		warnMsg := " [WARNING: Tabela CFOP vazia! Agregações podem falhar]"
		fmt.Println("Worker:" + warnMsg)
		db.Exec("UPDATE import_jobs SET message = message || $1 WHERE id=$2", warnMsg, jobID)
	}

	// BATCH PROCESSING SETUP
	// Instead of one huge transaction, we commit every BatchSize lines.
	const BatchSize = 2000
	var tx *sql.Tx
	var stmtPart, stmtC100, stmtC190, stmtC500, stmtC600, stmtD100, stmtD500 *sql.Stmt

	// Initial dummy participants (outside batch loop for simplicity, or inside first batch)
	// We'll do it quickly in a separate mini-tx to ensure they exist
	if _, err := db.Exec(`INSERT INTO participants (job_id, cod_part, nome, cnpj, cpf) VALUES ($1, '9999999999', 'CONSUMIDOR FINAL', '', ''), ($1, '8888888888', 'FORNECEDOR GENERICO', '', '') ON CONFLICT DO NOTHING`, jobID); err != nil {
		fmt.Printf("Worker: Warning inserting dummy participants: %v\n", err)
	}

	// Helper to start a new batch transaction
	startBatch := func() error {
		var err error
		// Retry logic for starting transaction AND preparing statements
		for i := 0; i < 5; i++ {
			// Reset statements to nil before attempt
			stmtPart = nil
			stmtC100 = nil
			stmtC190 = nil
			stmtC500 = nil
			stmtC600 = nil
			stmtD100 = nil
			stmtD500 = nil

			err = func() error {
				tx, err = db.Begin()
				if err != nil {
					return fmt.Errorf("db.Begin: %w", err)
				}

				stmtPart, err = tx.Prepare(`INSERT INTO participants (job_id, cod_part, nome, cod_pais, cnpj, cpf, ie, cod_mun, suframa, endereco, numero, complemento, bairro) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`)
				if err != nil {
					return fmt.Errorf("prepare stmtPart: %w", err)
				}

				stmtC100, err = tx.Prepare(`INSERT INTO reg_c100 (job_id, filial_cnpj, ind_oper, ind_emit, cod_part, cod_mod, cod_sit, ser, num_doc, chv_nfe, dt_doc, dt_e_s, vl_doc, vl_icms, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20) RETURNING id`)
				if err != nil {
					return fmt.Errorf("prepare stmtC100: %w", err)
				}

				stmtC190, err = tx.Prepare(`INSERT INTO reg_c190 (job_id, id_pai_c100, cfop, vl_opr, vl_bc_icms, vl_icms, vl_bc_icms_st, vl_icms_st, vl_red_bc, vl_ipi, cod_obs) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`)
				if err != nil {
					return fmt.Errorf("prepare stmtC190: %w", err)
				}

				stmtC500, err = tx.Prepare(`INSERT INTO reg_c500 (job_id, filial_cnpj, cod_part, cod_mod, ser, num_doc, dt_doc, dt_e_s, vl_doc, vl_icms, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`)
				if err != nil {
					return fmt.Errorf("prepare stmtC500: %w", err)
				}

				stmtC600, err = tx.Prepare(`INSERT INTO reg_c600 (job_id, filial_cnpj, cod_mod, cod_mun, ser, sub, cod_cons, qtd_cons, dt_doc, vl_doc, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`)
				if err != nil {
					return fmt.Errorf("prepare stmtC600: %w", err)
				}

				stmtD100, err = tx.Prepare(`INSERT INTO reg_d100 (job_id, filial_cnpj, ind_oper, ind_emit, cod_part, cod_mod, cod_sit, ser, num_doc, chv_cte, dt_doc, dt_a_p, vl_doc, vl_icms, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`)
				if err != nil {
					return fmt.Errorf("prepare stmtD100: %w", err)
				}

				stmtD500, err = tx.Prepare(`INSERT INTO reg_d500 (job_id, filial_cnpj, ind_oper, ind_emit, cod_part, cod_mod, cod_sit, ser, sub, num_doc, dt_doc, dt_a_p, vl_doc, vl_icms, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`)
				if err != nil {
					return fmt.Errorf("prepare stmtD500: %w", err)
				}

				return nil
			}()

			if err == nil {
				return nil
			}

			// Cleanup on failure
			if stmtPart != nil {
				stmtPart.Close()
			}
			if stmtC100 != nil {
				stmtC100.Close()
			}
			if stmtC190 != nil {
				stmtC190.Close()
			}
			if stmtC500 != nil {
				stmtC500.Close()
			}
			if stmtC600 != nil {
				stmtC600.Close()
			}
			if stmtD100 != nil {
				stmtD100.Close()
			}
			if stmtD500 != nil {
				stmtD500.Close()
			}
			if tx != nil {
				tx.Rollback()
			}

			fmt.Printf("Worker: Batch setup failed (attempt %d/5): %v. Retrying in %ds...\n", i+1, err, i+1)
			time.Sleep(time.Duration(i+1) * time.Second)
			db.Ping() // Try to reconnect
		}
		return fmt.Errorf("failed to begin transaction and prepare statements after retries: %v", err)
	}

	// Helper to commit the current batch
	commitBatch := func() error {
		// Flush CopyIn statements
		if stmtD100 != nil {
			// stmtD100 now uses standard INSERT, no need to Exec() for flush, just Close()
			stmtD100.Close()
		}
		if stmtD500 != nil {
			// stmtD500 now uses standard INSERT, no need to Exec() for flush, just Close()
			stmtD500.Close()
		}

		// Close other statements
		if stmtPart != nil {
			stmtPart.Close()
		}
		if stmtC100 != nil {
			stmtC100.Close()
		}
		if stmtC190 != nil {
			stmtC190.Close()
		}
		if stmtC500 != nil {
			stmtC500.Close()
		}
		if stmtC600 != nil {
			stmtC600.Close()
		}

		// Commit transaction
		if tx != nil {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("tx.Commit: %w", err)
			}
		}
		return nil
	}

	// Start first batch
	if err := startBatch(); err != nil {
		return "", err
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines or too short lines
		if len(line) < 7 {
			continue
		}

		// Optimize: Read first 6 chars to identify register (e.g. "|0000|")
		// This avoids strings.Split() for lines we don't care about
		if line[0] != '|' || line[5] != '|' {
			continue
		}
		reg := line[1:5]

		lineCount++

		// Progress Update & Checkpoint
		if lineCount%1000 == 0 || lineCount%BatchSize == 0 {
			// Console Log for Real-time tracking with RAM usage
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			fmt.Printf("Worker: Line %d / %d (%.1f%%) | RAM: %d MB | Reg: %s\n", 
				lineCount, totalLines, float64(lineCount)/float64(totalLines)*100, m.Alloc/1024/1024, reg)
		}

		// Check for 9999 OR D990 (End of File)
		if reg == "9999" || reg == "D990" {
			foundEOF = true
			fmt.Printf("Worker: Found End of File (%s) at line %d\n", reg, lineCount)
		}

		if lineCount%BatchSize == 0 {
			// Check for cancellation
			var currentStatus string
			if err := db.QueryRow("SELECT status FROM import_jobs WHERE id=$1", jobID).Scan(&currentStatus); err == nil {
				if currentStatus == "cancelling" {
					tx.Rollback()
					return "", fmt.Errorf("job cancelled by user")
				}
			}

			// Update Progress
			// We only split here for logging purposes if needed, or just use reg
			var msg string
			if totalLines > 0 {
				percent := float64(lineCount) / float64(totalLines) * 100
				msg = fmt.Sprintf("Processing line %d / %d (%.1f%%) - Reg %s (Batch Commit)...", lineCount, totalLines, percent, reg)
			} else {
				msg = fmt.Sprintf("Processing line %d (Reg %s)...", lineCount, reg)
			}
			fmt.Printf("Worker: %s\n", msg)
			db.Exec("UPDATE import_jobs SET message=$1, updated_at=NOW() WHERE id=$2", msg, jobID)

			// COMMIT BATCH AND RESTART
			if err := commitBatch(); err != nil {
				return "", fmt.Errorf("batch commit failed at line %d: %v", lineCount, err)
			}

			// Update Checkpoint in DB
			_, err = db.Exec("UPDATE import_jobs SET last_line_processed = $1, updated_at = NOW() WHERE id = $2", lineCount, jobID)
			if err != nil {
				fmt.Printf("Worker: Warning updating checkpoint: %v\n", err)
			}

			// THROTTLE: Sleep 50ms to allow HTTP requests to be processed (Prevents 504 Timeout on 2 vCPU)
			time.Sleep(50 * time.Millisecond)

			if err := startBatch(); err != nil {
				return "", fmt.Errorf("failed to restart batch at line %d: %v", lineCount, err)
			}
		}

		// Only Split if it is a register we process
		// We use switch/case on 'reg' for O(1) dispatch instead of if/else chain
		switch reg {
		case "0000":
			parts := strings.Split(line, "|")
			if len(parts) >= 8 {
				dtIni = parts[4]
				dtFin = parts[5]
				company = parts[6]
				// Sanitize CNPJ (remove ., /, -) to fit VARCHAR(14)
				filialCNPJ = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(parts[7], ".", ""), "/", ""), "-", "")
				count0000++
				// Update job metadata immediately (outside tx for visibility)
				db.Exec("UPDATE import_jobs SET company_name=$1, cnpj=$2, dt_ini=$3, dt_fin=$4 WHERE id=$5", company, filialCNPJ, parseDate(dtIni), parseDate(dtFin), jobID)

				if len(dtIni) == 8 {
					year, _ := strconv.Atoi(dtIni[4:8])
					if r, err := getTaxRates(db, year); err == nil {
						rates = r
					}
				}
			}
		case "0150":
			parts := strings.Split(line, "|")
			if len(parts) >= 14 {
				count0150++
				stmtPart.Exec(jobID, parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[8], parts[9], parts[10], parts[11], parts[12], parts[13])
			}
		case "C100":
			parts := strings.Split(line, "|")
			if len(parts) >= 29 {
				countC100++
				vlDoc := parseDecimal(parts[12])
				vlIcms := parseDecimal(parts[22])
				vlPis := parseDecimal(parts[26])
				vlCofins := parseDecimal(parts[27])
				vlIcmsProj := vlIcms * (1 - (rates.PercReducICMS / 100.0))
				vlIbsProj := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
				vlCbsProj := vlDoc * (rates.PercCBS / 100.0)

				stmtC100.QueryRow(jobID, filialCNPJ, parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[8], parts[9], parseDate(parts[10]), parseDate(parts[11]), vlDoc, vlIcms, vlPis, vlCofins, vlPis+vlCofins, vlIcmsProj, vlIbsProj, vlCbsProj).Scan(&currentC100ID)
			}
		case "C190":
			parts := strings.Split(line, "|")
			if len(parts) >= 12 && currentC100ID != "" {
				countC190++
				stmtC190.Exec(jobID, currentC100ID, parts[3], parseDecimal(parts[5]), parseDecimal(parts[6]), parseDecimal(parts[7]), parseDecimal(parts[8]), parseDecimal(parts[9]), parseDecimal(parts[10]), parseDecimal(parts[11]), parts[12])
			}
		case "C500":
			parts := strings.Split(line, "|")
			// C500 Layout (User Defined Indices)
			// 4: COD_PART, 11: DT_DOC, 13: VL_DOC, 20: VL_ICMS, 24: VL_PIS, 25: VL_COFINS

			if len(parts) < 14 {
				msg := fmt.Sprintf(" [DEBUG: C500 skipped (len=%d < 14)]", len(parts))
				fmt.Println(msg)
				if debugLog.Len() < 1000 {
					debugLog.WriteString(msg)
				}
			} else {
				countC500++

				// Standard Indices based on CodPart at 4
				codPart := parts[4]
				codMod := parts[5]
				ser := parts[7]
				numDoc := parts[10]
				dtDoc := parseDate(parts[11])
				dtES := parseDate(parts[12])
				vlDoc := parseDecimal(parts[13])

				vlIcms := 0.0
				if len(parts) > 20 {
					vlIcms = parseDecimal(parts[20])
				}

				vlPis := 0.0
				vlCofins := 0.0
				if len(parts) > 25 {
					vlPis = parseDecimal(parts[24])
					vlCofins = parseDecimal(parts[25])
				}

				// DEBUG First 5 C500s
				if countC500 <= 5 {
					msg := fmt.Sprintf(" [DEBUG C500 #%d: NumDoc=%s, VlDoc=%.2f, VlIcms=%.2f]", countC500, numDoc, vlDoc, vlIcms)
					fmt.Println(msg)
					if debugLog.Len() < 2000 {
						debugLog.WriteString(msg)
					}
				}

				vlIcmsProj := vlIcms * (1 - (rates.PercReducICMS / 100.0))
				vlIbsProj := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
				vlCbsProj := vlDoc * (rates.PercCBS / 100.0)

				if _, err := stmtC500.Exec(jobID, filialCNPJ, codPart, codMod, ser, numDoc, dtDoc, dtES, vlDoc, vlIcms, vlPis, vlCofins, vlPis+vlCofins, vlIcmsProj, vlIbsProj, vlCbsProj); err != nil {
					fmt.Printf("Worker: Error inserting C500 line %d: %v\n", lineCount, err)
				}
			}
		case "C600":
			parts := strings.Split(line, "|")
			if len(parts) >= 10 {
				countC600++
				vlDoc := parseDecimal(parts[9])
				vlIbsProj := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
				vlCbsProj := vlDoc * (rates.PercCBS / 100.0)
				stmtC600.Exec(jobID, filialCNPJ, parts[2], parts[3], parts[4], parts[5], parts[6], 0.0, parseDate(parts[8]), vlDoc, 0.0, 0.0, 0.0, 0.0, vlIbsProj, vlCbsProj)
			}
		case "D100":
			parts := strings.Split(line, "|")
			if len(parts) >= 13 {
				countD100++
				vlDoc := parseDecimal(parts[12])
				vlIcms := 0.0
				if len(parts) > 22 {
					vlIcms = parseDecimal(parts[22])
				}
				vlPis := 0.0
				vlCofins := 0.0
				if len(parts) > 26 {
					vlPis = parseDecimal(parts[25])
					vlCofins = parseDecimal(parts[26])
				}

				vlIcmsProj := vlIcms * (1 - (rates.PercReducICMS / 100.0))
				vlIbsProj := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
				vlCbsProj := vlDoc * (rates.PercCBS / 100.0)
				stmtD100.Exec(jobID, filialCNPJ, parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[9], "", parseDate(parts[10]), parseDate(parts[11]), vlDoc, vlIcms, vlPis, vlCofins, vlPis+vlCofins, vlIcmsProj, vlIbsProj, vlCbsProj)
			}
		case "D500":
			parts := strings.Split(line, "|")
			if len(parts) < 13 {
				msg := fmt.Sprintf(" [DEBUG: D500 skipped (len=%d < 13)]", len(parts))
				fmt.Println(msg)
				if debugLog.Len() < 1000 {
					debugLog.WriteString(msg)
				}
			} else {
				countD500++

				vlDoc := parseDecimal(parts[12])
				vlIcms := 0.0
				if len(parts) > 19 {
					vlIcms = parseDecimal(parts[19])
				}

				vlPis := 0.0
				vlCofins := 0.0
				if len(parts) > 22 {
					vlPis = parseDecimal(parts[21])
					vlCofins = parseDecimal(parts[22])
				}

				// DEBUG First 5 D500s
				if countD500 <= 5 {
					msg := fmt.Sprintf(" [DEBUG D500 #%d: NumDoc=%s, VlDoc=%.2f, VlIcms=%.2f]", countD500, parts[9], vlDoc, vlIcms)
					fmt.Println(msg)
					if debugLog.Len() < 2000 {
						debugLog.WriteString(msg)
					}
				}

				vlIcmsProj := vlIcms * (1 - (rates.PercReducICMS / 100.0))
				vlIbsProj := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
				vlCbsProj := vlDoc * (rates.PercCBS / 100.0)

				stmtD500.Exec(jobID, filialCNPJ, parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[8], parts[9], parseDate(parts[10]), parseDate(parts[11]), vlDoc, vlIcms, vlPis, vlCofins, vlPis+vlCofins, vlIcmsProj, vlIbsProj, vlCbsProj)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading file: %v", err)
	}
	if count0000 == 0 {
		return "", fmt.Errorf("invalid SPED file: Record 0000 not found")
	}
	if !foundEOF {
		fmt.Println("Worker: WARNING - File ended without '9999' record! File is likely truncated.")
		debugLog.WriteString(" [WARNING: TRUNCATED FILE - NO 9999 RECORD]")
	}

	// Final Batch Commit
	if err := commitBatch(); err != nil {
		return "", fmt.Errorf("final batch commit failed: %v", err)
	}

	// Run Aggregations (New Transaction)
	fmt.Println("Worker: Running aggregations (Database intensive)...")
	db.Exec("UPDATE import_jobs SET message='Running Aggregations (Database intensive)...' WHERE id=$1", jobID)

	// Aggregation Transaction
	aggTx, err := db.Begin()
	if err != nil {
		return "", fmt.Errorf("failed to begin aggregation tx: %v", err)
	}
	defer aggTx.Rollback()

	if err := runAggregations(aggTx, jobID, rates); err != nil {
		fmt.Printf("Worker: Error running aggregations: %v\n", err)
		return "", err
	}
	if err := aggTx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit aggregations: %v", err)
	}

	// Verify actual insertions
	var dbCountC100, dbCountC500, dbCountD100, dbCountD500 int
	db.QueryRow("SELECT COUNT(*) FROM reg_c100 WHERE job_id=$1", jobID).Scan(&dbCountC100)
	db.QueryRow("SELECT COUNT(*) FROM reg_c500 WHERE job_id=$1", jobID).Scan(&dbCountC500)
	db.QueryRow("SELECT COUNT(*) FROM reg_d100 WHERE job_id=$1", jobID).Scan(&dbCountD100)
	db.QueryRow("SELECT COUNT(*) FROM reg_d500 WHERE job_id=$1", jobID).Scan(&dbCountD500)

	return fmt.Sprintf("Imported: 0000=%d, 0150=%d, C100=%d(DB:%d), C190=%d, C500=%d(DB:%d), C600=%d, D100=%d(DB:%d), D500=%d(DB:%d)%s",
		count0000, count0150, countC100, dbCountC100, countC190, countC500, dbCountC500, countC600, countD100, dbCountD100, countD500, dbCountD500, debugLog.String()), nil
}

func runAggregations(tx *sql.Tx, jobID string, rates TaxRates) error {
	// 1. Operacoes Comerciais
	_, err := tx.Exec(`
		INSERT INTO operacoes_comerciais (
			job_id, filial_cnpj, cod_part, mes_ano, ind_oper, 
			vl_doc, vl_icms, vl_icms_projetado, vl_piscofins, vl_ibs_projetado, vl_cbs_projetado
		)
		SELECT 
			c100.job_id,
			c100.filial_cnpj,
			c100.cod_part,
			TO_CHAR(c100.dt_doc, 'MM/YYYY'),
			c100.ind_oper,
			SUM(c100.vl_doc),
			SUM(c100.vl_icms),
			SUM(c100.vl_icms * (1 - ($2::float8 / 100.0))),
			SUM(c100.vl_piscofins),
			SUM(c100.vl_doc * (($3::float8 + $4::float8) / 100.0)),
			SUM(c100.vl_doc * ($5::float8 / 100.0))
		FROM reg_c100 c100
		WHERE c100.job_id = $1
		AND EXISTS (
			SELECT 1 FROM reg_c190 c190
			JOIN cfop c ON c190.cfop = c.cfop
			WHERE c190.id_pai_c100 = c100.id
			AND c.tipo IN ('S', 'R')
		)
		GROUP BY c100.job_id, c100.filial_cnpj, c100.cod_part, TO_CHAR(c100.dt_doc, 'MM/YYYY'), c100.ind_oper
	`, jobID, rates.PercReducICMS, rates.PercIBS_UF, rates.PercIBS_Mun, rates.PercCBS)
	if err != nil {
		return fmt.Errorf("aggregation operacoes_comerciais failed: %v", err)
	}

	// 2. Energia C500
	_, err = tx.Exec(`
		INSERT INTO energia_agregado (
			job_id, filial_cnpj, cod_part, mes_ano, ind_oper, 
			vl_doc, vl_icms, vl_icms_projetado, vl_piscofins, vl_ibs_projetado, vl_cbs_projetado
		)
		SELECT 
			job_id, filial_cnpj, cod_part, TO_CHAR(dt_doc, 'MM/YYYY'), '0',
			SUM(vl_doc), SUM(vl_icms), SUM(vl_icms * (1 - ($2::float8 / 100.0))), SUM(vl_piscofins),
			SUM(vl_doc * (($3::float8 + $4::float8) / 100.0)), SUM(vl_doc * ($5::float8 / 100.0))
		FROM reg_c500
		WHERE job_id = $1
		GROUP BY job_id, filial_cnpj, cod_part, TO_CHAR(dt_doc, 'MM/YYYY')
	`, jobID, rates.PercReducICMS, rates.PercIBS_UF, rates.PercIBS_Mun, rates.PercCBS)
	if err != nil {
		return fmt.Errorf("aggregation energia C500 failed: %v", err)
	}

	// 3. Energia C600
	_, err = tx.Exec(`
		INSERT INTO energia_agregado (
			job_id, filial_cnpj, cod_part, mes_ano, ind_oper, 
			vl_doc, vl_icms, vl_icms_projetado, vl_piscofins, vl_ibs_projetado, vl_cbs_projetado
		)
		SELECT 
			job_id, filial_cnpj, 'CONSUMIDOR', TO_CHAR(dt_doc, 'MM/YYYY'), '1',
			SUM(vl_doc), 0, 0, SUM(vl_piscofins),
			SUM(vl_doc * (($2::float8 + $3::float8) / 100.0)), SUM(vl_doc * ($4::float8 / 100.0))
		FROM reg_c600
		WHERE job_id = $1
		GROUP BY job_id, filial_cnpj, TO_CHAR(dt_doc, 'MM/YYYY')
	`, jobID, rates.PercIBS_UF, rates.PercIBS_Mun, rates.PercCBS)
	if err != nil {
		return fmt.Errorf("aggregation energia C600 failed: %v", err)
	}

	// 4. Frete (D100)
	_, err = tx.Exec(`
		INSERT INTO frete_agregado (
			job_id, filial_cnpj, cod_part, mes_ano, ind_oper, 
			vl_doc, vl_icms, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado
		)
		SELECT 
			job_id, filial_cnpj, cod_part, TO_CHAR(dt_doc, 'MM/YYYY'), ind_oper,
			SUM(vl_doc), SUM(vl_icms), SUM(vl_icms * (1 - ($2::float8 / 100.0))),
			SUM(vl_doc * (($3::float8 + $4::float8) / 100.0)), SUM(vl_doc * ($5::float8 / 100.0))
		FROM reg_d100
		WHERE job_id = $1
		GROUP BY job_id, filial_cnpj, cod_part, TO_CHAR(dt_doc, 'MM/YYYY'), ind_oper
	`, jobID, rates.PercReducICMS, rates.PercIBS_UF, rates.PercIBS_Mun, rates.PercCBS)
	if err != nil {
		return fmt.Errorf("aggregation frete failed: %v", err)
	}

	// 5. Comunicacoes (D500)
	_, err = tx.Exec(`
		INSERT INTO comunicacoes_agregado (
			job_id, filial_cnpj, cod_part, mes_ano, ind_oper, 
			vl_doc, vl_icms, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado
		)
		SELECT 
			job_id, filial_cnpj, cod_part, TO_CHAR(dt_doc, 'MM/YYYY'), ind_oper,
			SUM(vl_doc), SUM(vl_icms), SUM(vl_icms * (1 - ($2::float8 / 100.0))),
			SUM(vl_doc * (($3::float8 + $4::float8) / 100.0)), SUM(vl_doc * ($5::float8 / 100.0))
		FROM reg_d500
		WHERE job_id = $1
		GROUP BY job_id, filial_cnpj, cod_part, TO_CHAR(dt_doc, 'MM/YYYY'), ind_oper
	`, jobID, rates.PercReducICMS, rates.PercIBS_UF, rates.PercIBS_Mun, rates.PercCBS)
	if err != nil {
		return fmt.Errorf("aggregation comunicacoes failed: %v", err)
	}

	return nil
}

type TaxRates struct {
	PercIBS_UF         float64
	PercIBS_Mun        float64
	PercCBS            float64
	PercReducICMS      float64
	PercReducPisCofins float64
}

func getTaxRates(db *sql.DB, year int) (TaxRates, error) {
	var r TaxRates
	// Default values if not found or error
	r.PercIBS_UF = 0.05
	r.PercIBS_Mun = 0.05
	r.PercCBS = 8.80
	r.PercReducICMS = 0.0
	r.PercReducPisCofins = 100.0

	query := `SELECT perc_ibs_uf, perc_ibs_mun, perc_cbs, perc_reduc_icms, perc_reduc_piscofins FROM tabela_aliquotas WHERE ano = $1`
	err := db.QueryRow(query, year).Scan(&r.PercIBS_UF, &r.PercIBS_Mun, &r.PercCBS, &r.PercReducICMS, &r.PercReducPisCofins)
	if err == sql.ErrNoRows {
		return r, nil // Return defaults
	}
	return r, err
}
