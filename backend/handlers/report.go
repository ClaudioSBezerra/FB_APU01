package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type MercadoriasReport struct {
	FilialNome    string  `json:"filial_nome"`
	MesAno        string  `json:"mes_ano"`
	Tipo          string  `json:"tipo"`
	TipoCfop      string  `json:"tipo_cfop,omitempty"` // Added for clarity
	Valor         float64 `json:"valor"`
	Pis           float64 `json:"pis"`
	Cofins        float64 `json:"cofins"`
	Icms          float64 `json:"icms"`
	IcmsProjetado float64 `json:"vl_icms_projetado"`
	IbsProjetado  float64 `json:"vl_ibs_projetado"`
	CbsProjetado  float64 `json:"vl_cbs_projetado"`
}

func GetMercadoriasReportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		targetYearStr := r.URL.Query().Get("target_year")
		var targetYear interface{} = nil
		if targetYearStr != "" {
			if y, err := strconv.Atoi(targetYearStr); err == nil {
				targetYear = y
			}
		}

		// operation_type: "comercial" (default) or "outras"
		opType := r.URL.Query().Get("tipo_operacao")
		if opType == "" {
			opType = "comercial"
		}

		// Base query for C100 using C190 breakdown
		// We join C190 to C100 to get date/header info
		// We join CFOP to filter by type

		var typeFilter string
		if opType == "comercial" {
			typeFilter = "COALESCE(f.tipo, 'O') IN ('R', 'S')" // Revenda (incluindo Legacy S)
		} else {
			typeFilter = "COALESCE(f.tipo, 'O') IN ('A', 'C', 'O', 'T')" // Ativo, Consumo, Outros, Transferencia
		}

		var query string

		// Query for C190 (Granular C100)
		queryC190 := fmt.Sprintf(`
			SELECT 
				COALESCE(j.company_name, 'Desconhecida') as filial_nome,
				COALESCE(TO_CHAR(c.dt_doc, 'MM/YYYY'), 'ND') as mes_ano,
				CASE WHEN c.ind_oper = '0' THEN 'ENTRADA' ELSE 'SAIDA' END as tipo,
				MAX(COALESCE(f.tipo, 'O')) as tipo_cfop,
				COALESCE(SUM(c190.vl_opr), 0) as valor,
				0 as pis, -- Not available in C190
				0 as cofins, -- Not available in C190
				COALESCE(SUM(c190.vl_icms), 0) as icms,
				COALESCE(SUM(c190.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0))), 0) as icms_projetado,
				COALESCE(SUM(c190.vl_opr * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)), 0) as ibs_projetado,
				COALESCE(SUM(c190.vl_opr * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)), 0) as cbs_projetado
			FROM reg_c190 c190
			JOIN reg_c100 c ON c.id = c190.id_pai_c100
			JOIN import_jobs j ON j.id = c.job_id
			LEFT JOIN cfop f ON c190.cfop = f.cfop
			LEFT JOIN tabela_aliquotas ta ON ta.ano = COALESCE($1, CAST(TO_CHAR(c.dt_doc, 'YYYY') AS INTEGER))
			WHERE %s
			GROUP BY 1, 2, 3
		`, typeFilter)

		// Query for D100/C500/C600 (Legacy/Other)
		// We only include these in 'outras' usually, as they are mostly usage/consumption/services.
		// Unless we have specific knowledge.
		// The user instruction: "No lugar do CFOP com TIPO 'R' de Revenda você trará 'A' de Ativo e 'C' de consumo".
		// This implies strict CFOP filtering.
		// D100/C500/C600 do NOT have CFOP in our current schema (checked migrations).
		// Therefore, we cannot filter them by CFOP type 'A' or 'C'.
		// To be safe and follow instructions strictly ("trará 'A' e 'C'"), we should ONLY return records where we can verify the type.
		// Since we can only verify CFOP type for C190, we will ONLY return C190 data in this new filtered mode.
		// D100/C500/C600 will be excluded from this view to avoid pollution with unknown types.
		// This makes the report purely based on the CFOP classification requested.

		query = queryC190

		rows, err := db.Query(query, targetYear)
		if err != nil {
			fmt.Printf("Error querying mercadorias report: %v\n", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var reports []MercadoriasReport
		for rows.Next() {
			var r MercadoriasReport
			if err := rows.Scan(&r.FilialNome, &r.MesAno, &r.Tipo, &r.TipoCfop, &r.Valor, &r.Pis, &r.Cofins, &r.Icms, &r.IcmsProjetado, &r.IbsProjetado, &r.CbsProjetado); err != nil {
				fmt.Printf("Error scanning mercadorias report: %v\n", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			reports = append(reports, r)
		}

		if reports == nil {
			reports = []MercadoriasReport{}
		}

		json.NewEncoder(w).Encode(reports)
	}
}

func GetTransporteReportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		targetYearStr := r.URL.Query().Get("target_year")
		var targetYear interface{} = nil
		if targetYearStr != "" {
			if y, err := strconv.Atoi(targetYearStr); err == nil {
				targetYear = y
			}
		}

		query := `
			SELECT 
				COALESCE(j.company_name, 'Desconhecida') as filial_nome,
				COALESCE(TO_CHAR(d.dt_doc, 'MM/YYYY'), 'ND') as mes_ano,
				CASE WHEN d.ind_oper = '0' THEN 'ENTRADA' ELSE 'SAIDA' END as tipo,
				COALESCE(SUM(d.vl_doc), 0) as valor,
				COALESCE(SUM(d.vl_pis), 0) as pis,
				COALESCE(SUM(d.vl_cofins), 0) as cofins,
				COALESCE(SUM(d.vl_icms), 0) as icms,
				COALESCE(SUM(d.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0))), 0) as icms_projetado,
				COALESCE(SUM(d.vl_doc * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)), 0) as ibs_projetado,
				COALESCE(SUM(d.vl_doc * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)), 0) as cbs_projetado
			FROM reg_d100 d
			JOIN import_jobs j ON j.id = d.job_id
			LEFT JOIN tabela_aliquotas ta ON ta.ano = COALESCE($1, CAST(TO_CHAR(d.dt_doc, 'YYYY') AS INTEGER))
			GROUP BY 1, 2, 3
		`

		rows, err := db.Query(query, targetYear)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var reports []MercadoriasReport
		for rows.Next() {
			var r MercadoriasReport
			// Helper struct doesn't have TipoCfop, need to scan carefully
			// Actually we are using MercadoriasReport struct which now has TipoCfop.
			// Scan order must match struct or we must skip the field in query?
			// The query above returns 10 columns.
			// The struct has 11 fields.
			// We need to scan explicitly.

			// Re-ordered scan to match query columns:
			// filial_nome, mes_ano, tipo, valor, pis, cofins, icms, icms_proj, ibs_proj, cbs_proj
			if err := rows.Scan(&r.FilialNome, &r.MesAno, &r.Tipo, &r.Valor, &r.Pis, &r.Cofins, &r.Icms, &r.IcmsProjetado, &r.IbsProjetado, &r.CbsProjetado); err != nil {
				fmt.Printf("Error scanning transporte report: %v\n", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			r.TipoCfop = "ND" // Default
			reports = append(reports, r)
		}

		if reports == nil {
			reports = []MercadoriasReport{}
		}

		json.NewEncoder(w).Encode(reports)
	}
}

func GetEnergiaReportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		targetYearStr := r.URL.Query().Get("target_year")
		var targetYear interface{} = nil
		if targetYearStr != "" {
			if y, err := strconv.Atoi(targetYearStr); err == nil {
				targetYear = y
			}
		}

		// Logic similar to original report.go for Energia, assuming C500/C600
		// We'll keep it simple for now, just copying the structure but ensuring it compiles with the struct change.

		query := `
			SELECT 
				COALESCE(j.company_name, 'Desconhecida') as filial_nome,
				COALESCE(TO_CHAR(c5.dt_doc, 'MM/YYYY'), 'ND') as mes_ano,
				'ENTRADA' as tipo,
				COALESCE(SUM(c5.vl_doc), 0) as valor,
				COALESCE(SUM(c5.vl_pis), 0) as pis,
				COALESCE(SUM(c5.vl_cofins), 0) as cofins,
				COALESCE(SUM(c5.vl_icms), 0) as icms,
				COALESCE(SUM(c5.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0))), 0) as icms_projetado,
				COALESCE(SUM(c5.vl_doc * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)), 0) as ibs_projetado,
				COALESCE(SUM(c5.vl_doc * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)), 0) as cbs_projetado
			FROM reg_c500 c5
			JOIN import_jobs j ON j.id = c5.job_id
			LEFT JOIN tabela_aliquotas ta ON ta.ano = COALESCE($1, CAST(TO_CHAR(c5.dt_doc, 'YYYY') AS INTEGER))
			GROUP BY 1, 2
		`

		rows, err := db.Query(query, targetYear)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var reports []MercadoriasReport
		for rows.Next() {
			var r MercadoriasReport
			if err := rows.Scan(&r.FilialNome, &r.MesAno, &r.Tipo, &r.Valor, &r.Pis, &r.Cofins, &r.Icms, &r.IcmsProjetado, &r.IbsProjetado, &r.CbsProjetado); err != nil {
				fmt.Printf("Error scanning energia report: %v\n", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			r.TipoCfop = "ND"
			reports = append(reports, r)
		}

		if reports == nil {
			reports = []MercadoriasReport{}
		}

		json.NewEncoder(w).Encode(reports)
	}
}

func GetComunicacoesReportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode([]MercadoriasReport{})
	}
}
