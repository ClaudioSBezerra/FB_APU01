package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"fb_apu01/services"

	"github.com/golang-jwt/jwt/v5"
)

type aiQueryRequest struct {
	Pergunta string `json:"pergunta"`
}

type aiQueryResult struct {
	Pergunta string                   `json:"pergunta"`
	SQL      string                   `json:"sql"`
	Columns  []string                 `json:"columns"`
	Rows     []map[string]interface{} `json:"rows"`
	RowCount int                      `json:"row_count"`
	Model    string                   `json:"model"`
}

// AIQueryHandler receives a natural language question, generates SQL via GLM,
// executes it against the database, and returns the results as JSON.
func AIQueryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		claims, ok := r.Context().Value(ClaimsKey).(jwt.MapClaims)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID, _ := claims["user_id"].(string)

		companyID, err := GetEffectiveCompanyID(db, userID, r.Header.Get("X-Company-ID"))
		if err != nil {
			http.Error(w, "erro ao obter empresa: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var req aiQueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Pergunta) == "" {
			http.Error(w, "pergunta inválida ou ausente", http.StatusBadRequest)
			return
		}

		aiClient := services.NewAIClient()
		if !aiClient.IsAvailable() {
			http.Error(w, "IA não configurada (ZAI_API_KEY ausente)", http.StatusServiceUnavailable)
			return
		}

		// Generate SQL via AI
		userPrompt := services.BuildTextToSQLPrompt(req.Pergunta)
		aiResp, err := aiClient.GenerateFastRaw(services.SystemPromptTextToSQL, userPrompt, "", 1024)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Erro na IA: %v", err),
			})
			return
		}

		// Extract and validate SQL from AI response
		generatedSQL, err := services.ExtractSQL(aiResp.Text)
		if err != nil {
			fmt.Printf("[AI Query] ExtractSQL failed: %v\nRaw AI text (first 500): %.500s\n", err, aiResp.Text)
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   fmt.Sprintf("IA não retornou SQL válido: %v", err),
				"ai_text": aiResp.Text,
			})
			return
		}

		// Inject company_id (UUID — safe for direct substitution)
		finalSQL := strings.ReplaceAll(generatedSQL, "__COMPANY_ID__", companyID)

		// Ensure there's a LIMIT
		if !strings.Contains(strings.ToUpper(finalSQL), "LIMIT") {
			finalSQL += "\nLIMIT 100"
		}

		// Execute the query
		rows, err := db.Query(finalSQL)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Erro ao executar query: %v", err),
				"sql":   finalSQL,
			})
			return
		}
		defer rows.Close()

		// Read results
		cols, _ := rows.Columns()
		var resultRows []map[string]interface{}
		for rows.Next() {
			vals := make([]interface{}, len(cols))
			ptrs := make([]interface{}, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				continue
			}
			row := make(map[string]interface{})
			for i, col := range cols {
				// PostgreSQL DECIMAL/NUMERIC scans as []byte; convert to string
				// to prevent json.Marshal from base64-encoding it.
				if b, ok := vals[i].([]byte); ok {
					row[col] = string(b)
				} else {
					row[col] = vals[i]
				}
			}
			resultRows = append(resultRows, row)
		}

		if resultRows == nil {
			resultRows = []map[string]interface{}{}
		}

		json.NewEncoder(w).Encode(aiQueryResult{
			Pergunta: req.Pergunta,
			SQL:      finalSQL,
			Columns:  cols,
			Rows:     resultRows,
			RowCount: len(resultRows),
			Model:    aiResp.Model,
		})
	}
}
