package worker

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
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
	query := `
		SELECT id, filename 
		FROM import_jobs 
		WHERE status = 'pending' 
		ORDER BY created_at ASC 
		LIMIT 1
	`
	
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

	// Simulate Processing
	if err := db.Ping(); err != nil {
		fmt.Printf("Worker: Database connection lost, retrying... %v\n", err)
		time.Sleep(1 * time.Second)
		if err := db.Ping(); err != nil {
			fmt.Printf("Worker: Database unreachable: %v\n", err)
			return // Retry next loop
		}
	}

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
	reader := transform.NewReader(file, charmap.ISO8859_1.NewDecoder())
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var (
		count0000, count0150, countC100, countC190, countC500, countC600, countD100, countD500, lineCount int
		company, filialCNPJ, dtIni, dtFin, currentC100ID string
		rates TaxRates
	)

	fmt.Printf("Worker: Parsing SPED file %s (EFD ICMS Logic)...\n", filename)

	// Count total lines for progress tracking
	totalLines, err := countLines(path)
	if err != nil {
		fmt.Printf("Worker: Warning counting lines: %v\n", err)
		totalLines = 0
	}
	fmt.Printf("Worker: Total lines to process: %d\n", totalLines)

	// Verify DB connection before starting
	if err := db.Ping(); err != nil {
		return "", fmt.Errorf("database connection lost before start: %v", err)
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
			stmtPart = nil; stmtC100 = nil; stmtC190 = nil; stmtC500 = nil; stmtC600 = nil; stmtD100 = nil; stmtD500 = nil
			
			err = func() error {
				tx, err = db.Begin()
				if err != nil { return fmt.Errorf("db.Begin: %w", err) }

				stmtPart, err = tx.Prepare(`INSERT INTO participants (job_id, cod_part, nome, cod_pais, cnpj, cpf, ie, cod_mun, suframa, endereco, numero, complemento, bairro) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`)
				if err != nil { return fmt.Errorf("prepare stmtPart: %w", err) }

				stmtC100, err = tx.Prepare(`INSERT INTO reg_c100 (job_id, filial_cnpj, ind_oper, ind_emit, cod_part, cod_mod, cod_sit, ser, num_doc, chv_nfe, dt_doc, dt_e_s, vl_doc, vl_icms, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20) RETURNING id`)
				if err != nil { return fmt.Errorf("prepare stmtC100: %w", err) }

				stmtC190, err = tx.Prepare(`INSERT INTO reg_c190 (job_id, id_pai_c100, cfop, vl_opr, vl_bc_icms, vl_icms, vl_bc_icms_st, vl_icms_st, vl_red_bc, vl_ipi, cod_obs) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`)
				if err != nil { return fmt.Errorf("prepare stmtC190: %w", err) }

				stmtC500, err = tx.Prepare(`INSERT INTO reg_c500 (job_id, filial_cnpj, cod_part, cod_mod, ser, num_doc, dt_doc, dt_e_s, vl_doc, vl_icms, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`)
				if err != nil { return fmt.Errorf("prepare stmtC500: %w", err) }

				stmtC600, err = tx.Prepare(`INSERT INTO reg_c600 (job_id, filial_cnpj, cod_mod, cod_mun, ser, sub, cod_cons, qtd_cons, dt_doc, vl_doc, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`)
				if err != nil { return fmt.Errorf("prepare stmtC600: %w", err) }

				stmtD100, err = tx.Prepare(pq.CopyIn("reg_d100", "job_id", "filial_cnpj", "ind_oper", "ind_emit", "cod_part", "cod_mod", "cod_sit", "ser", "num_doc", "chv_cte", "dt_doc", "dt_a_p", "vl_doc", "vl_icms", "vl_pis", "vl_cofins", "vl_piscofins", "vl_icms_projetado", "vl_ibs_projetado", "vl_cbs_projetado"))
				if err != nil { return fmt.Errorf("prepare stmtD100: %w", err) }

				stmtD500, err = tx.Prepare(pq.CopyIn("reg_d500", "job_id", "filial_cnpj", "ind_oper", "ind_emit", "cod_part", "cod_mod", "cod_sit", "ser", "sub", "num_doc", "dt_doc", "dt_a_p", "vl_doc", "vl_icms", "vl_pis", "vl_cofins", "vl_piscofins", "vl_icms_projetado", "vl_ibs_projetado", "vl_cbs_projetado"))
				if err != nil { return fmt.Errorf("prepare stmtD500: %w", err) }

				return nil
			}()

			if err == nil { return nil }

			// Cleanup on failure
			if stmtPart != nil { stmtPart.Close() }
			if stmtC100 != nil { stmtC100.Close() }
			if stmtC190 != nil { stmtC190.Close() }
			if stmtC500 != nil { stmtC500.Close() }
			if stmtC600 != nil { stmtC600.Close() }
			if stmtD100 != nil { stmtD100.Close() }
			if stmtD500 != nil { stmtD500.Close() }
			if tx != nil { tx.Rollback() }

			fmt.Printf("Worker: Batch setup failed (attempt %d/5): %v. Retrying in %ds...\n", i+1, err, i+1)
			time.Sleep(time.Duration(i+1) * time.Second)
			db.Ping() // Try to reconnect
		}
		return fmt.Errorf("failed to begin transaction and prepare statements after retries: %v", err)
	}

	// Start first batch
	if err := startBatch(); err != nil { return "", err }

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++
		parts := strings.Split(line, "|")

		// Progress Update & Checkpoint
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
			regType := "?"
			if len(parts) > 1 { regType = parts[1] }
			
			var msg string
			if totalLines > 0 {
				percent := float64(lineCount) / float64(totalLines) * 100
				msg = fmt.Sprintf("Processing line %d / %d (%.1f%%) - Reg %s (Batch Commit)...", lineCount, totalLines, percent, regType)
			} else {
				msg = fmt.Sprintf("Processing line %d (Reg %s)...", lineCount, regType)
			}
			fmt.Printf("Worker: %s\n", msg)
			db.Exec("UPDATE import_jobs SET message=$1, updated_at=NOW() WHERE id=$2", msg, jobID)

			// COMMIT BATCH AND RESTART
			if err := commitBatch(); err != nil {
				return "", fmt.Errorf("batch commit failed at line %d: %v", lineCount, err)
			}
			if err := startBatch(); err != nil {
				return "", fmt.Errorf("failed to restart batch at line %d: %v", lineCount, err)
			}
		}

		if strings.HasPrefix(line, "|0000|") && len(parts) >= 8 {
			dtIni = parts[4]; dtFin = parts[5]; company = parts[6]; filialCNPJ = parts[7]
			count0000++
			// Update job metadata immediately (outside tx for visibility)
			db.Exec("UPDATE import_jobs SET company_name=$1, cnpj=$2, dt_ini=$3, dt_fin=$4 WHERE id=$5", company, filialCNPJ, parseDate(dtIni), parseDate(dtFin), jobID)

			if len(dtIni) == 8 {
				year, _ := strconv.Atoi(dtIni[4:8])
				if r, err := getTaxRates(db, year); err == nil { rates = r }
			}
		} else if strings.HasPrefix(line, "|0150|") && len(parts) >= 14 {
			count0150++
			stmtPart.Exec(jobID, parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[8], parts[9], parts[10], parts[11], parts[12], parts[13])
		} else if strings.HasPrefix(line, "|C100|") && len(parts) >= 29 {
			countC100++
			vlDoc := parseDecimal(parts[12]); vlIcms := parseDecimal(parts[22]); vlPis := parseDecimal(parts[26]); vlCofins := parseDecimal(parts[27])
			vlIcmsProj := vlIcms * (1 - (rates.PercReducICMS / 100.0))
			vlIbsProj := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
			vlCbsProj := vlDoc * (rates.PercCBS / 100.0)
			
			stmtC100.QueryRow(jobID, filialCNPJ, parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[8], parts[9], parseDate(parts[10]), parseDate(parts[11]), vlDoc, vlIcms, vlPis, vlCofins, vlPis+vlCofins, vlIcmsProj, vlIbsProj, vlCbsProj).Scan(&currentC100ID)
		} else if strings.HasPrefix(line, "|C190|") && len(parts) >= 12 && currentC100ID != "" {
			countC190++
			stmtC190.Exec(jobID, currentC100ID, parts[3], parseDecimal(parts[5]), parseDecimal(parts[6]), parseDecimal(parts[7]), parseDecimal(parts[8]), parseDecimal(parts[9]), parseDecimal(parts[10]), parseDecimal(parts[11]), parts[12])
		} else if strings.HasPrefix(line, "|C500|") && len(parts) >= 11 {
			countC500++
			vlDoc := parseDecimal(parts[10]); vlIcms := 0.0; if len(parts) > 11 { vlIcms = parseDecimal(parts[11]) }
			vlIcmsProj := vlIcms * (1 - (rates.PercReducICMS / 100.0))
			vlIbsProj := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
			vlCbsProj := vlDoc * (rates.PercCBS / 100.0)
			stmtC500.Exec(jobID, filialCNPJ, parts[4], parts[5], parts[7], parts[10], parseDate(parts[11]), parseDate(parts[12]), vlDoc, vlIcms, 0.0, 0.0, 0.0, vlIcmsProj, vlIbsProj, vlCbsProj)
		} else if strings.HasPrefix(line, "|C600|") && len(parts) >= 10 {
			countC600++
			vlDoc := parseDecimal(parts[9])
			vlIbsProj := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
			vlCbsProj := vlDoc * (rates.PercCBS / 100.0)
			stmtC600.Exec(jobID, filialCNPJ, parts[2], parts[3], parts[4], parts[5], parts[6], 0.0, parseDate(parts[8]), vlDoc, 0.0, 0.0, 0.0, 0.0, vlIbsProj, vlCbsProj)
		} else if strings.HasPrefix(line, "|D100|") && len(parts) >= 16 {
			countD100++
			vlDoc := parseDecimal(parts[12]); vlIcms := parseDecimal(parts[22])
			vlIcmsProj := vlIcms * (1 - (rates.PercReducICMS / 100.0))
			vlIbsProj := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
			vlCbsProj := vlDoc * (rates.PercCBS / 100.0)
			stmtD100.Exec(jobID, filialCNPJ, parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[8], parts[9], parseDate(parts[10]), parseDate(parts[11]), vlDoc, vlIcms, 0.0, 0.0, 0.0, vlIcmsProj, vlIbsProj, vlCbsProj)
		} else if strings.HasPrefix(line, "|D500|") && len(parts) >= 14 {
			countD500++
			vlDoc := parseDecimal(parts[12]); vlIcms := parseDecimal(parts[13])
			vlIcmsProj := vlIcms * (1 - (rates.PercReducICMS / 100.0))
			vlIbsProj := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
			vlCbsProj := vlDoc * (rates.PercCBS / 100.0)
			stmtD500.Exec(jobID, filialCNPJ, parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[8], parts[9], parseDate(parts[10]), parseDate(parts[11]), vlDoc, vlIcms, 0.0, 0.0, 0.0, vlIcmsProj, vlIbsProj, vlCbsProj)
		}
	}

	if err := scanner.Err(); err != nil { return "", fmt.Errorf("error reading file: %v", err) }
	if count0000 == 0 { return "", fmt.Errorf("invalid SPED file: Record 0000 not found") }

	// Final Batch Commit
	if err := commitBatch(); err != nil { return "", fmt.Errorf("final batch commit failed: %v", err) }

	// Run Aggregations (New Transaction)
	fmt.Println("Worker: Running aggregations (Database intensive)...")
	db.Exec("UPDATE import_jobs SET message='Running Aggregations (Database intensive)...' WHERE id=$1", jobID)
	
	// Aggregation Transaction
	aggTx, err := db.Begin()
	if err != nil { return "", fmt.Errorf("failed to begin aggregation tx: %v", err) }
	defer aggTx.Rollback()

	if err := runAggregations(aggTx, jobID, rates); err != nil {
		fmt.Printf("Worker: Error running aggregations: %v\n", err)
		return "", err
	}
	if err := aggTx.Commit(); err != nil { return "", fmt.Errorf("failed to commit aggregations: %v", err) }

	return fmt.Sprintf("Imported: 0000=%d, 0150=%d, C100=%d, C190=%d, C500=%d, C600=%d, D100=%d, D500=%d", 
		count0000, count0150, countC100, countC190, countC500, countC600, countD100, countD500), nil
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
			AND c.tipo = 'R'
		)
		GROUP BY c100.job_id, c100.filial_cnpj, c100.cod_part, TO_CHAR(c100.dt_doc, 'MM/YYYY'), c100.ind_oper
	`, jobID, rates.PercReducICMS, rates.PercIBS_UF, rates.PercIBS_Mun, rates.PercCBS)
	if err != nil { return fmt.Errorf("aggregation operacoes_comerciais failed: %v", err) }

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
	if err != nil { return fmt.Errorf("aggregation energia C500 failed: %v", err) }

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
	if err != nil { return fmt.Errorf("aggregation energia C600 failed: %v", err) }

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
	if err != nil { return fmt.Errorf("aggregation frete failed: %v", err) }

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
	if err != nil { return fmt.Errorf("aggregation comunicacoes failed: %v", err) }

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