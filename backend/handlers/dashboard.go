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
		if mesAno == "" {
			// If no filter, maybe return empty or aggregate all?
			// Let's aggregate all for now, or require a filter.
			// User asked for "Filtro mesano", implying it's optional or selectable.
		}

		// 1. Get Base Data (Current Reality)
		// We sum the "Origin" values (what we have today) to project into the future.
		// If mes_ano is present, filter by it.

		var queryBase string
		var args []interface{}

		if mesAno != "" {
			queryBase = `
				SELECT 
					COALESCE(SUM(valor_contabil), 0),
					COALESCE(SUM(vl_icms_origem), 0)
				FROM mv_mercadorias_agregada
				WHERE mes_ano = $1
			`
			args = append(args, mesAno)
		} else {
			queryBase = `
				SELECT 
					COALESCE(SUM(valor_contabil), 0),
					COALESCE(SUM(vl_icms_origem), 0)
				FROM mv_mercadorias_agregada
			`
		}

		var baseValor, baseIcms float64
		err := db.QueryRow(queryBase, args...).Scan(&baseValor, &baseIcms)
		if err != nil {
			http.Error(w, "Error querying base data: "+err.Error(), http.StatusInternalServerError)
			return
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

			// Calculation Logic
			// ICMS Projected = Current ICMS * (1 - Reduction)
			icmsProj := baseIcms * (1.0 - (reducIcms / 100.0))

			// Base for IBS/CBS = Valor Contabil - ICMS Projected (Tax on Tax removal? Or just new base?)
			// The memory says: "Base IBS/CBS = VL_DOC - VL_ICMS_PROJ"
			baseIbsCbs := baseValor - icmsProj

			// IBS Rate = UF + Mun
			ibsRate := (ibsUf + ibsMun) / 100.0
			cbsRate := cbs / 100.0

			ibsProj := baseIbsCbs * ibsRate
			cbsProj := baseIbsCbs * cbsRate

			saldo := icmsProj + ibsProj + cbsProj

			points = append(points, ProjectionPoint{
				Ano:           ano,
				Icms:          icmsProj,
				Ibs:           ibsProj,
				Cbs:           cbsProj,
				Saldo:         saldo,
				BaseCalculo:   baseIbsCbs,
				PercReducIcms: reducIcms,
			})
		}

		json.NewEncoder(w).Encode(points)
	}
}
