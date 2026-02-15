package worker

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
	TotalNFes        int       `json:"total_nfs"`
	Operacoes        []AIOperacaoResumo `json:"operacoes"`
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

	//3. Generate AI narrative
	aiClient := services.NewAIClient()
	if aiClient == nil || !aiClient.IsAvailable() {
		return fmt.Errorf("AI client not available (ZAI_API_KEY not set)")
	}

	dataPrompt := buildExecutiveSummaryPromptForAI(resumo)
	aiResp, err := aiClient.Generate(executiveSummarySystem, dataPrompt, services.ModelFlash, 2048)
	if err != nil {
		return fmt.Errorf("AI generation failed: %w", err)
	}

	//4. Save report to database
	titulo := fmt.Sprintf("%s | %s", resumo.CompanyName, formatoPeriodoBR(periodo))
	dadosBrutosJSON := buildDadosBrutosJSON(resumo)

	var reportID string
	err = db.QueryRow(`
		INSERT INTO ai_reports (company_id, job_id, periodo, titulo, resumo, dados_brutos, gerado_automaticamente)
		VALUES ($1, $2, $3, $4, $5, $6, true)
		RETURNING id
	`, companyID, jobID, periodo, titulo, aiResp.Text, dadosBrutosJSON).Scan(&reportID)
	if err != nil {
		return fmt.Errorf("error saving AI report: %w", err)
	}
	fmt.Printf("[AI Report] Report saved to database: %s\n", reportID)

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

	//7. Send email
	err = services.SendAIReportEmail(recipients, resumo.CompanyName, periodo, aiResp.Text, dadosBrutosJSON)
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
		"total_nfs":         resumo.TotalNFes,
		"operacoes":         resumo.Operacoes,
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

	return resumo, nil
}

// buildExecutiveSummaryPromptForAI builds prompt for AI generation
func buildExecutiveSummaryPromptForAI(resumo *AIResumo) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Empresa: %s (CNPJ: %s)\n", resumo.CompanyName, resumo.CNPJ))
	sb.WriteString(fmt.Sprintf("Período de apuração: %s\n\n", resumo.Periodo))
	sb.WriteString("DADOS DO PERÍODO ATUAL:\n")
	sb.WriteString(fmt.Sprintf("- Faturamento bruto (saídas): R$ %.2f\n", resumo.FaturamentoBruto))
	sb.WriteString(fmt.Sprintf("- Total de entradas: R$ %.2f\n", resumo.TotalEntradas))
	sb.WriteString(fmt.Sprintf("- ICMS sobre saídas (débito): R$ %.2f\n", resumo.IcmsSaida))
	sb.WriteString(fmt.Sprintf("- ICMS sobre entradas (crédito): R$ %.2f\n", resumo.IcmsEntrada))
	sb.WriteString(fmt.Sprintf("- ICMS a recolher (débito - crédito): R$ %.2f\n", resumo.IcmsAPagar))

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
const executiveSummarySystem = `Você é um assistente fiscal especializado em tributação brasileira.
Gere um RESUMO EXECUTIVO MENSAL de apuração fiscal para ser lido por um CEO ou Controller que não é especialista em tributos.

REGRAS:
- Escreva em português brasileiro (pt-BR)
- Use tom profissional e direto, sem jargão excessivo
- Formate valores monetários como R$ XX.XXX,00 (separador de milhar com ponto, decimal com vírgula)
- Use Markdown para formatação (headers ##, **negrito**, listas)
- Inclua obrigatoriamente:
  1. Situação geral (1-2 frases de abertura)
  2. Impostos a recolher com valores
  3. Destaques: o que subiu, o que caiu e por que
  4. Recomendações práticas (2-3 itens)
- NÃO invente dados. Use APENAS os números fornecidos.
- Se não houver dados suficientes, diga "Dados insuficientes para esta análise".
- Mantenha o relatório entre 200-400 palavras.`

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
