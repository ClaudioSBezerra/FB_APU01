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
	sb.WriteString(fmt.Sprintf("Empresa: %s (CNPJ: %s)\n", resumo.CompanyName, resumo.CNPJ))
	sb.WriteString(fmt.Sprintf("Periodo de apuracao: %s\n\n", resumo.Periodo))
	sb.WriteString("DADOS DO PERIODO ATUAL:\n")
	sb.WriteString(fmt.Sprintf("- Faturamento bruto (saidas): R$ %.2f\n", resumo.FaturamentoBruto))
	sb.WriteString(fmt.Sprintf("- Total de entradas: R$ %.2f\n", resumo.TotalEntradas))
	sb.WriteString(fmt.Sprintf("- ICMS sobre saidas (debito): R$ %.2f\n", resumo.IcmsSaida))
	sb.WriteString(fmt.Sprintf("- ICMS sobre entradas (credito): R$ %.2f\n", resumo.IcmsEntrada))
	sb.WriteString(fmt.Sprintf("- ICMS a recolher (debito - credito): R$ %.2f\n", resumo.IcmsAPagar))

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

const executiveSummarySystem = `Voce e um assistente fiscal especializado em tributacao brasileira.
Gere um RESUMO EXECUTIVO MENSAL de apuracao fiscal para ser lido por um CEO ou Controller que nao e especialista em tributos.

REGRAS:
- Escreva em portugues brasileiro (pt-BR)
- Use tom profissional e direto, sem jargao excessivo
- Formate valores monetarios como R$ XX.XXX,00 (separador de milhar com ponto, decimal com virgula)
- Use Markdown para formatacao (headers ##, **negrito**, listas)
- Inclua obrigatoriamente:
  1. Situacao geral (1-2 frases de abertura)
  2. Impostos a recolher com valores
  3. Comparativo com periodo anterior (se disponivel) com variacao percentual
  4. Destaques: o que subiu, o que caiu e por que
  5. Recomendacoes praticas (2-3 itens)
- NAO invente dados. Use APENAS os numeros fornecidos.
- Se nao houver dados suficientes, diga "Dados insuficientes para esta analise".
- Mantenha o relatorio entre 200-400 palavras.`

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

		// Aggregate data
		resumo, err := getApuracaoResumo(db, companyID, periodo)
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
		aiResp, err := aiClient.Generate(executiveSummarySystem, dataPrompt, services.ModelHaiku, 2048)
		if err != nil {
			fmt.Printf("AI generation error (falling back): %v\n", err)
			response.Narrativa = buildFallbackNarrative(resumo)
			json.NewEncoder(w).Encode(response)
			return
		}

		response.Narrativa = aiResp.Text
		response.Model = aiResp.Model
		json.NewEncoder(w).Encode(response)
	}
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
		aiResp, err := aiClient.Generate(insightSystem, dataPrompt, services.ModelHaiku, 256)
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

	if r.FaturamentoAnterior > 0 {
		varFat := ((r.FaturamentoBruto - r.FaturamentoAnterior) / r.FaturamentoAnterior) * 100
		direcao := "aumento"
		if varFat < 0 {
			direcao = "reducao"
		}
		sb.WriteString(fmt.Sprintf("**Comparativo com %s:** %s de %.1f%% no faturamento.\n\n", r.PeriodoAnterior, direcao, math.Abs(varFat)))
	}

	sb.WriteString("*Relatorio gerado sem IA (ANTHROPIC_API_KEY nao configurada). Configure a chave para relatorios com narrativa inteligente.*")
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
