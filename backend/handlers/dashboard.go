package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type ProjectionPoint struct {
	Ano           int     `json:"ano"`
	Icms          float64 `json:"vl_icms"`
	Ibs           float64 `json:"vl_ibs"`
	Cbs           float64 `json:"vl_cbs"`
	Saldo         float64 `json:"vl_saldo"`
	BaseCalculo   float64 `json:"vl_base"`
	PercReducIcms float64 `json:"perc_reduc_icms"`
}

func GetDashboardProjectionHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		mesAno := r.URL.Query().Get("mes_ano")
		
		// 1. Get Base Data (Current Reality) Split by Type (Entrada vs Saida)
		var queryBase string
		var args []interface{}
		
		if mesAno != "" {
			queryBase = `
				SELECT 
					tipo,
					COALESCE(SUM(valor_contabil), 0),
					COALESCE(SUM(vl_icms_origem), 0)
				FROM mv_mercadorias_agregada
				WHERE mes_ano = $1
				GROUP BY tipo
			`
			args = append(args, mesAno)
		} else {
			queryBase = `
				SELECT 
					tipo,
					COALESCE(SUM(valor_contabil), 0),
					COALESCE(SUM(vl_icms_origem), 0)
				FROM mv_mercadorias_agregada
				GROUP BY tipo
			`
		}

		rowsBase, err := db.Query(queryBase, args...)
		if err != nil {
			http.Error(w, "Error querying base data: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rowsBase.Close()

		var (
			valSaida, icmsSaida     float64
			valEntrada, icmsEntrada float64
		)

		for rowsBase.Next() {
			var tipo string
			var val, icms float64
			if err := rowsBase.Scan(&tipo, &val, &icms); err != nil {
				continue
			}
			if tipo == "SAIDA" {
				valSaida += val
				icmsSaida += icms
			} else if tipo == "ENTRADA" {
				valEntrada += val
				icmsEntrada += icms
			}
		}

		// 2. Get Future Aliquotas (2027-2033)
		rows, err := db.Query(`
			SELECT ano, perc_reduc_icms, perc_ibs_uf, perc_ibs_mun, perc_cbs
			FROM tabela_aliquotas
			WHERE ano BETWEEN 2027 AND 2033
			ORDER BY ano
		`)
		if err != nil {
			http.Error(w, "Error querying aliquotas: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var points []ProjectionPoint
		for rows.Next() {
			var ano int
			var reducIcms, ibsUf, ibsMun, cbs float64
			
			if err := rows.Scan(&ano, &reducIcms, &ibsUf, &ibsMun, &cbs); err != nil {
				continue
			}

			// Calculation Logic (Net = Debit - Credit)
			
			// ICMS Projected (Debit & Credit)
			icmsProjDebit := icmsSaida * (1.0 - (reducIcms / 100.0))
			icmsProjCredit := icmsEntrada * (1.0 - (reducIcms / 100.0))
			icmsNet := icmsProjDebit - icmsProjCredit
			
			// Base for IBS/CBS (Debit & Credit)
			// Base = Valor - ICMS Projected
			baseDebit := valSaida - icmsProjDebit
			baseCredit := valEntrada - icmsProjCredit
			
			// IBS/CBS Rates
			ibsRate := (ibsUf + ibsMun) / 100.0
			cbsRate := cbs / 100.0
			
			// IBS/CBS Projected
			ibsNet := (baseDebit * ibsRate) - (baseCredit * ibsRate)
			cbsNet := (baseDebit * cbsRate) - (baseCredit * cbsRate)
			
			// Total Saldo a Pagar
			saldo := icmsNet + ibsNet + cbsNet

			points = append(points, ProjectionPoint{
				Ano:           ano,
				Icms:          icmsNet,
				Ibs:           ibsNet,
				Cbs:           cbsNet,
				Saldo:         saldo,
				BaseCalculo:   baseDebit - baseCredit, // Net Base
				PercReducIcms: reducIcms,
			})
		}

		json.NewEncoder(w).Encode(points)
	}
}
