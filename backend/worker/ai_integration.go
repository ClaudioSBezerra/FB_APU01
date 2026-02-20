package worker

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"fb_apu01/services"
)

// AIResumo holds aggregated fiscal data for AI report generation (simplified for worker)
type AIResumo struct {
	CompanyName       string   `json:"company_name"`
	CNPJ             string   `json:"cnpj"`
	Periodo          string   `json:"periodo"`
	FaturamentoBruto  float64  `json:"faturamento_bruto"`
	TotalEntradas    float64  `json:"total_entradas"`
	TotalSaidas      float64  `json:"total_saidas"`
	IcmsEntrada      float64  `json:"icms_entrada"`
	IcmsSaida        float64  `json:"icms_saida"`
	IcmsAPagar       float64  `json:"icms_a_pagar"`
	IbsProjetado     float64  `json:"ibs_projetado"`
	CbsProjetado     float64  `json:"cbs_projetado"`
	TotalNFes        int       `json:"total_nfs"`
	Operacoes        []AIOperacaoResumo `json:"operacoes"`
	// Alíquotas efetivas (% sobre faturamento bruto)
	AliquotaEfetivaICMS         float64 `json:"aliquota_efetiva_icms"`
	AliquotaEfetivaIBS          float64 `json:"aliquota_efetiva_ibs"`
	AliquotaEfetivaCBS          float64 `json:"aliquota_efetiva_cbs"`
	AliquotaEfetivaTotalReforma float64 `json:"aliquota_efetiva_total_reforma"`
}

// AIOperacaoResumo represents an operation for AI report
type AIOperacaoResumo struct {
	TipoOperacao string  `json:"tipo_operacao"`
	Tipo         string  `json:"tipo"`
	Valor        float64 `json:"valor"`
	Icms         float64 `json:"icms"`
}

// AIManager represents a manager for AI report emailing
type AIManager struct {
	ID           string    `json:"id"`
	CompanyID     string    `json:"company_id"`
	NomeCompleto  string    `json:"nome_completo"`
	Cargo         string    `json:"cargo"`
	Email         string    `json:"email"`
	Ativo         bool      `json:"ativo"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TriggerAIReportGeneration generates AI-powered executive summary and sends email to managers
func TriggerAIReportGeneration(db *sql.DB, companyID, periodo, jobID string) error {
	//1. Check if there are active managers for this company
	var managerCount int
	err := db.QueryRow("SELECT COUNT(*) FROM managers WHERE company_id = $1 AND ativo = true", companyID).Scan(&managerCount)
	if err != nil || managerCount == 0 {
		fmt.Printf("[AI Report] No active managers for company %s, skipping email\n", companyID)
		return nil // Not an error, just no one to email
	}

	//2. Aggregate fiscal data
	resumo, err := getApuracaoResumoForAI(db, companyID, periodo)
	if err != nil {
		return fmt.Errorf("error aggregating data: %w", err)
	}

	// Check if there's data to analyze
	if resumo.FaturamentoBruto == 0 && resumo.TotalEntradas == 0 {
		fmt.Printf("[AI Report] No fiscal data for company %s period %s, skipping AI report\n", companyID, periodo)
		return nil
	}

	//3. Generate AI narrative (with fallback if AI unavailable)
	var narrative string
	var modelUsed string

	aiClient := services.NewAIClient()
	if aiClient != nil && aiClient.IsAvailable() {
		dataPrompt := buildExecutiveSummaryPromptForAI(resumo)
		aiResp, err := aiClient.Generate(executiveSummarySystem, dataPrompt, services.ModelFlash, 4096)
		if err != nil {
			fmt.Printf("[AI Report] AI generation failed, using fallback: %v\n", err)
			narrative = buildFallbackNarrative(resumo)
			modelUsed = "fallback"
		} else {
			narrative = aiResp.Text
			modelUsed = aiResp.Model
		}
	} else {
		fmt.Println("[AI Report] AI client not available, using fallback narrative")
		narrative = buildFallbackNarrative(resumo)
		modelUsed = "fallback"
	}

	//4. Save report to database
	titulo := fmt.Sprintf("%s | %s", resumo.CompanyName, formatoPeriodoBR(periodo))
	dadosBrutosJSON := buildDadosBrutosJSON(resumo)

	var reportID string
	err = db.QueryRow(`
		INSERT INTO ai_reports (company_id, job_id, periodo, titulo, resumo, dados_brutos, gerado_automaticamente)
		VALUES ($1, $2, $3, $4, $5, $6, true)
		RETURNING id
	`, companyID, jobID, periodo, titulo, narrative, dadosBrutosJSON).Scan(&reportID)
	if err != nil {
		return fmt.Errorf("error saving AI report: %w", err)
	}
	fmt.Printf("[AI Report] Report saved to database: %s (model: %s)\n", reportID, modelUsed)

	//5. Get all active managers
	managers, err := getActiveManagersForAIReport(db, companyID)
	if err != nil {
		return fmt.Errorf("error getting managers: %w", err)
	}

	//6. Extract emails
	recipients := make([]string, 0, len(managers))
	for _, m := range managers {
		recipients = append(recipients, m.Email)
	}

	//7. Send email with tax comparison data
	taxData := services.TaxComparisonData{
		IcmsAPagar:   resumo.IcmsAPagar,
		IbsProjetado: resumo.IbsProjetado,
		CbsProjetado: resumo.CbsProjetado,
	}
	err = services.SendAIReportEmail(recipients, resumo.CompanyName, periodo, narrative, dadosBrutosJSON, taxData)
	if err != nil {
		return fmt.Errorf("error sending AI report email: %w", err)
	}

	return nil
}

// formatoPeriodoBR converts MM/YYYY to Portuguese month name YYYY
func formatoPeriodoBR(periodo string) string {
	parts := strings.Split(periodo, "/")
	if len(parts) != 2 {
		return periodo
	}
	meses := []string{"Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho",
		"Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro"}
	mesNum, _ := strconv.Atoi(parts[0])
	if mesNum < 1 || mesNum > 12 {
		return periodo
	}
	return fmt.Sprintf("%s/%s", meses[mesNum-1], parts[1])
}

// buildDadosBrutosJSON converts summary to JSON for storage
func buildDadosBrutosJSON(resumo *AIResumo) string {
	data := map[string]interface{}{
		"empresa":          resumo.CompanyName,
		"cnpj":             resumo.CNPJ,
		"periodo":          resumo.Periodo,
		"faturamento":       resumo.FaturamentoBruto,
		"total_entradas":    resumo.TotalEntradas,
		"total_saidas":      resumo.TotalSaidas,
		"icms_entrada":      resumo.IcmsEntrada,
		"icms_saida":        resumo.IcmsSaida,
		"icms_a_pagar":      resumo.IcmsAPagar,
		"ibs_projetado":               resumo.IbsProjetado,
		"cbs_projetado":               resumo.CbsProjetado,
		"ibs_cbs_total":               resumo.IbsProjetado + resumo.CbsProjetado,
		"aliquota_efetiva_icms":        resumo.AliquotaEfetivaICMS,
		"aliquota_efetiva_ibs":         resumo.AliquotaEfetivaIBS,
		"aliquota_efetiva_cbs":         resumo.AliquotaEfetivaCBS,
		"aliquota_efetiva_total_reforma": resumo.AliquotaEfetivaTotalReforma,
		"total_nfs":                    resumo.TotalNFes,
		"operacoes":                    resumo.Operacoes,
	}
	jsonBytes, _ := json.Marshal(data)
	return string(jsonBytes)
}

// getApuracaoResumoForAI aggregates fiscal data (simplified version for worker)
func getApuracaoResumoForAI(db *sql.DB, companyID, periodo string) (*AIResumo, error) {
	resumo := &AIResumo{Periodo: periodo}

	// Get company info
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
		var op AIOperacaoResumo
		if err := rows.Scan(&op.TipoOperacao, &op.Tipo, &op.Valor, &op.Icms); err != nil {
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

	// Count NFs for current period
	db.QueryRow(`
		SELECT COUNT(DISTINCT mv.filial_cnpj || mv.mes_ano || mv.tipo_operacao)
		FROM mv_mercadorias_agregada mv
		WHERE mv.company_id = $1 AND mv.mes_ano = $2
	`, companyID, periodo).Scan(&resumo.TotalNFes)

	// Calculate IBS/CBS projections using same logic as Dashboard (year 2033 = full reform)
	// Uses mv_mercadorias_agregada with NET calculation (Debit - Credit), excludes CFOP T and O
	var rates TaxRates
	rates, _ = getTaxRates(db, 2033)

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
		var ibsDebit, ibsCredit, cbsDebit, cbsCredit float64
		ibsRate := (rates.PercIBS_UF + rates.PercIBS_Mun) / 100.0
		cbsRate := rates.PercCBS / 100.0
		for ibsCbsRows.Next() {
			var tipo string
			var valTax, icmsTax float64
			if err := ibsCbsRows.Scan(&tipo, &valTax, &icmsTax); err != nil {
				continue
			}
			icmsProj := icmsTax * (1.0 - (rates.PercReducICMS / 100.0))
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

	return resumo, nil
}

// buildExecutiveSummaryPromptForAI builds prompt for AI generation
func buildExecutiveSummaryPromptForAI(resumo *AIResumo) string {
	var sb strings.Builder
	sb.WriteString("IMPORTANTE: Responda EXCLUSIVAMENTE em português brasileiro (pt-BR). NÃO escreva em inglês.\n\n")
	sb.WriteString(fmt.Sprintf("Empresa: %s (CNPJ: %s)\n", resumo.CompanyName, resumo.CNPJ))
	sb.WriteString(fmt.Sprintf("Período de apuração: %s\n\n", resumo.Periodo))
	sb.WriteString("DADOS DO PERÍODO ATUAL:\n")
	sb.WriteString(fmt.Sprintf("- Faturamento bruto (saídas): R$ %.2f\n", resumo.FaturamentoBruto))
	sb.WriteString(fmt.Sprintf("- Total de entradas: R$ %.2f\n", resumo.TotalEntradas))
	sb.WriteString(fmt.Sprintf("- ICMS sobre saídas (débito): R$ %.2f\n", resumo.IcmsSaida))
	sb.WriteString(fmt.Sprintf("- ICMS sobre entradas (crédito): R$ %.2f\n", resumo.IcmsEntrada))
	sb.WriteString(fmt.Sprintf("- ICMS a recolher (débito - crédito): R$ %.2f\n", resumo.IcmsAPagar))

	sb.WriteString("\nNOVOS IMPOSTOS - REFORMA TRIBUTÁRIA (Projeção 2033 - Implementação Completa):\n")
	sb.WriteString("NOTA: Valores calculados como SALDO A PAGAR (débito saídas - crédito entradas), mesma lógica do painel operacional.\n")
	sb.WriteString(fmt.Sprintf("- IBS projetado a pagar (Imposto sobre Bens e Serviços): R$ %.2f\n", resumo.IbsProjetado))
	sb.WriteString(fmt.Sprintf("- CBS projetado a pagar (Contribuição sobre Bens e Serviços): R$ %.2f\n", resumo.CbsProjetado))
	sb.WriteString(fmt.Sprintf("- Total IBS + CBS a pagar: R$ %.2f\n", resumo.IbsProjetado+resumo.CbsProjetado))

	if resumo.FaturamentoBruto > 0 {
		sb.WriteString("\nALÍQUOTA EFETIVA DO NEGÓCIO (sobre faturamento bruto):\n")
		sb.WriteString(fmt.Sprintf("- ICMS efetivo atual: %.2f%%\n", resumo.AliquotaEfetivaICMS))
		sb.WriteString(fmt.Sprintf("- IBS efetivo projetado (2033): %.2f%%\n", resumo.AliquotaEfetivaIBS))
		sb.WriteString(fmt.Sprintf("- CBS efetivo projetado (2033): %.2f%%\n", resumo.AliquotaEfetivaCBS))
		sb.WriteString(fmt.Sprintf("- Total IBS+CBS efetivo (2033): %.2f%%\n", resumo.AliquotaEfetivaTotalReforma))
	}

	if len(resumo.Operacoes) > 0 {
		sb.WriteString("\nDETALHAMENTO POR TIPO DE OPERAÇÃO:\n")
		for _, op := range resumo.Operacoes {
			sb.WriteString(fmt.Sprintf("- %s (%s): Valor R$ %.2f | ICMS R$ %.2f\n", op.TipoOperacao, op.Tipo, op.Valor, op.Icms))
		}
	}

	sb.WriteString(fmt.Sprintf("\nINFORMAÇÕES ADICIONAIS:\n"))
	sb.WriteString(fmt.Sprintf("- Total de notas fiscais processadas: %d\n", resumo.TotalNFes))

	return sb.String()
}

// executiveSummarySystem is AI system prompt for executive summaries
const executiveSummarySystem = `Você é um assistente fiscal especializado em tributação brasileira e na Reforma Tributária.

INSTRUÇÃO CRÍTICA: Responda DIRETAMENTE com o relatório em Markdown. NÃO inclua raciocínio, análise prévia, passos numerados ou pensamento interno. Comece sua resposta IMEDIATAMENTE com "## Resumo Executivo".

Gere um RESUMO EXECUTIVO MENSAL de apuração fiscal para ser lido por um CEO ou Controller.

REGRAS:
- Escreva EXCLUSIVAMENTE em português brasileiro (pt-BR). NUNCA use inglês.
- Tom profissional e direto
- Valores monetários: R$ XX.XXX,00 (milhar com ponto, decimal com vírgula)
- Formatação Markdown (## headers, **negrito**, listas)
- Inclua obrigatoriamente:
  1. Situação geral (1-2 frases)
  2. Impostos a recolher
  3. Tabela comparativa em Markdown com alíquota efetiva:
     | Imposto | Valor | Alíquota Efetiva | Observação |
     |---------|-------|------------------|------------|
     | ICMS a Recolher | R$ X | X,XX% | Regime atual |
     | IBS Projetado a Pagar | R$ X | X,XX% | Novo imposto - projeção 2033 |
     | CBS Projetado a Pagar | R$ X | X,XX% | Novo imposto - projeção 2033 |
     | Total IBS + CBS a Pagar | R$ X | X,XX% | Substituirá ICMS + PIS/COFINS |
  4. Comentário sobre alíquota efetiva: compare ICMS efetivo atual vs total IBS+CBS efetivo, explique o impacto prático para o negócio
  5. Comparativo com período anterior (se disponível) com variação percentual e variação em pontos percentuais da alíquota efetiva
  6. Destaques relevantes
  7. Recomendações práticas (2-3 itens)
- NÃO invente dados. Use APENAS os números fornecidos.
- Mantenha entre 300-500 palavras.
- Comece DIRETO com "## Resumo Executivo" sem nenhum texto antes.`

// buildFallbackNarrative generates a basic report when AI is unavailable
func buildFallbackNarrative(r *AIResumo) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Resumo Executivo - %s | %s\n\n", r.CompanyName, r.Periodo))
	sb.WriteString(fmt.Sprintf("**Faturamento Bruto (Saidas):** R$ %.2f\n\n", r.FaturamentoBruto))
	sb.WriteString(fmt.Sprintf("**Total de Entradas:** R$ %.2f\n\n", r.TotalEntradas))
	sb.WriteString("### ICMS do Periodo\n\n")
	sb.WriteString(fmt.Sprintf("- **ICMS Debito (Saidas):** R$ %.2f\n", r.IcmsSaida))
	sb.WriteString(fmt.Sprintf("- **ICMS Credito (Entradas):** R$ %.2f\n", r.IcmsEntrada))
	sb.WriteString(fmt.Sprintf("- **ICMS a Recolher:** R$ %.2f\n\n", r.IcmsAPagar))

	sb.WriteString("### Novos Impostos - Reforma Tributaria (Projecao 2033)\n\n")
	sb.WriteString("| Imposto | Valor | Aliquota Efetiva |\n")
	sb.WriteString("|---------|-------|------------------|\n")
	sb.WriteString(fmt.Sprintf("| ICMS a Recolher | R$ %.2f | %.2f%% |\n", r.IcmsAPagar, r.AliquotaEfetivaICMS))
	sb.WriteString(fmt.Sprintf("| IBS Projetado | R$ %.2f | %.2f%% |\n", r.IbsProjetado, r.AliquotaEfetivaIBS))
	sb.WriteString(fmt.Sprintf("| CBS Projetado | R$ %.2f | %.2f%% |\n", r.CbsProjetado, r.AliquotaEfetivaCBS))
	sb.WriteString(fmt.Sprintf("| **Total IBS + CBS** | **R$ %.2f** | **%.2f%%** |\n\n", r.IbsProjetado+r.CbsProjetado, r.AliquotaEfetivaTotalReforma))

	if len(r.Operacoes) > 0 {
		sb.WriteString("### Detalhamento por Operacao\n\n")
		for _, op := range r.Operacoes {
			sb.WriteString(fmt.Sprintf("- %s (%s): R$ %.2f | ICMS: R$ %.2f\n", op.TipoOperacao, op.Tipo, op.Valor, op.Icms))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("*Total de registros processados: %d*\n\n", r.TotalNFes))
	sb.WriteString("*Relatorio gerado automaticamente pelo FBTax Cloud. Narrativa com IA sera incluida quando disponivel.*")
	return sb.String()
}

// getActiveManagersForAIReport returns active managers for AI report emailing
func getActiveManagersForAIReport(db *sql.DB, companyID string) ([]AIManager, error) {
	rows, err := db.Query(`
		SELECT id, company_id, nome_completo, cargo, email, ativo, created_at, updated_at
		FROM managers
		WHERE company_id = $1 AND ativo = true
		ORDER BY nome_completo ASC
	`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var managers []AIManager
	for rows.Next() {
		var m AIManager
		if err := rows.Scan(&m.ID, &m.CompanyID, &m.NomeCompleto, &m.Cargo, &m.Email, &m.Ativo, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		managers = append(managers, m)
	}

	return managers, nil
}
