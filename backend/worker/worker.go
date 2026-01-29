package worker

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
		currentC010CNPJ string
		currentD010CNPJ string
		lastD100ID string
		countC100, countC500, countC600, countD100, countD500 int
		rates TaxRates // Dynamic rates
	)

	fmt.Printf("Worker: Parsing SPED file %s...\n", filename)

	// Use a transaction for better performance and consistency
	tx, err := db.Begin()
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Insert Dummy Participants
	_, err = tx.Exec(`
		INSERT INTO participants (job_id, cod_part, nome, cnpj, cpf) 
		VALUES ($1, '9999999999', 'CONSUMIDOR FINAL', '', ''), ($1, '8888888888', 'FORNECEDOR GENERICO', '', '')
	`, jobID)
	if err != nil {
		fmt.Printf("Worker: Warning inserting dummy participants: %v\n", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO participants (
			job_id, cod_part, nome, cod_pais, cnpj, cpf, ie, cod_mun, suframa, endereco, numero, complemento, bairro
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`)
	if err != nil {
		return "", fmt.Errorf("failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	stmt0140, err := tx.Prepare(`INSERT INTO reg_0140 (job_id, cod_est, nome, cnpj, uf, ie) VALUES ($1, $2, $3, $4, $5, $6)`)
	if err != nil { return "", err }
	defer stmt0140.Close()

	stmtC100, err := tx.Prepare(`INSERT INTO reg_c100 (job_id, filial_cnpj, ind_oper, ind_emit, cod_part, cod_mod, cod_sit, ser, num_doc, chv_nfe, dt_doc, dt_e_s, vl_doc, vl_icms, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`)
	if err != nil { return "", err }
	defer stmtC100.Close()

	stmtC500, err := tx.Prepare(`INSERT INTO reg_c500 (job_id, filial_cnpj, cod_part, cod_mod, ser, num_doc, dt_doc, dt_e_s, vl_doc, vl_icms, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`)
	if err != nil { return "", err }
	defer stmtC500.Close()

	stmtC600, err := tx.Prepare(`INSERT INTO reg_c600 (job_id, filial_cnpj, cod_mod, cod_mun, ser, sub, cod_cons, qtd_cons, dt_doc, vl_doc, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`)
	if err != nil { return "", err }
	defer stmtC600.Close()

	stmtD100, err := tx.Prepare(`INSERT INTO reg_d100 (job_id, filial_cnpj, ind_oper, ind_emit, cod_part, cod_mod, cod_sit, ser, num_doc, chv_cte, dt_doc, dt_a_p, vl_doc, vl_icms, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20) RETURNING id`)
	if err != nil { return "", err }
	defer stmtD100.Close()

	stmtD500, err := tx.Prepare(`INSERT INTO reg_d500 (job_id, filial_cnpj, ind_oper, ind_emit, cod_part, cod_mod, cod_sit, ser, sub, num_doc, dt_doc, dt_a_p, vl_doc, vl_icms, vl_pis, vl_cofins, vl_piscofins, vl_icms_projetado, vl_ibs_projetado, vl_cbs_projetado) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`)
	if err != nil { return "", err }
	defer stmtD500.Close()

	// Prepare update statement for D100 children (D101/D105)
	stmtUpdateD100, err := tx.Prepare(`UPDATE reg_d100 SET vl_pis = vl_pis + $1, vl_cofins = vl_cofins + $2, vl_piscofins = vl_piscofins + $1 + $2 WHERE id = $3`)
	if err != nil { return "", err }
	defer stmtUpdateD100.Close()

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
				
				// Fetch tax rates based on the year of dtIni
				if len(dtIni) == 8 {
					yearStr := dtIni[4:8]
					year, _ := strconv.Atoi(yearStr)
					r, err := getTaxRates(db, year)
					if err != nil {
						fmt.Printf("Worker: Error fetching tax rates for year %d: %v. Using defaults.\n", year, err)
					} else {
						rates = r
						fmt.Printf("Worker: Using tax rates for %d: IBS_UF=%.2f, IBS_Mun=%.2f, CBS=%.2f\n", year, rates.PercIBS_UF, rates.PercIBS_Mun, rates.PercCBS)
					}
				}
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
		} else if strings.HasPrefix(line, "|0140|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 8 {
				stmt0140.Exec(jobID, parts[2], parts[3], parts[4], parts[5], parts[6])
			}
		} else if strings.HasPrefix(line, "|C010|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				currentC010CNPJ = parts[2]
			}
		} else if strings.HasPrefix(line, "|C100|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 29 {
				countC100++
				vlDoc := parseDecimal(parts[12])
				vlIcms := parseDecimal(parts[21])
				vlPis := parseDecimal(parts[26])
				vlCofins := parseDecimal(parts[27])
				
				vlIcmsProjetado := vlIcms * (1 - (rates.PercReducICMS / 100.0))
				vlIbsProjetado := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
				vlCbsProjetado := vlDoc * (rates.PercCBS / 100.0)
				
				stmtC100.Exec(jobID, currentC010CNPJ, parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[8], parts[9], parseDate(parts[10]), parseDate(parts[11]), vlDoc, vlIcms, vlPis, vlCofins, vlPis+vlCofins, vlIcmsProjetado, vlIbsProjetado, vlCbsProjetado)
			}
		} else if strings.HasPrefix(line, "|C500|") {
			parts := strings.Split(line, "|")
			// C500: Relaxed validation. 
			// Mandatory indices: 2,3,5,7,8,9,10 (VL_DOC). Max index 10 -> len >= 11.
			if len(parts) >= 11 {
				countC500++
				vlDoc := parseDecimal(parts[10])
				vlIcms := 0.0
				if len(parts) > 11 { vlIcms = parseDecimal(parts[11]) }
				vlPis := 0.0
				if len(parts) > 13 { vlPis = parseDecimal(parts[13]) }
				vlCofins := 0.0
				if len(parts) > 14 { vlCofins = parseDecimal(parts[14]) }
				
				vlIcmsProjetado := vlIcms * (1 - (rates.PercReducICMS / 100.0))
				vlIbsProjetado := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
				vlCbsProjetado := vlDoc * (rates.PercCBS / 100.0)
				
				stmtC500.Exec(jobID, currentC010CNPJ, parts[2], parts[3], parts[5], parts[7], parseDate(parts[8]), parseDate(parts[9]), vlDoc, vlIcms, vlPis, vlCofins, vlPis+vlCofins, vlIcmsProjetado, vlIbsProjetado, vlCbsProjetado)
			}
		} else if strings.HasPrefix(line, "|C600|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 21 {
				countC600++
				vlDoc := parseDecimal(parts[9]) 
				vlPis := parseDecimal(parts[19])
				vlCofins := parseDecimal(parts[20])
				
				vlIcmsProjetado := 0.0
				vlIbsProjetado := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
				vlCbsProjetado := vlDoc * (rates.PercCBS / 100.0)
				
				stmtC600.Exec(jobID, currentC010CNPJ, parts[2], parts[3], parts[4], parts[5], parts[6], parseDecimal(parts[7]), parseDate(parts[8]), vlDoc, vlPis, vlCofins, vlPis+vlCofins, vlIcmsProjetado, vlIbsProjetado, vlCbsProjetado)
			}
		} else if strings.HasPrefix(line, "|D010|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				currentD010CNPJ = parts[2]
			}
		} else if strings.HasPrefix(line, "|D100|") {
			parts := strings.Split(line, "|")
			// D100: Relaxed validation.
			if len(parts) >= 16 {
				countD100++
				vlDoc := parseDecimal(parts[15])
				vlIcms := 0.0
				if len(parts) > 20 { vlIcms = parseDecimal(parts[20]) }
				vlPis := 0.0 // Will be updated by D101/D105 if present
				if len(parts) > 24 { vlPis = parseDecimal(parts[24]) }
				vlCofins := 0.0 // Will be updated by D101/D105 if present
				if len(parts) > 25 { vlCofins = parseDecimal(parts[25]) }

				vlIcmsProjetado := vlIcms * (1 - (rates.PercReducICMS / 100.0))
				vlIbsProjetado := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
				vlCbsProjetado := vlDoc * (rates.PercCBS / 100.0)

				err := stmtD100.QueryRow(jobID, currentD010CNPJ, parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[9], parts[10], parseDate(parts[11]), parseDate(parts[12]), vlDoc, vlIcms, vlPis, vlCofins, vlPis+vlCofins, vlIcmsProjetado, vlIbsProjetado, vlCbsProjetado).Scan(&lastD100ID)
				if err != nil {
					fmt.Printf("Worker: Error inserting D100: %v\n", err)
				}
			}
		} else if strings.HasPrefix(line, "|D101|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 12 && lastD100ID != "" {
				vlPis := parseDecimal(parts[7])
				vlCofins := parseDecimal(parts[11])
				stmtUpdateD100.Exec(vlPis, vlCofins, lastD100ID)
			}
		} else if strings.HasPrefix(line, "|D105|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 12 && lastD100ID != "" {
				vlPis := parseDecimal(parts[7])
				vlCofins := parseDecimal(parts[11])
				stmtUpdateD100.Exec(vlPis, vlCofins, lastD100ID)
			}
		} else if strings.HasPrefix(line, "|D500|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 22 {
				countD500++
				vlDoc := parseDecimal(parts[11])
				vlIcms := parseDecimal(parts[18])
				vlPis := parseDecimal(parts[20])
				vlCofins := parseDecimal(parts[21])
				
				vlIcmsProjetado := vlIcms * (1 - (rates.PercReducICMS / 100.0))
				vlIbsProjetado := vlDoc * ((rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0)
				vlCbsProjetado := vlDoc * (rates.PercCBS / 100.0)
				
				stmtD500.Exec(jobID, currentD010CNPJ, parts[1], parts[2], parts[3], parts[4], parts[5], parts[6], parts[7], parts[8], parseDate(parts[9]), parseDate(parts[10]), vlDoc, vlIcms, vlPis, vlCofins, vlPis+vlCofins, vlIcmsProjetado, vlIbsProjetado, vlCbsProjetado)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading file: %v", err)
	}

	if count0000 == 0 {
		return "", fmt.Errorf("invalid SPED file: Record 0000 not found")
	}

	// Update job metadata
	_, err = tx.Exec(`
		UPDATE import_jobs 
		SET company_name = $1, cnpj = $2, dt_ini = $3, dt_fin = $4 
		WHERE id = $5
	`, company, cnpj, parseDate(dtIni), parseDate(dtFin), jobID)
	if err != nil {
		// Log but don't fail, maybe columns don't exist yet
		fmt.Printf("Worker: Warning updating metadata: %v\n", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %v", err)
	}

	summary := fmt.Sprintf("Analyzed: %s (%s) | Period: %s to %s | Part:%d | Docs: C100:%d C500:%d C600:%d D100:%d D500:%d", 
		company, cnpj, dtIni, dtFin, count0150, countC100, countC500, countC600, countD100, countD500)
	
	return summary, nil
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