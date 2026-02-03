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
	FilialCNPJ    string  `json:"filial_cnpj"`
	MesAno        string  `json:"mes_ano"`
	Tipo          string  `json:"tipo"`
	TipoCfop      string  `json:"tipo_cfop,omitempty"`
	Origem        string  `json:"origem,omitempty"`
	Valor         float64 `json:"valor"`
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

		// OPTIMIZATION: Use Materialized View (mv_mercadorias_agregada)
		// This view is aggregated by Filial, Month, Type, and CFOP Type.

		var typeFilter string
		if opType == "comercial" {
			// Commercial: Revenda (R), Saída (S)
			typeFilter = "mv.tipo_cfop IN ('R', 'S')"
		} else if opType == "todos" {
			// Returns all records
			typeFilter = "1=1"
		} else {
			// Others: Ativo (A), Consumo (C), Transferência (T), Outros (O)
			// 'O' includes unmapped CFOPs
			typeFilter = "mv.tipo_cfop IN ('A', 'C', 'T', 'O')"
		}

		// Projection Logic for Tax Reform (2027+):
		// - PIS/COFINS are replaced by IBS/CBS.
		// - Base IBS/CBS = VL_DOC - VL_ICMS_PROJ
		// - VL_ICMS_PROJ = VL_ICMS_ORIGEM * (1 - Reducao)

		query := fmt.Sprintf(`
			SELECT 
				mv.filial_nome,
				mv.filial_cnpj,
				mv.mes_ano,
				mv.tipo,
				mv.tipo_cfop,
				mv.origem,
				SUM(mv.valor_contabil) as valor,
				SUM(mv.vl_icms_origem) as icms,
				SUM(mv.vl_icms_origem * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0))) as icms_projetado,
				SUM((mv.valor_contabil - (mv.vl_icms_origem * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0)))) * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)) as ibs_projetado,
				SUM((mv.valor_contabil - (mv.vl_icms_origem * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0)))) * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)) as cbs_projetado
			FROM mv_mercadorias_agregada mv
			LEFT JOIN tabela_aliquotas ta ON ta.ano = COALESCE($1, mv.ano)
			WHERE %s
			GROUP BY 1, 2, 3, 4, 5, 6
		`, typeFilter)

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
			if err := rows.Scan(&r.FilialNome, &r.FilialCNPJ, &r.MesAno, &r.Tipo, &r.TipoCfop, &r.Origem, &r.Valor, &r.Icms, &r.IcmsProjetado, &r.IbsProjetado, &r.CbsProjetado); err != nil {
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
				COALESCE(SUM(d.vl_icms), 0) as icms,
				COALESCE(SUM(d.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0))), 0) as icms_projetado,
				COALESCE(SUM((d.vl_doc - (d.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0)))) * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)), 0) as ibs_projetado,
				COALESCE(SUM((d.vl_doc - (d.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0)))) * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)), 0) as cbs_projetado
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
			// Scan updated to remove Pis and Cofins
			if err := rows.Scan(&r.FilialNome, &r.MesAno, &r.Tipo, &r.Valor, &r.Icms, &r.IcmsProjetado, &r.IbsProjetado, &r.CbsProjetado); err != nil {
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

		query := `
			SELECT 
				COALESCE(j.company_name, 'Desconhecida') as filial_nome,
				COALESCE(TO_CHAR(c5.dt_doc, 'MM/YYYY'), 'ND') as mes_ano,
				'ENTRADA' as tipo,
				COALESCE(SUM(c5.vl_doc), 0) as valor,
				COALESCE(SUM(c5.vl_icms), 0) as icms,
				COALESCE(SUM(c5.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0))), 0) as icms_projetado,
				COALESCE(SUM((c5.vl_doc - (c5.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0)))) * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)), 0) as ibs_projetado,
				COALESCE(SUM((c5.vl_doc - (c5.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0)))) * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)), 0) as cbs_projetado
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
			if err := rows.Scan(&r.FilialNome, &r.MesAno, &r.Tipo, &r.Valor, &r.Icms, &r.IcmsProjetado, &r.IbsProjetado, &r.CbsProjetado); err != nil {
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

		targetYearStr := r.URL.Query().Get("target_year")
		var targetYear interface{} = nil
		if targetYearStr != "" {
			if y, err := strconv.Atoi(targetYearStr); err == nil {
				targetYear = y
			}
		}

		// Implementação para Comunicações (D500)
		query := `
			SELECT 
				COALESCE(j.company_name, 'Desconhecida') as filial_nome,
				COALESCE(TO_CHAR(d5.dt_doc, 'MM/YYYY'), 'ND') as mes_ano,
				CASE WHEN d5.ind_oper = '0' THEN 'ENTRADA' ELSE 'SAIDA' END as tipo,
				COALESCE(SUM(d5.vl_doc), 0) as valor,
				COALESCE(SUM(d5.vl_icms), 0) as icms,
				COALESCE(SUM(d5.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0))), 0) as icms_projetado,
				COALESCE(SUM((d5.vl_doc - (d5.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0)))) * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)), 0) as ibs_projetado,
				COALESCE(SUM((d5.vl_doc - (d5.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0)))) * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)), 0) as cbs_projetado
			FROM reg_d500 d5
			JOIN import_jobs j ON j.id = d5.job_id
			LEFT JOIN tabela_aliquotas ta ON ta.ano = COALESCE($1, CAST(TO_CHAR(d5.dt_doc, 'YYYY') AS INTEGER))
			GROUP BY 1, 2, 3
		`

		rows, err := db.Query(query, targetYear)
		if err != nil {
			// Se a tabela não existir ou erro de query, retorna vazio por enquanto para não quebrar
			fmt.Printf("Error querying comunicacoes report: %v\n", err)
			json.NewEncoder(w).Encode([]MercadoriasReport{})
			return
		}
		defer rows.Close()

		var reports []MercadoriasReport
		for rows.Next() {
			var r MercadoriasReport
			if err := rows.Scan(&r.FilialNome, &r.MesAno, &r.Tipo, &r.Valor, &r.Icms, &r.IcmsProjetado, &r.IbsProjetado, &r.CbsProjetado); err != nil {
				fmt.Printf("Error scanning comunicacoes report: %v\n", err)
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
