package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// RFBTime handles RFB datetime strings that may lack timezone suffix (e.g. "2026-03-01T08:30:09").
type RFBTime struct {
	T *time.Time
}

func (rt *RFBTime) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if s == "null" || s == "" {
		rt.T = nil
		return nil
	}
	for _, format := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.999999999",
	} {
		if t, err := time.Parse(format, s); err == nil {
			rt.T = &t
			return nil
		}
	}
	// Log and ignore unparseable dates rather than failing the whole import
	log.Printf("[RFB Processor] WARNING: could not parse datetime '%s', storing as nil", s)
	rt.T = nil
	return nil
}

// RFB JSON structures matching the API response layout
type RFBApuracaoJSON struct {
	ApuracaoCorrente     *RFBGrupoDebitos `json:"apuracaoCorrente"`
	ApuracaoAjuste       *RFBGrupoDebitos `json:"apuracaoAjuste"`
	DebitosExtemporaneos *RFBGrupoDebitos `json:"debitosExtemporaneos"`
}

type RFBGrupoDebitos struct {
	Debitos []RFBDebito `json:"debitos"`
}

type RFBDebito struct {
	ModeloDfe          string          `json:"modeloDfe"`
	NumeroDfe          string          `json:"numeroDfe"`
	ChaveDfe           string          `json:"chaveDfe"`
	DataDfeEmissao     *RFBTime        `json:"dataDfeEmissao"`
	DataDfeAutorizacao *RFBTime        `json:"dataDfeAutorizacao"`
	DataDfeRegistro    *RFBTime        `json:"dataDfeRegistro"`
	DataApuracao       string          `json:"dataApuracao"`
	NiEmitente         string          `json:"niEmitente"`
	NiAdquirente       string          `json:"niAdquirente"`
	ValorCBSTotal      float64         `json:"valorCBSTotal"`
	ValorCBSExtinto    float64         `json:"valorCBSExtinto"`
	ValorCBSNaoExtinto float64         `json:"valorCBSNaoExtinto"`
	SituacaoDebito     string          `json:"situacaoDebito"`
	FormasExtincao     json.RawMessage `json:"formasExtincao"`
	Eventos            json.RawMessage `json:"eventos"`
}

// ProcessarDownloadRFB downloads and processes the RFB CBS assessment JSON.
// It saves the raw JSON, normalizes debits into rfb_debitos, and creates a summary in rfb_resumo.
func ProcessarDownloadRFB(db *sql.DB, rfbClient *RFBClient, requestID string) error {
	log.Printf("[RFB Processor] Starting download processing for request %s", requestID)

	// 1. Fetch request details and company credentials
	var companyID, tiquete, cnpjBase string
	var tiqueteDownload *string
	err := db.QueryRow(`
		SELECT r.company_id, r.tiquete, r.cnpj_base, r.tiquete_download
		FROM rfb_requests r
		WHERE r.id = $1
	`, requestID).Scan(&companyID, &tiquete, &cnpjBase, &tiqueteDownload)
	if err != nil {
		return fmt.Errorf("failed to fetch request: %w", err)
	}

	var clientID, clientSecret, ambiente string
	err = db.QueryRow(`
		SELECT client_id, client_secret, COALESCE(ambiente, 'producao') FROM rfb_credentials
		WHERE company_id = $1 AND ativo = true
	`, companyID).Scan(&clientID, &clientSecret, &ambiente)
	if err != nil {
		updateRequestError(db, requestID, "CRED_NOT_FOUND", "Credenciais RFB não encontradas ou inativas")
		return fmt.Errorf("failed to fetch credentials: %w", err)
	}

	// Apply the correct API path prefix based on the registered environment
	rfbClient.SetAmbiente(ambiente)

	// Use tiqueteDownload if provided by the webhook (RFB sends two separate tíquetes)
	tiqueteParaDownload := tiquete
	if tiqueteDownload != nil && *tiqueteDownload != "" {
		tiqueteParaDownload = *tiqueteDownload
		log.Printf("[RFB Processor] Using tiqueteDownload '%s' (solicitacao: '%s')", tiqueteParaDownload, tiquete)
	} else {
		log.Printf("[RFB Processor] WARNING: tiqueteDownload not set, falling back to tiqueteSolicitacao '%s'", tiquete)
	}

	// 2. Get fresh OAuth2 token
	updateRequestStatus(db, requestID, "downloading")
	token, err := rfbClient.GetToken(clientID, clientSecret)
	if err != nil {
		updateRequestError(db, requestID, "TOKEN_ERROR", err.Error())
		return fmt.Errorf("failed to get token: %w", err)
	}

	// 3. Download the JSON file (single use per ticket!)
	rawJSON, err := rfbClient.DownloadArquivo(token, tiqueteParaDownload)
	if err != nil {
		updateRequestError(db, requestID, "DOWNLOAD_ERROR", err.Error())
		return fmt.Errorf("failed to download: %w", err)
	}

	// 4. Save raw JSON (skip if > 200 MB — PostgreSQL JSONB limit is ~268 MB)
	const maxRawJSONBytes = 200 * 1024 * 1024
	if len(rawJSON) <= maxRawJSONBytes {
		_, err = db.Exec(`
			UPDATE rfb_requests SET raw_json = $1, updated_at = CURRENT_TIMESTAMP
			WHERE id = $2
		`, rawJSON, requestID)
		if err != nil {
			log.Printf("[RFB Processor] WARNING: Failed to save raw JSON for request %s: %v", requestID, err)
		}
	} else {
		log.Printf("[RFB Processor] Skipping raw_json storage for request %s: too large (%d MB > 200 MB limit)",
			requestID, len(rawJSON)/1024/1024)
	}

	// 5. Parse JSON
	var apuracao RFBApuracaoJSON
	if err := json.Unmarshal(rawJSON, &apuracao); err != nil {
		updateRequestError(db, requestID, "PARSE_ERROR", "Falha ao interpretar JSON da RFB: "+err.Error())
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// 6. Normalize debits into rfb_debitos
	var totalCorrente, totalAjuste, totalExtemporaneo int
	var valorTotal, valorExtinto, valorNaoExtinto float64
	var dataApuracao string

	if apuracao.ApuracaoCorrente != nil {
		for _, d := range apuracao.ApuracaoCorrente.Debitos {
			if err := insertDebito(db, requestID, companyID, "corrente", d); err != nil {
				log.Printf("[RFB Processor] Error inserting corrente debit: %v", err)
			}
			totalCorrente++
			valorTotal += d.ValorCBSTotal
			valorExtinto += d.ValorCBSExtinto
			valorNaoExtinto += d.ValorCBSNaoExtinto
			if dataApuracao == "" && d.DataApuracao != "" {
				dataApuracao = d.DataApuracao
			}
		}
	}

	if apuracao.ApuracaoAjuste != nil {
		for _, d := range apuracao.ApuracaoAjuste.Debitos {
			if err := insertDebito(db, requestID, companyID, "ajuste", d); err != nil {
				log.Printf("[RFB Processor] Error inserting ajuste debit: %v", err)
			}
			totalAjuste++
			valorTotal += d.ValorCBSTotal
			valorExtinto += d.ValorCBSExtinto
			valorNaoExtinto += d.ValorCBSNaoExtinto
		}
	}

	if apuracao.DebitosExtemporaneos != nil {
		for _, d := range apuracao.DebitosExtemporaneos.Debitos {
			if err := insertDebito(db, requestID, companyID, "extemporaneo", d); err != nil {
				log.Printf("[RFB Processor] Error inserting extemporaneo debit: %v", err)
			}
			totalExtemporaneo++
			valorTotal += d.ValorCBSTotal
			valorExtinto += d.ValorCBSExtinto
			valorNaoExtinto += d.ValorCBSNaoExtinto
		}
	}

	totalDebitos := totalCorrente + totalAjuste + totalExtemporaneo

	// 7. Upsert summary in rfb_resumo
	_, err = db.Exec(`
		INSERT INTO rfb_resumo (request_id, company_id, data_apuracao, total_debitos,
			valor_cbs_total, valor_cbs_extinto, valor_cbs_nao_extinto,
			total_corrente, total_ajuste, total_extemporaneo)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (company_id, data_apuracao)
		DO UPDATE SET request_id = $1, total_debitos = $4,
			valor_cbs_total = $5, valor_cbs_extinto = $6, valor_cbs_nao_extinto = $7,
			total_corrente = $8, total_ajuste = $9, total_extemporaneo = $10
	`, requestID, companyID, dataApuracao, totalDebitos,
		valorTotal, valorExtinto, valorNaoExtinto,
		totalCorrente, totalAjuste, totalExtemporaneo)
	if err != nil {
		log.Printf("[RFB Processor] Error upserting summary: %v", err)
	}

	// 8. Mark request as completed
	updateRequestStatus(db, requestID, "completed")
	log.Printf("[RFB Processor] Request %s completed: %d debits (%d corrente, %d ajuste, %d extemporaneo), CBS total: %.2f",
		requestID, totalDebitos, totalCorrente, totalAjuste, totalExtemporaneo, valorTotal)

	return nil
}

func insertDebito(db *sql.DB, requestID, companyID, tipoApuracao string, d RFBDebito) error {
	formasExtincao := sql.NullString{}
	if len(d.FormasExtincao) > 0 && string(d.FormasExtincao) != "null" {
		formasExtincao = sql.NullString{String: string(d.FormasExtincao), Valid: true}
	}
	eventos := sql.NullString{}
	if len(d.Eventos) > 0 && string(d.Eventos) != "null" {
		eventos = sql.NullString{String: string(d.Eventos), Valid: true}
	}

	// Extract *time.Time from RFBTime wrapper (handles nil safely)
	var dataEmissao *time.Time
	if d.DataDfeEmissao != nil {
		dataEmissao = d.DataDfeEmissao.T
	}

	_, err := db.Exec(`
		INSERT INTO rfb_debitos (request_id, company_id, tipo_apuracao,
			modelo_dfe, numero_dfe, chave_dfe, data_dfe_emissao, data_apuracao,
			ni_emitente, ni_adquirente,
			valor_cbs_total, valor_cbs_extinto, valor_cbs_nao_extinto,
			situacao_debito, formas_extincao, eventos)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, requestID, companyID, tipoApuracao,
		d.ModeloDfe, d.NumeroDfe, d.ChaveDfe, dataEmissao, d.DataApuracao,
		d.NiEmitente, d.NiAdquirente,
		d.ValorCBSTotal, d.ValorCBSExtinto, d.ValorCBSNaoExtinto,
		d.SituacaoDebito, formasExtincao, eventos)
	return err
}

func updateRequestStatus(db *sql.DB, requestID, status string) {
	_, err := db.Exec(`
		UPDATE rfb_requests SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2
	`, status, requestID)
	if err != nil {
		log.Printf("[RFB Processor] Error updating request %s status to %s: %v", requestID, status, err)
	}
}

func updateRequestError(db *sql.DB, requestID, code, message string) {
	_, err := db.Exec(`
		UPDATE rfb_requests SET status = 'error', error_code = $1, error_message = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
	`, code, message, requestID)
	if err != nil {
		log.Printf("[RFB Processor] Error updating request %s error: %v", requestID, err)
	}
}
