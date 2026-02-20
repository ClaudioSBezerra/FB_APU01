package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"fb_apu01/services"

	"github.com/golang-jwt/jwt/v5"
)

// ApuracaoResumo holds aggregated fiscal data for AI prompt generation.
type ApuracaoResumo struct {
	CompanyName string  `json:"company_name"`
	CNPJ        string  `json:"cnpj"`
	Periodo     string  `json:"periodo"`
	// Current period
	FaturamentoBruto  float64 `json:"faturamento_bruto"`
	TotalEntradas     float64 `json:"total_entradas"`
	TotalSaidas       float64 `json:"total_saidas"`
	IcmsEntrada       float64 `json:"icms_entrada"`
	IcmsSaida         float64 `json:"icms_saida"`
	IcmsAPagar        float64 `json:"icms_a_pagar"`
	IbsProjetado      float64 `json:"ibs_projetado"`
	CbsProjetado      float64 `json:"cbs_projetado"`
	TotalNFes         int     `json:"total_nfes"`
	// Previous period (for comparison)
	PeriodoAnterior       string  `json:"periodo_anterior"`
	FaturamentoAnterior   float64 `json:"faturamento_anterior"`
	IcmsAPagarAnterior    float64 `json:"icms_a_pagar_anterior"`
	TotalNFesAnterior     int     `json:"total_nfes_anterior"`
	// Breakdown by operation type
	Operacoes []OperacaoResumo `json:"operacoes"`
	// Import jobs info
	UltimaImportacao string `json:"ultima_importacao"`
	TotalJobs        int    `json:"total_jobs"`
	// Alíquotas efetivas (% sobre faturamento bruto)
	AliquotaEfetivaICMS         float64 `json:"aliquota_efetiva_icms"`
	AliquotaEfetivaIBS          float64 `json:"aliquota_efetiva_ibs"`
	AliquotaEfetivaCBS          float64 `json:"aliquota_efetiva_cbs"`
	AliquotaEfetivaTotalReforma float64 `json:"aliquota_efetiva_total_reforma"`
	AliquotaEfetivaICMSAnterior float64 `json:"aliquota_efetiva_icms_anterior"`
}

type OperacaoResumo struct {
	TipoOperacao string  `json:"tipo_operacao"`
	Tipo         string  `json:"tipo"` // ENTRADA or SAIDA
	Valor        float64 `json:"valor"`
	Icms         float64 `json:"icms"`
}

// Executive Summary response
type ExecutiveSummaryResponse struct {
	Narrativa string          `json:"narrativa"`
	Dados     *ApuracaoResumo `json:"dados"`
	Periodo   string          `json:"periodo"`
	Model     string          `json:"model,omitempty"`
	Cached    bool            `json:"cached"`
}

// Insight response
type InsightResponse struct {
	Texto    string `json:"texto"`
	Tipo     string `json:"tipo"` // alerta, info, positivo
	AcaoURL  string `json:"acao_url,omitempty"`
	AcaoText string `json:"acao_text,omitempty"`
	Cached   bool   `json:"cached"`
}

// getApuracaoResumo aggregates fiscal data from materialized views.
func getApuracaoResumo(db *sql.DB, companyID, periodo string) (*ApuracaoResumo, error) {
	resumo := &ApuracaoResumo{Periodo: periodo}

	// Get company info (cnpj may not exist in all schemas)
	err := db.QueryRow(`
		SELECT COALESCE(c.name, ''), COALESCE(c.trade_name, '')
		FROM companies c WHERE c.id = $1
	`, companyID).Scan(&resumo.CompanyName, &resumo.CNPJ)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("query company: %w", err)
	}

	// Aggregate current period from MV
	rows, err := db.Query(`
		SELECT
			mv.tipo,
			mv.tipo_operacao,
			COALESCE(SUM(mv.valor_contabil), 0) as valor,
			COALESCE(SUM(mv.vl_icms_origem), 0) as icms
		FROM mv_mercadorias_agregada mv
		WHERE mv.company_id = $1 AND mv.mes_ano = $2
		GROUP BY mv.tipo, mv.tipo_operacao
	`, companyID, periodo)
	if err != nil {
		return nil, fmt.Errorf("query current period: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var op OperacaoResumo
		if err := rows.Scan(&op.Tipo, &op.TipoOperacao, &op.Valor, &op.Icms); err != nil {
			return nil, fmt.Errorf("scan operation: %w", err)
		}
		resumo.Operacoes = append(resumo.Operacoes, op)
		if op.Tipo == "ENTRADA" {
			resumo.TotalEntradas += op.Valor
			resumo.IcmsEntrada += op.Icms
		} else {
			resumo.TotalSaidas += op.Valor
			resumo.IcmsSaida += op.Icms
		}
	}
	resumo.FaturamentoBruto = resumo.TotalSaidas
	resumo.IcmsAPagar = resumo.IcmsSaida - resumo.IcmsEntrada
	if resumo.IcmsAPagar < 0 {
		resumo.IcmsAPagar = 0
	}

	// Count NFes for current period
	db.QueryRow(`
		SELECT COUNT(DISTINCT mv.filial_cnpj || mv.mes_ano || mv.tipo_operacao)
		FROM mv_mercadorias_agregada mv
		WHERE mv.company_id = $1 AND mv.mes_ano = $2
	`, companyID, periodo).Scan(&resumo.TotalNFes)

	// Previous period comparison
	prevPeriodo := calcPreviousPeriod(periodo)
	resumo.PeriodoAnterior = prevPeriodo

	var prevEntradas, prevSaidas, prevIcmsEntrada, prevIcmsSaida float64
	prevRows, err := db.Query(`
		SELECT
			mv.tipo,
			COALESCE(SUM(mv.valor_contabil), 0) as valor,
			COALESCE(SUM(mv.vl_icms_origem), 0) as icms
		FROM mv_mercadorias_agregada mv
		WHERE mv.company_id = $1 AND mv.mes_ano = $2
		GROUP BY mv.tipo
	`, companyID, prevPeriodo)
	if err == nil {
		defer prevRows.Close()
		for prevRows.Next() {
			var tipo string
			var valor, icms float64
			if err := prevRows.Scan(&tipo, &valor, &icms); err == nil {
				if tipo == "ENTRADA" {
					prevEntradas = valor
					prevIcmsEntrada = icms
				} else {
					prevSaidas = valor
					prevIcmsSaida = icms
				}
			}
		}
	}
	resumo.FaturamentoAnterior = prevSaidas
	resumo.IcmsAPagarAnterior = prevIcmsSaida - prevIcmsEntrada
	if resumo.IcmsAPagarAnterior < 0 {
		resumo.IcmsAPagarAnterior = 0
	}
	_ = prevEntradas // used implicitly in icms calc

	// Last import info
	db.QueryRow(`
		SELECT COUNT(*), COALESCE(MAX(created_at)::text, 'nunca')
		FROM import_jobs WHERE company_id = $1 AND status = 'completed'
	`, companyID).Scan(&resumo.TotalJobs, &resumo.UltimaImportacao)

	// Calculate IBS/CBS projections using same logic as Dashboard (year 2033 = full reform)
	// Uses mv_mercadorias_agregada with NET calculation (Debit - Credit), excludes CFOP T and O
	ibsCbsRows, err := db.Query(`
		SELECT
			tipo,
			COALESCE(SUM(CASE WHEN tipo_cfop NOT IN ('T', 'O') THEN valor_contabil ELSE 0 END), 0) as taxable_valor,
			COALESCE(SUM(CASE WHEN tipo_cfop NOT IN ('T', 'O') THEN vl_icms_origem ELSE 0 END), 0) as taxable_icms
		FROM mv_mercadorias_agregada
		WHERE company_id = $1 AND mes_ano = $2
		GROUP BY tipo
	`, companyID, periodo)
	if err == nil {
		defer ibsCbsRows.Close()
		// Use 2033 rates (full reform implementation)
		var percIBSUF, percIBSMun, percCBS, percReducICMS float64
		rateErr := db.QueryRow(`SELECT perc_ibs_uf, perc_ibs_mun, perc_cbs, perc_reduc_icms FROM tabela_aliquotas WHERE ano = 2033`).
			Scan(&percIBSUF, &percIBSMun, &percCBS, &percReducICMS)
		if rateErr != nil {
			// Default 2033 rates
			percIBSUF = 26.0
			percIBSMun = 5.0
			percCBS = 8.80
			percReducICMS = 100.0
		}
		ibsRate := (percIBSUF + percIBSMun) / 100.0
		cbsRate := percCBS / 100.0
		var ibsDebit, ibsCredit, cbsDebit, cbsCredit float64
		for ibsCbsRows.Next() {
			var tipo string
			var valTax, icmsTax float64
			if err := ibsCbsRows.Scan(&tipo, &valTax, &icmsTax); err != nil {
				continue
			}
			icmsProj := icmsTax * (1.0 - (percReducICMS / 100.0))
			base := valTax - icmsProj
			if tipo == "SAIDA" {
				ibsDebit = base * ibsRate
				cbsDebit = base * cbsRate
			} else {
				ibsCredit = base * ibsRate
				cbsCredit = base * cbsRate
			}
		}
		resumo.IbsProjetado = ibsDebit - ibsCredit
		resumo.CbsProjetado = cbsDebit - cbsCredit
		if resumo.IbsProjetado < 0 {
			resumo.IbsProjetado = 0
		}
		if resumo.CbsProjetado < 0 {
			resumo.CbsProjetado = 0
		}
	}

	// Calcular alíquotas efetivas (% sobre faturamento bruto)
	if resumo.FaturamentoBruto > 0 {
		resumo.AliquotaEfetivaICMS = math.Round((resumo.IcmsAPagar/resumo.FaturamentoBruto)*10000) / 100
		resumo.AliquotaEfetivaIBS = math.Round((resumo.IbsProjetado/resumo.FaturamentoBruto)*10000) / 100
		resumo.AliquotaEfetivaCBS = math.Round((resumo.CbsProjetado/resumo.FaturamentoBruto)*10000) / 100
		resumo.AliquotaEfetivaTotalReforma = math.Round(((resumo.IbsProjetado+resumo.CbsProjetado)/resumo.FaturamentoBruto)*10000) / 100
	}
	if resumo.FaturamentoAnterior > 0 {
		resumo.AliquotaEfetivaICMSAnterior = math.Round((resumo.IcmsAPagarAnterior/resumo.FaturamentoAnterior)*10000) / 100
	}

	return resumo, nil
}

func calcPreviousPeriod(periodo string) string {
	// Format: MM/YYYY
	parts := strings.Split(periodo, "/")
	if len(parts) != 2 {
		return periodo
	}
	var month, year int
	fmt.Sscanf(parts[0], "%d", &month)
	fmt.Sscanf(parts[1], "%d", &year)
	month--
	if month < 1 {
		month = 12
		year--
	}
	return fmt.Sprintf("%02d/%04d", month, year)
}

func buildExecutiveSummaryPrompt(resumo *ApuracaoResumo) string {
	var sb strings.Builder
	sb.WriteString("IMPORTANTE: Responda EXCLUSIVAMENTE em português brasileiro (pt-BR). NÃO escreva em inglês.\n\n")
	sb.WriteString(fmt.Sprintf("Empresa: %s (CNPJ: %s)\n", resumo.CompanyName, resumo.CNPJ))
	sb.WriteString(fmt.Sprintf("Periodo de apuracao: %s\n\n", resumo.Periodo))
	sb.WriteString("DADOS DO PERIODO ATUAL:\n")
	sb.WriteString(fmt.Sprintf("- Faturamento bruto (saidas): R$ %.2f\n", resumo.FaturamentoBruto))
	sb.WriteString(fmt.Sprintf("- Total de entradas: R$ %.2f\n", resumo.TotalEntradas))
	sb.WriteString(fmt.Sprintf("- ICMS sobre saidas (debito): R$ %.2f\n", resumo.IcmsSaida))
	sb.WriteString(fmt.Sprintf("- ICMS sobre entradas (credito): R$ %.2f\n", resumo.IcmsEntrada))
	sb.WriteString(fmt.Sprintf("- ICMS a recolher (debito - credito): R$ %.2f\n", resumo.IcmsAPagar))

	sb.WriteString("\nNOVOS IMPOSTOS - REFORMA TRIBUTARIA (Projecao 2033 - Implementacao Completa):\n")
	sb.WriteString("NOTA: Valores calculados como SALDO A PAGAR (debito saidas - credito entradas), mesma logica do painel operacional.\n")
	sb.WriteString(fmt.Sprintf("- IBS projetado a pagar (Imposto sobre Bens e Servicos): R$ %.2f\n", resumo.IbsProjetado))
	sb.WriteString(fmt.Sprintf("- CBS projetado a pagar (Contribuicao sobre Bens e Servicos): R$ %.2f\n", resumo.CbsProjetado))
	sb.WriteString(fmt.Sprintf("- Total IBS + CBS a pagar: R$ %.2f\n", resumo.IbsProjetado+resumo.CbsProjetado))

	if resumo.FaturamentoBruto > 0 {
		sb.WriteString("\nALIQUOTA EFETIVA DO NEGOCIO (sobre faturamento bruto):\n")
		sb.WriteString(fmt.Sprintf("- ICMS efetivo atual: %.2f%%\n", resumo.AliquotaEfetivaICMS))
		sb.WriteString(fmt.Sprintf("- IBS efetivo projetado (2033): %.2f%%\n", resumo.AliquotaEfetivaIBS))
		sb.WriteString(fmt.Sprintf("- CBS efetivo projetado (2033): %.2f%%\n", resumo.AliquotaEfetivaCBS))
		sb.WriteString(fmt.Sprintf("- Total IBS+CBS efetivo (2033): %.2f%%\n", resumo.AliquotaEfetivaTotalReforma))
		if resumo.AliquotaEfetivaICMSAnterior > 0 {
			varAliq := resumo.AliquotaEfetivaICMS - resumo.AliquotaEfetivaICMSAnterior
			sb.WriteString(fmt.Sprintf("- ICMS efetivo periodo anterior: %.2f%% (variacao: %+.2f p.p.)\n", resumo.AliquotaEfetivaICMSAnterior, varAliq))
		}
	}

	if resumo.FaturamentoAnterior > 0 {
		varFat := ((resumo.FaturamentoBruto - resumo.FaturamentoAnterior) / resumo.FaturamentoAnterior) * 100
		varIcms := 0.0
		if resumo.IcmsAPagarAnterior > 0 {
			varIcms = ((resumo.IcmsAPagar - resumo.IcmsAPagarAnterior) / resumo.IcmsAPagarAnterior) * 100
		}
		sb.WriteString(fmt.Sprintf("\nCOMPARATIVO COM PERIODO ANTERIOR (%s):\n", resumo.PeriodoAnterior))
		sb.WriteString(fmt.Sprintf("- Faturamento anterior: R$ %.2f (variacao: %.1f%%)\n", resumo.FaturamentoAnterior, varFat))
		sb.WriteString(fmt.Sprintf("- ICMS a recolher anterior: R$ %.2f (variacao: %.1f%%)\n", resumo.IcmsAPagarAnterior, varIcms))
	}

	if len(resumo.Operacoes) > 0 {
		sb.WriteString("\nDETALHAMENTO POR TIPO DE OPERACAO:\n")
		for _, op := range resumo.Operacoes {
			sb.WriteString(fmt.Sprintf("- %s (%s): Valor R$ %.2f | ICMS R$ %.2f\n", op.TipoOperacao, op.Tipo, op.Valor, op.Icms))
		}
	}

	sb.WriteString(fmt.Sprintf("\nINFORMACOES ADICIONAIS:\n"))
	sb.WriteString(fmt.Sprintf("- Total de importacoes realizadas: %d\n", resumo.TotalJobs))
	sb.WriteString(fmt.Sprintf("- Ultima importacao: %s\n", resumo.UltimaImportacao))

	return sb.String()
}

const executiveSummarySystem = `Voce e um assistente fiscal especializado em tributacao brasileira e na Reforma Tributaria.

INSTRUCAO CRITICA: Responda DIRETAMENTE com o relatorio em Markdown. NAO inclua raciocinio, analise previa, passos numerados ou pensamento interno. Comece sua resposta IMEDIATAMENTE com "## Resumo Executivo".

Gere um RESUMO EXECUTIVO MENSAL de apuracao fiscal para um CEO ou Controller.

REGRAS:
- Escreva EXCLUSIVAMENTE em portugues brasileiro (pt-BR). NUNCA use ingles.
- Tom profissional e direto
- Valores monetarios: R$ XX.XXX,00 (milhar com ponto, decimal com virgula)
- Formatacao Markdown (## headers, **negrito**, listas)
- Inclua obrigatoriamente:
  1. Situacao geral (1-2 frases)
  2. Impostos a recolher
  3. Tabela comparativa em Markdown com aliquota efetiva:
     | Imposto | Valor | Aliquota Efetiva | Observacao |
     |---------|-------|------------------|------------|
     | ICMS a Recolher | R$ X | X.XX% | Regime atual |
     | IBS Projetado a Pagar | R$ X | X.XX% | Novo imposto - projecao 2033 |
     | CBS Projetado a Pagar | R$ X | X.XX% | Novo imposto - projecao 2033 |
     | Total IBS + CBS a Pagar | R$ X | X.XX% | Substituira ICMS + PIS/COFINS |
  4. Comentario sobre aliquota efetiva: compare ICMS efetivo atual vs total IBS+CBS efetivo, explique o impacto pratico
  5. Comparativo com periodo anterior (se disponivel) com variacao percentual e variacao em pontos percentuais da aliquota efetiva
  6. Destaques relevantes
  7. Recomendacoes praticas (2-3 itens)
- NAO invente dados. Use APENAS os numeros fornecidos.
- Mantenha entre 300-500 palavras.
- Comece DIRETO com "## Resumo Executivo" sem nenhum texto antes.`

const insightSystem = `Voce e um assistente fiscal que gera insights curtos para um dashboard.
Gere EXATAMENTE UMA frase (maximo 2 frases) com um insight relevante sobre a situacao fiscal.

REGRAS:
- Escreva em portugues brasileiro (pt-BR)
- Maximo 2 frases curtas e diretas
- Formate valores como R$ XX.XXX,00
- Inclua numeros concretos quando possivel
- Foque no que e mais relevante AGORA: vencimentos, variacao significativa, creditos nao usados, anomalias
- Tom informativo, nao alarmista
- NAO use Markdown, apenas texto puro
- NAO invente dados

Responda APENAS com o texto do insight, sem prefixos como "Insight:" ou "Dica:".
Apos a frase, em uma nova linha, escreva o TIPO do insight: alerta, info, ou positivo.`

// GetExecutiveSummaryHandler generates an AI-powered executive summary.
func GetExecutiveSummaryHandler(db *sql.DB) http.HandlerFunc {
	aiClient := services.NewAIClient()

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

		periodo := r.URL.Query().Get("periodo")
		if periodo == "" {
			// Default to current month
			now := time.Now()
			periodo = fmt.Sprintf("%02d/%04d", now.Month(), now.Year())
		}

		// Check if force regeneration requested (skip cache)
		forceRegen := r.URL.Query().Get("force") == "true"

		// Check if we already have a saved report for this company/period (to save tokens)
		var savedNarrativa string
		var savedModel string
		var resumo *ApuracaoResumo

		if !forceRegen {
			db.QueryRow(`
				SELECT resumo, 'cached' as model
				FROM ai_reports
				WHERE company_id = $1 AND periodo = $2
				ORDER BY created_at DESC
				LIMIT 1
			`, companyID, periodo).Scan(&savedNarrativa, &savedModel)
		}

		// If found saved report, aggregate data and return cached version
		if !forceRegen && savedNarrativa != "" {
			resumo, err = getApuracaoResumo(db, companyID, periodo)
			if err != nil {
				http.Error(w, "Error aggregating data: "+err.Error(), http.StatusInternalServerError)
				return
			}

			response := ExecutiveSummaryResponse{
				Narrativa: savedNarrativa,
				Dados:     resumo,
				Periodo:   periodo,
				Model:     savedModel,
				Cached:    true,
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		// No saved report found - generate new one
		// Aggregate data
		resumo, err = getApuracaoResumo(db, companyID, periodo)
		if err != nil {
			http.Error(w, "Error aggregating data: "+err.Error(), http.StatusInternalServerError)
			return
		}

		response := ExecutiveSummaryResponse{
			Dados:   resumo,
			Periodo: periodo,
			Cached:  false,
		}

		// Check if there's data to analyze
		if resumo.FaturamentoBruto == 0 && resumo.TotalEntradas == 0 {
			response.Narrativa = fmt.Sprintf("## Resumo Executivo - %s\n\nNenhum dado fiscal encontrado para o periodo %s. Importe arquivos SPED EFD para gerar o resumo executivo com inteligencia artificial.", periodo, periodo)
			json.NewEncoder(w).Encode(response)
			return
		}

		// Generate AI narrative
		if aiClient == nil || !aiClient.IsAvailable() {
			response.Narrativa = buildFallbackNarrative(resumo)
			json.NewEncoder(w).Encode(response)
			return
		}

		dataPrompt := buildExecutiveSummaryPrompt(resumo)
		// GenerateFast: 1 tentativa, 25s timeout — worker trata retries em background
		aiResp, err := aiClient.GenerateFast(executiveSummarySystem, dataPrompt, services.ModelFlash, 4096)
		if err != nil {
			fmt.Printf("AI generation error (falling back): %v\n", err)
			// Re-check cache — worker may have saved a report while we were waiting
			var workerNarrativa string
			errRecheck := db.QueryRow(`
				SELECT resumo FROM ai_reports
				WHERE company_id = $1 AND periodo = $2
				ORDER BY created_at DESC LIMIT 1
			`, companyID, periodo).Scan(&workerNarrativa)
			if errRecheck == nil && workerNarrativa != "" {
				response.Narrativa = workerNarrativa
				response.Model = "cached"
				response.Cached = true
			} else {
				response.Narrativa = buildFallbackNarrative(resumo)
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		response.Narrativa = aiResp.Text
		response.Model = aiResp.Model

		// If force regeneration, save report and send email to managers
		if forceRegen && response.Narrativa != "" {
			go func() {
				// Save to ai_reports
				dadosBrutos := buildDadosBrutosJSON(resumo)
				titulo := fmt.Sprintf("%s | %s (regenerado)", resumo.CompanyName, resumo.Periodo)
				_, errSave := db.Exec(`
					INSERT INTO ai_reports (company_id, periodo, titulo, resumo, dados_brutos, gerado_automaticamente)
					VALUES ($1, $2, $3, $4, $5, false)
				`, companyID, periodo, titulo, response.Narrativa, dadosBrutos)
				if errSave != nil {
					fmt.Printf("[Regenerate] Error saving report: %v\n", errSave)
				}

				// Send email to active managers
				var managers []struct{ Email string }
				rows, errMgr := db.Query(`SELECT email FROM managers WHERE company_id = $1 AND ativo = true`, companyID)
				if errMgr == nil {
					defer rows.Close()
					for rows.Next() {
						var m struct{ Email string }
						if rows.Scan(&m.Email) == nil {
							managers = append(managers, m)
						}
					}
				}
				if len(managers) > 0 {
					recipients := make([]string, len(managers))
					for i, m := range managers {
						recipients[i] = m.Email
					}
					taxData := services.TaxComparisonData{
						IcmsAPagar:   resumo.IcmsAPagar,
						IbsProjetado: resumo.IbsProjetado,
						CbsProjetado: resumo.CbsProjetado,
					}
					errEmail := services.SendAIReportEmail(recipients, resumo.CompanyName, periodo, response.Narrativa, dadosBrutos, taxData)
					if errEmail != nil {
						fmt.Printf("[Regenerate] Error sending email: %v\n", errEmail)
					} else {
						fmt.Printf("[Regenerate] Email sent to %d managers\n", len(recipients))
					}
				}
			}()
		}

		json.NewEncoder(w).Encode(response)
	}
}

// buildDadosBrutosJSON converts ApuracaoResumo to JSON string for storage
func buildDadosBrutosJSON(r *ApuracaoResumo) string {
	data := map[string]any{
		"empresa":        r.CompanyName,
		"cnpj":           r.CNPJ,
		"periodo":        r.Periodo,
		"faturamento":    r.FaturamentoBruto,
		"total_entradas": r.TotalEntradas,
		"total_saidas":   r.TotalSaidas,
		"icms_entrada":   r.IcmsEntrada,
		"icms_saida":     r.IcmsSaida,
		"icms_a_pagar":   r.IcmsAPagar,
		"ibs_projetado":  r.IbsProjetado,
		"cbs_projetado":  r.CbsProjetado,
		"ibs_cbs_total":  r.IbsProjetado + r.CbsProjetado,
		"total_nfes":     r.TotalNFes,
		"operacoes":      r.Operacoes,
	}
	jsonBytes, _ := json.Marshal(data)
	return string(jsonBytes)
}

// GetDailyInsightHandler generates a short AI-powered insight for the dashboard.
func GetDailyInsightHandler(db *sql.DB) http.HandlerFunc {
	aiClient := services.NewAIClient()

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

		now := time.Now()
		periodo := fmt.Sprintf("%02d/%04d", now.Month(), now.Year())

		resumo, err := getApuracaoResumo(db, companyID, periodo)
		if err != nil {
			http.Error(w, "Error aggregating data: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// No data: return generic insight
		if resumo.FaturamentoBruto == 0 && resumo.TotalEntradas == 0 && resumo.TotalJobs == 0 {
			json.NewEncoder(w).Encode(InsightResponse{
				Texto:    "Importe seus arquivos SPED EFD para comecar a receber insights fiscais personalizados com inteligencia artificial.",
				Tipo:     "info",
				AcaoURL:  "/importar",
				AcaoText: "Importar SPED",
			})
			return
		}

		// If no AI client, generate deterministic insight
		if aiClient == nil || !aiClient.IsAvailable() {
			json.NewEncoder(w).Encode(buildFallbackInsight(resumo))
			return
		}

		dataPrompt := buildExecutiveSummaryPrompt(resumo)
		aiResp, err := aiClient.GenerateFast(insightSystem, dataPrompt, services.ModelFlash, 256)
		if err != nil {
			fmt.Printf("AI insight error (falling back): %v\n", err)
			json.NewEncoder(w).Encode(buildFallbackInsight(resumo))
			return
		}

		// Parse response: first line = texto, second line = tipo
		lines := strings.SplitN(strings.TrimSpace(aiResp.Text), "\n", 2)
		texto := lines[0]
		tipo := "info"
		if len(lines) > 1 {
			t := strings.TrimSpace(strings.ToLower(lines[1]))
			if t == "alerta" || t == "positivo" || t == "info" {
				tipo = t
			}
		}

		json.NewEncoder(w).Encode(InsightResponse{
			Texto:  texto,
			Tipo:   tipo,
			Cached: false,
		})
	}
}

// buildFallbackNarrative generates a narrative without AI when the API is unavailable.
func buildFallbackNarrative(r *ApuracaoResumo) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Resumo Executivo - %s\n\n", r.Periodo))
	sb.WriteString(fmt.Sprintf("**Empresa:** %s | **CNPJ:** %s\n\n", r.CompanyName, r.CNPJ))
	sb.WriteString(fmt.Sprintf("**Faturamento bruto:** R$ %s\n\n", formatBRL(r.FaturamentoBruto)))
	sb.WriteString(fmt.Sprintf("**ICMS a recolher:** R$ %s (debito R$ %s - credito R$ %s)\n\n",
		formatBRL(r.IcmsAPagar), formatBRL(r.IcmsSaida), formatBRL(r.IcmsEntrada)))

	sb.WriteString("### Novos Impostos - Reforma Tributaria (Projecao)\n\n")
	sb.WriteString("| Imposto | Valor |\n")
	sb.WriteString("|---------|-------|\n")
	sb.WriteString(fmt.Sprintf("| IBS Projetado | R$ %s |\n", formatBRL(r.IbsProjetado)))
	sb.WriteString(fmt.Sprintf("| CBS Projetado | R$ %s |\n", formatBRL(r.CbsProjetado)))
	sb.WriteString(fmt.Sprintf("| **Total IBS + CBS** | **R$ %s** |\n\n", formatBRL(r.IbsProjetado+r.CbsProjetado)))

	if r.FaturamentoAnterior > 0 {
		varFat := ((r.FaturamentoBruto - r.FaturamentoAnterior) / r.FaturamentoAnterior) * 100
		direcao := "aumento"
		if varFat < 0 {
			direcao = "reducao"
		}
		sb.WriteString(fmt.Sprintf("**Comparativo com %s:** %s de %.1f%% no faturamento.\n\n", r.PeriodoAnterior, direcao, math.Abs(varFat)))
	}

	sb.WriteString("*Relatorio gerado com dados fiscais. A narrativa com IA sera incluida automaticamente quando disponivel.*")
	return sb.String()
}

// buildFallbackInsight generates a deterministic insight when AI is unavailable.
func buildFallbackInsight(r *ApuracaoResumo) InsightResponse {
	// Priority 1: Significant variation from previous period
	if r.FaturamentoAnterior > 0 {
		varPct := ((r.FaturamentoBruto - r.FaturamentoAnterior) / r.FaturamentoAnterior) * 100
		if math.Abs(varPct) > 10 {
			direcao := "aumento"
			tipo := "info"
			if varPct < 0 {
				direcao = "reducao"
				tipo = "alerta"
			} else {
				tipo = "positivo"
			}
			return InsightResponse{
				Texto: fmt.Sprintf("O faturamento de %s apresentou %s de %.1f%% em relacao a %s (R$ %s vs R$ %s).",
					r.Periodo, direcao, math.Abs(varPct), r.PeriodoAnterior, formatBRL(r.FaturamentoBruto), formatBRL(r.FaturamentoAnterior)),
				Tipo: tipo,
			}
		}
	}

	// Priority 2: ICMS info
	if r.IcmsAPagar > 0 {
		return InsightResponse{
			Texto: fmt.Sprintf("ICMS a recolher em %s: R$ %s. Total de creditos aproveitados: R$ %s.",
				r.Periodo, formatBRL(r.IcmsAPagar), formatBRL(r.IcmsEntrada)),
			Tipo: "info",
		}
	}

	// Priority 3: Generic import milestone
	return InsightResponse{
		Texto: fmt.Sprintf("Voce tem %d importacoes de SPED realizadas. Acesse o Resumo Executivo para uma analise completa.", r.TotalJobs),
		Tipo:  "info",
	}
}

func formatBRL(value float64) string {
	// Format as Brazilian currency without R$ prefix
	if value == 0 {
		return "0,00"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	intPart := int64(value)
	decPart := int64(math.Round((value - float64(intPart)) * 100))

	// Format integer part with dots as thousands separator
	intStr := fmt.Sprintf("%d", intPart)
	var parts []string
	for i := len(intStr); i > 0; i -= 3 {
		start := i - 3
		if start < 0 {
			start = 0
		}
		parts = append([]string{intStr[start:i]}, parts...)
	}
	result := strings.Join(parts, ".") + fmt.Sprintf(",%02d", decPart)
	if negative {
		return "-" + result
	}
	return result
}

// GetAvailablePeriodsHandler returns periods that have imported fiscal data for the company.
func GetAvailablePeriodsHandler(db *sql.DB) http.HandlerFunc {
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
			SELECT DISTINCT mes_ano
			FROM import_jobs
			WHERE company_id = $1 AND status = 'completed' AND mes_ano IS NOT NULL AND mes_ano != ''
			ORDER BY mes_ano DESC
		`, companyID)
		if err != nil {
			http.Error(w, "Error querying periods: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var periods []string
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err == nil {
				periods = append(periods, p)
			}
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"periods": periods,
			"latest":  func() string { if len(periods) > 0 { return periods[0] }; return "" }(),
		})
	}
}

// SavedAIReport represents a saved AI-generated report from database
type SavedAIReport struct {
	ID                   string    `json:"id"`
	CompanyID            string    `json:"company_id"`
	JobID                *string   `json:"job_id,omitempty"`
	Periodo              string    `json:"periodo"`
	Titulo               string    `json:"titulo"`
	Resumo               string    `json:"resumo"`
	GeradoAutomaticamente bool      `json:"gerado_automaticamente"`
	CreatedAt            time.Time `json:"created_at"`
}

// ListSavedAIReportsHandler returns all saved AI reports for a company
func ListSavedAIReportsHandler(db *sql.DB) http.HandlerFunc {
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
			SELECT id, company_id, job_id, periodo, titulo, resumo, gerado_automaticamente, created_at
			FROM ai_reports
			WHERE company_id = $1
			ORDER BY created_at DESC
			LIMIT 50
		`, companyID)
		if err != nil {
			http.Error(w, "Error querying AI reports: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var reports []SavedAIReport
		for rows.Next() {
			var r SavedAIReport
			if err := rows.Scan(&r.ID, &r.CompanyID, &r.JobID, &r.Periodo, &r.Titulo, &r.Resumo, &r.GeradoAutomaticamente, &r.CreatedAt); err != nil {
				http.Error(w, "Error scanning AI report: "+err.Error(), http.StatusInternalServerError)
				return
			}
			reports = append(reports, r)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"reports": reports,
			"count":   len(reports),
		})
	}
}

// GetSavedAIReportHandler returns a single saved AI report by ID
func GetSavedAIReportHandler(db *sql.DB) http.HandlerFunc {
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

		// Extract report ID from URL path
		path := r.URL.Path
		reportID := ""
		if len(path) > len("/api/reports/") {
			reportID = path[len("/api/reports/"):]
		}
		if reportID == "" {
			http.Error(w, "Invalid report ID", http.StatusBadRequest)
			return
		}

		// Get report
		var report SavedAIReport
		err = db.QueryRow(`
			SELECT id, company_id, job_id, periodo, titulo, resumo, gerado_automaticamente, created_at
			FROM ai_reports
			WHERE id = $1 AND company_id = $2
		`, reportID, companyID).Scan(&report.ID, &report.CompanyID, &report.JobID, &report.Periodo, &report.Titulo, &report.Resumo, &report.GeradoAutomaticamente, &report.CreatedAt)
		if err == sql.ErrNoRows {
			http.Error(w, "Report not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "Error getting AI report: "+err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(report)
	}
}
