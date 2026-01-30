package handlers

import (
"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

type MercadoriasReport struct {
	FilialNome    string  `json:"filial_nome"`
	MesAno        string  `json:"mes_ano"`
	Tipo          string  `json:"tipo"`
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

		query := `
			SELECT 
				COALESCE(j.company_name, 'Desconhecida') as filial_nome,
				COALESCE(TO_CHAR(c.dt_doc, 'MM/YYYY'), 'ND') as mes_ano,
				CASE WHEN c.ind_oper = '0' THEN 'ENTRADA' ELSE 'SAIDA' END as tipo,
				COALESCE(SUM(c.vl_doc), 0) as valor,
				COALESCE(SUM(c.vl_pis), 0) as pis,
				COALESCE(SUM(c.vl_cofins), 0) as cofins,
				COALESCE(SUM(c.vl_icms), 0) as icms,
				COALESCE(SUM(c.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0))), 0) as icms_projetado,
				COALESCE(SUM(c.vl_doc * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)), 0) as ibs_projetado,
				COALESCE(SUM(c.vl_doc * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)), 0) as cbs_projetado
			FROM reg_c100 c
			JOIN import_jobs j ON j.id = c.job_id
			LEFT JOIN tabela_aliquotas ta ON ta.ano = COALESCE($1, CAST(TO_CHAR(c.dt_doc, 'YYYY') AS INTEGER))
			GROUP BY 1, 2, 3

			UNION ALL

			SELECT 
				COALESCE(j.company_name, 'Desconhecida'),
				TO_CHAR(d.dt_doc, 'MM/YYYY'),
				CASE WHEN d.ind_oper = '0' THEN 'ENTRADA' ELSE 'SAIDA' END,
				COALESCE(SUM(d.vl_doc), 0),
				COALESCE(SUM(d.vl_pis), 0),
				COALESCE(SUM(d.vl_cofins), 0),
				COALESCE(SUM(d.vl_icms), 0),
				COALESCE(SUM(d.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0))), 0),
				COALESCE(SUM(d.vl_doc * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)), 0),
				COALESCE(SUM(d.vl_doc * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)), 0)
			FROM reg_d100 d
			JOIN import_jobs j ON j.id = d.job_id
			LEFT JOIN tabela_aliquotas ta ON ta.ano = COALESCE($2, CAST(TO_CHAR(d.dt_doc, 'YYYY') AS INTEGER))
			GROUP BY 1, 2, 3

			UNION ALL

			SELECT 
				COALESCE(j.company_name, 'Desconhecida'),
				TO_CHAR(c5.dt_doc, 'MM/YYYY'),
				'ENTRADA',
				COALESCE(SUM(c5.vl_doc), 0),
				COALESCE(SUM(c5.vl_pis), 0),
				COALESCE(SUM(c5.vl_cofins), 0),
				COALESCE(SUM(c5.vl_icms), 0),
				COALESCE(SUM(c5.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0))), 0),
				COALESCE(SUM(c5.vl_doc * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)), 0),
				COALESCE(SUM(c5.vl_doc * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)), 0)
			FROM reg_c500 c5
			JOIN import_jobs j ON j.id = c5.job_id
			LEFT JOIN tabela_aliquotas ta ON ta.ano = COALESCE($3, CAST(TO_CHAR(c5.dt_doc, 'YYYY') AS INTEGER))
			GROUP BY 1, 2

			UNION ALL

			SELECT 
				COALESCE(j.company_name, 'Desconhecida'),
				TO_CHAR(c6.dt_doc, 'MM/YYYY'),
				'SAIDA',
				COALESCE(SUM(c6.vl_doc), 0),
				COALESCE(SUM(c6.vl_pis), 0),
				COALESCE(SUM(c6.vl_cofins), 0),
				0,
				0,
				COALESCE(SUM(c6.vl_doc * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)), 0),
				COALESCE(SUM(c6.vl_doc * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)), 0)
			FROM reg_c600 c6
			JOIN import_jobs j ON j.id = c6.job_id
			LEFT JOIN tabela_aliquotas ta ON ta.ano = COALESCE($4, CAST(TO_CHAR(c6.dt_doc, 'YYYY') AS INTEGER))
			GROUP BY 1, 2
`

		rows, err := db.Query(query, targetYear, targetYear, targetYear, targetYear)
if err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
defer rows.Close()

var reports []MercadoriasReport
for rows.Next() {
		var r MercadoriasReport
		if err := rows.Scan(&r.FilialNome, &r.MesAno, &r.Tipo, &r.Valor, &r.Pis, &r.Cofins, &r.Icms, &r.IcmsProjetado, &r.IbsProjetado, &r.CbsProjetado); err != nil {
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
			if err := rows.Scan(&r.FilialNome, &r.MesAno, &r.Tipo, &r.Valor, &r.Pis, &r.Cofins, &r.Icms, &r.IcmsProjetado, &r.IbsProjetado, &r.CbsProjetado); err != nil {
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
				COALESCE(TO_CHAR(c.dt_doc, 'MM/YYYY'), 'ND') as mes_ano,
				'ENTRADA' as tipo,
				COALESCE(SUM(c.vl_doc), 0) as valor,
				COALESCE(SUM(c.vl_pis), 0) as pis,
				COALESCE(SUM(c.vl_cofins), 0) as cofins,
				COALESCE(SUM(c.vl_icms), 0) as icms,
				COALESCE(SUM(c.vl_icms * (1 - (COALESCE(ta.perc_reduc_icms, 0) / 100.0))), 0) as icms_projetado,
				COALESCE(SUM(c.vl_doc * ((COALESCE(NULLIF(ta.perc_ibs_uf, 0), 9.0) + COALESCE(NULLIF(ta.perc_ibs_mun, 0), 8.7)) / 100.0)), 0) as ibs_projetado,
				COALESCE(SUM(c.vl_doc * (COALESCE(NULLIF(ta.perc_cbs, 0), 8.80) / 100.0)), 0) as cbs_projetado
			FROM reg_c500 c
			JOIN import_jobs j ON j.id = c.job_id
			LEFT JOIN tabela_aliquotas ta ON ta.ano = COALESCE($1, CAST(TO_CHAR(c.dt_doc, 'YYYY') AS INTEGER))
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
			FROM reg_d500 d
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
			if err := rows.Scan(&r.FilialNome, &r.MesAno, &r.Tipo, &r.Valor, &r.Pis, &r.Cofins, &r.Icms, &r.IcmsProjetado, &r.IbsProjetado, &r.CbsProjetado); err != nil {
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