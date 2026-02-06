package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/golang-jwt/jwt/v5"
)

type SimplesSupplierData struct {
	FornecedorNome string  `json:"fornecedor_nome"`
	FornecedorCNPJ string  `json:"fornecedor_cnpj"`
	TotalValor     float64 `json:"total_valor"`
	TotalICMS      float64 `json:"total_icms"`
	LostIBS        float64 `json:"lost_ibs"`
	LostCBS        float64 `json:"lost_cbs"`
	TotalLost      float64 `json:"total_lost"`
}

func GetSimplesDashboardHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Get User Context
		claims, ok := r.Context().Value(ClaimsKey).(jwt.MapClaims)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userID := claims["user_id"].(string)

		companyID, err := GetEffectiveCompanyID(db, userID, r.Header.Get("X-Company-ID"))
		if err != nil {
			http.Error(w, "Error getting user company: "+err.Error(), http.StatusInternalServerError)
			return
		}

		mesAno := r.URL.Query().Get("mes_ano")
		projectionYearStr := r.URL.Query().Get("projection_year")
		projectionYear := 2033
		if projectionYearStr != "" {
			fmt.Sscanf(projectionYearStr, "%d", &projectionYear)
		}
		if projectionYear < 2027 || projectionYear > 2033 {
			projectionYear = 2033
		}

		// Query logic
		var query string
		var args []interface{}

		if mesAno != "" {
			query = `
				SELECT 
					fornecedor_nome,
					fornecedor_cnpj,
					SUM(total_valor) as total_valor,
					SUM(total_icms) as total_icms
				FROM mv_operacoes_simples
				WHERE company_id = $1 AND mes_ano = $2
				GROUP BY fornecedor_nome, fornecedor_cnpj
			`
			args = append(args, companyID, mesAno)
		} else {
			query = `
				SELECT 
					fornecedor_nome,
					fornecedor_cnpj,
					SUM(total_valor) as total_valor,
					SUM(total_icms) as total_icms
				FROM mv_operacoes_simples
				WHERE company_id = $1
				GROUP BY fornecedor_nome, fornecedor_cnpj
			`
			args = append(args, companyID)
		}

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, "Error querying simples data: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		// Fetch Aliquots for Selected Year (Default 2033) for "Lost Credit" Calculation
		var ibsRate, cbsRate, reducIcms float64
		err = db.QueryRow(`
			SELECT perc_ibs_uf + perc_ibs_mun, perc_cbs, perc_reduc_icms 
			FROM tabela_aliquotas 
			WHERE ano = $1 
			LIMIT 1
		`, projectionYear).Scan(&ibsRate, &cbsRate, &reducIcms)

		if err != nil {
			// Fallback defaults if year not found (should not happen if seeded)
			ibsRate = 17.7
			cbsRate = 8.8
			reducIcms = 100.0
		}

		var results []SimplesSupplierData

		for rows.Next() {
			var d SimplesSupplierData
			if err := rows.Scan(&d.FornecedorNome, &d.FornecedorCNPJ, &d.TotalValor, &d.TotalICMS); err != nil {
				continue
			}

			// Calculate "Lost Credit"
			// Base Calculation:
			// If it wasn't Simples, we would credit IBS/CBS on the Base.
			// Base = Value - ICMS_Projected.
			// Since Simples has little/no ICMS credit transfer, the "Lost" opportunity is significant.
			// We assume the Full Value is the potential base if it were a normal supplier.

			// Note: If we use the "Projected" logic:
			// ICMS Proj would be 0 (reducIcms = 100% in 2033).
			// So Base = TotalValor.

			base := d.TotalValor

			d.LostIBS = base * (ibsRate / 100.0)
			d.LostCBS = base * (cbsRate / 100.0)
			d.TotalLost = d.LostIBS + d.LostCBS

			results = append(results, d)
		}

		// Sort by Total Lost (Desc)
		sort.Slice(results, func(i, j int) bool {
			return results[i].TotalLost > results[j].TotalLost
		})

		json.NewEncoder(w).Encode(results)
	}
}
