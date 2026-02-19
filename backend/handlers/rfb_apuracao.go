package handlers

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"fb_apu01/services"

	"github.com/golang-jwt/jwt/v5"
)

// RFBRequest represents a request to the RFB API
type RFBRequest struct {
	ID           string     `json:"id"`
	CompanyID    string     `json:"company_id"`
	CNPJBase     string     `json:"cnpj_base"`
	Tiquete      string     `json:"tiquete,omitempty"`
	Status       string     `json:"status"`
	Ambiente     string     `json:"ambiente"`
	ErrorCode    *string    `json:"error_code,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// RFBResumo represents the summary of a CBS assessment
type RFBResumo struct {
	ID               string  `json:"id"`
	RequestID        string  `json:"request_id"`
	DataApuracao     string  `json:"data_apuracao"`
	TotalDebitos     int     `json:"total_debitos"`
	ValorCBSTotal    float64 `json:"valor_cbs_total"`
	ValorCBSExtinto  float64 `json:"valor_cbs_extinto"`
	ValorCBSNaoExtinto float64 `json:"valor_cbs_nao_extinto"`
	TotalCorrente    int     `json:"total_corrente"`
	TotalAjuste      int     `json:"total_ajuste"`
	TotalExtemporaneo int    `json:"total_extemporaneo"`
}

// RFBDebitoRow represents a normalized debit row for the frontend
type RFBDebitoRow struct {
	ID               string   `json:"id"`
	TipoApuracao     string   `json:"tipo_apuracao"`
	ModeloDfe        string   `json:"modelo_dfe"`
	NumeroDfe        string   `json:"numero_dfe"`
	ChaveDfe         string   `json:"chave_dfe"`
	DataDfeEmissao   *string  `json:"data_dfe_emissao"`
	DataApuracao     string   `json:"data_apuracao"`
	NiEmitente       string   `json:"ni_emitente"`
	NiAdquirente     string   `json:"ni_adquirente"`
	ValorCBSTotal    float64  `json:"valor_cbs_total"`
	ValorCBSExtinto  float64  `json:"valor_cbs_extinto"`
	ValorCBSNaoExtinto float64 `json:"valor_cbs_nao_extinto"`
	SituacaoDebito   string   `json:"situacao_debito"`
}

// SolicitarApuracaoHandler triggers a new CBS assessment request to the RFB API
func SolicitarApuracaoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		claims, ok := r.Context().Value(ClaimsKey).(jwt.MapClaims)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userID := claims["user_id"].(string)

		companyID, err := GetEffectiveCompanyID(db, userID, r.Header.Get("X-Company-ID"))
		if err != nil {
			http.Error(w, "Error getting company: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Check if company has active credentials
		var clientID, clientSecret, cnpjMatriz string
		err = db.QueryRow(`
			SELECT client_id, client_secret, cnpj_matriz FROM rfb_credentials
			WHERE company_id = $1 AND ativo = true
		`, companyID).Scan(&clientID, &clientSecret, &cnpjMatriz)
		if err == sql.ErrNoRows {
			http.Error(w, "Credenciais RFB não configuradas. Configure em Conectar Receita Federal > Credenciais API.", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "Erro ao buscar credenciais: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Check daily limit (max 2 requests per day)
		var todayCount int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM rfb_requests
			WHERE company_id = $1 AND status != 'error'
			AND created_at >= CURRENT_DATE
		`, companyID).Scan(&todayCount)
		if err == nil && todayCount >= 2 {
			http.Error(w, "Limite diário atingido (máximo 2 solicitações por dia)", http.StatusTooManyRequests)
			return
		}

		// Extract CNPJ base (first 8 digits)
		cnpjBase := cnpjMatriz
		if len(cnpjBase) > 8 {
			cnpjBase = cnpjBase[:8]
		}

		// 1. Get OAuth2 token
		rfbClient := services.NewRFBClient()
		token, err := rfbClient.GetToken(clientID, clientSecret)
		if err != nil {
			// Save failed request
			db.Exec(`
				INSERT INTO rfb_requests (company_id, cnpj_base, status, error_code, error_message)
				VALUES ($1, $2, 'error', 'TOKEN_ERROR', $3)
			`, companyID, cnpjBase, err.Error())
			http.Error(w, "Erro ao obter token da RFB: "+err.Error(), http.StatusBadGateway)
			return
		}

		// 2. Request CBS assessment
		tiquete, err := rfbClient.SolicitarApuracao(token, cnpjBase)
		if err != nil {
			db.Exec(`
				INSERT INTO rfb_requests (company_id, cnpj_base, status, error_code, error_message)
				VALUES ($1, $2, 'error', 'REQUEST_ERROR', $3)
			`, companyID, cnpjBase, err.Error())
			http.Error(w, "Erro ao solicitar apuração: "+err.Error(), http.StatusBadGateway)
			return
		}

		// 3. Save request with ticket
		var requestID string
		err = db.QueryRow(`
			INSERT INTO rfb_requests (company_id, cnpj_base, tiquete, status)
			VALUES ($1, $2, $3, 'requested')
			RETURNING id
		`, companyID, cnpjBase, tiquete).Scan(&requestID)
		if err != nil {
			http.Error(w, "Erro ao salvar solicitação: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"request_id": requestID,
			"tiquete":    tiquete,
			"status":     "requested",
			"message":    "Solicitação enviada à Receita Federal. Aguarde o retorno via webhook.",
		})
	}
}

// RFBWebhookHandler receives callbacks from the RFB API (PUBLIC - no JWT auth)
func RFBWebhookHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("[RFB Webhook] Error reading body: %v", err)
			http.Error(w, "Error reading body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		log.Printf("[RFB Webhook] Received callback: %s", string(body))

		// Parse webhook payload - try to extract tiquete
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			log.Printf("[RFB Webhook] Error parsing JSON: %v", err)
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Extract tiquete from payload
		tiquete, _ := payload["tiquete"].(string)
		if tiquete == "" {
			log.Printf("[RFB Webhook] No tiquete in payload, storing raw: %s", string(body))
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "received"})
			return
		}

		// Find the request by tiquete
		var requestID string
		err = db.QueryRow(`
			SELECT id FROM rfb_requests WHERE tiquete = $1 AND status = 'requested'
		`, tiquete).Scan(&requestID)
		if err != nil {
			log.Printf("[RFB Webhook] Request not found for tiquete %s: %v", tiquete, err)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "received", "warning": "request not found"})
			return
		}

		// Update status
		_, err = db.Exec(`
			UPDATE rfb_requests SET status = 'webhook_received', updated_at = CURRENT_TIMESTAMP WHERE id = $1
		`, requestID)
		if err != nil {
			log.Printf("[RFB Webhook] Error updating status: %v", err)
		}

		// Trigger async download and processing
		go func() {
			rfbClient := services.NewRFBClient()
			if err := services.ProcessarDownloadRFB(db, rfbClient, requestID); err != nil {
				log.Printf("[RFB Webhook] Error processing download for request %s: %v", requestID, err)
			}
		}()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "processing"})
	}
}

// StatusApuracaoHandler returns the list of RFB requests for the company
func StatusApuracaoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		claims, ok := r.Context().Value(ClaimsKey).(jwt.MapClaims)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userID := claims["user_id"].(string)

		companyID, err := GetEffectiveCompanyID(db, userID, r.Header.Get("X-Company-ID"))
		if err != nil {
			http.Error(w, "Error getting company: "+err.Error(), http.StatusInternalServerError)
			return
		}

		rows, err := db.Query(`
			SELECT id, company_id, cnpj_base, COALESCE(tiquete, ''), status, ambiente,
				error_code, error_message, created_at, updated_at
			FROM rfb_requests
			WHERE company_id = $1
			ORDER BY created_at DESC
			LIMIT 20
		`, companyID)
		if err != nil {
			http.Error(w, "Error querying requests: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var requests []RFBRequest
		for rows.Next() {
			var req RFBRequest
			if err := rows.Scan(&req.ID, &req.CompanyID, &req.CNPJBase, &req.Tiquete, &req.Status, &req.Ambiente,
				&req.ErrorCode, &req.ErrorMessage, &req.CreatedAt, &req.UpdatedAt); err != nil {
				http.Error(w, "Error scanning request: "+err.Error(), http.StatusInternalServerError)
				return
			}
			requests = append(requests, req)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"requests": requests,
			"count":    len(requests),
		})
	}
}

// DetalheApuracaoHandler returns details of a specific RFB request including debits
func DetalheApuracaoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		claims, ok := r.Context().Value(ClaimsKey).(jwt.MapClaims)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userID := claims["user_id"].(string)

		companyID, err := GetEffectiveCompanyID(db, userID, r.Header.Get("X-Company-ID"))
		if err != nil {
			http.Error(w, "Error getting company: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Extract request ID from path
		requestID := strings.TrimPrefix(r.URL.Path, "/api/rfb/apuracao/")
		requestID = strings.TrimSpace(requestID)
		if requestID == "" || requestID == "status" || requestID == "solicitar" {
			http.Error(w, "Invalid request ID", http.StatusBadRequest)
			return
		}

		// Fetch request (verify company ownership)
		var req RFBRequest
		err = db.QueryRow(`
			SELECT id, company_id, cnpj_base, COALESCE(tiquete, ''), status, ambiente,
				error_code, error_message, created_at, updated_at
			FROM rfb_requests
			WHERE id = $1 AND company_id = $2
		`, requestID, companyID).Scan(&req.ID, &req.CompanyID, &req.CNPJBase, &req.Tiquete, &req.Status, &req.Ambiente,
			&req.ErrorCode, &req.ErrorMessage, &req.CreatedAt, &req.UpdatedAt)
		if err == sql.ErrNoRows {
			http.Error(w, "Solicitação não encontrada", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "Error querying request: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Fetch summary if available
		var resumo *RFBResumo
		var r2 RFBResumo
		err = db.QueryRow(`
			SELECT id, request_id, COALESCE(data_apuracao, ''), total_debitos,
				valor_cbs_total, valor_cbs_extinto, valor_cbs_nao_extinto,
				total_corrente, total_ajuste, total_extemporaneo
			FROM rfb_resumo WHERE request_id = $1
		`, requestID).Scan(&r2.ID, &r2.RequestID, &r2.DataApuracao, &r2.TotalDebitos,
			&r2.ValorCBSTotal, &r2.ValorCBSExtinto, &r2.ValorCBSNaoExtinto,
			&r2.TotalCorrente, &r2.TotalAjuste, &r2.TotalExtemporaneo)
		if err == nil {
			resumo = &r2
		}

		// Fetch debits
		debitRows, err := db.Query(`
			SELECT id, tipo_apuracao, COALESCE(modelo_dfe, ''), COALESCE(numero_dfe, ''),
				COALESCE(chave_dfe, ''),
				CASE WHEN data_dfe_emissao IS NOT NULL THEN to_char(data_dfe_emissao, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') END,
				COALESCE(data_apuracao, ''),
				COALESCE(ni_emitente, ''), COALESCE(ni_adquirente, ''),
				COALESCE(valor_cbs_total, 0), COALESCE(valor_cbs_extinto, 0), COALESCE(valor_cbs_nao_extinto, 0),
				COALESCE(situacao_debito, '')
			FROM rfb_debitos
			WHERE request_id = $1
			ORDER BY tipo_apuracao, data_apuracao
		`, requestID)
		if err != nil {
			http.Error(w, "Error querying debits: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer debitRows.Close()

		var debitos []RFBDebitoRow
		for debitRows.Next() {
			var d RFBDebitoRow
			if err := debitRows.Scan(&d.ID, &d.TipoApuracao, &d.ModeloDfe, &d.NumeroDfe,
				&d.ChaveDfe, &d.DataDfeEmissao, &d.DataApuracao,
				&d.NiEmitente, &d.NiAdquirente,
				&d.ValorCBSTotal, &d.ValorCBSExtinto, &d.ValorCBSNaoExtinto,
				&d.SituacaoDebito); err != nil {
				log.Printf("[RFB Detail] Error scanning debit: %v", err)
				continue
			}
			debitos = append(debitos, d)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"request": req,
			"resumo":  resumo,
			"debitos": debitos,
		})
	}
}
