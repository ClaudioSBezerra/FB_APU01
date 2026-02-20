package services

import (
	"fmt"
	"regexp"
	"strings"
)

// SystemPromptTextToSQL is the system message sent to the AI for SQL generation.
const SystemPromptTextToSQL = `Você é um especialista em SQL PostgreSQL e tributação brasileira (Reforma Tributária - IBS/CBS/ICMS).
Sua única tarefa é gerar uma query SQL para responder à pergunta do usuário.

REGRAS OBRIGATÓRIAS:
1. Responda SOMENTE com o bloco SQL dentro de ` + "```sql\n...\n```" + `. Zero texto fora do bloco.
2. Sempre filtre por company_id = '__COMPANY_ID__' em todas as tabelas/views que possuem essa coluna.
3. Use APENAS SELECT. Jamais use INSERT, UPDATE, DELETE, DROP, ALTER, CREATE, TRUNCATE.
4. Inclua LIMIT 100 no final, a menos que a query já tenha LIMIT.
5. Use aliases em português nos resultados (ex: AS fornecedor, AS valor_total, AS periodo).
6. Períodos estão no formato 'MM/YYYY' (ex: '05/2024').
7. "Prejuízo do Simples Nacional" = vl_icms_origem da mv_operacoes_simples (crédito de ICMS que o comprador perde).
8. Faturamento/vendas = tipo = 'SAIDA' na mv_mercadorias_agregada.
9. Compras/entradas = tipo = 'ENTRADA' na mv_mercadorias_agregada.
10. Sempre ordene por valor DESC quando relevante.`

// dbSchemaContext describes the key tables/views the AI can query.
const dbSchemaContext = `
-- Schema PostgreSQL do FBTax Cloud (multi-empresa, sempre filtrar por company_id)

-- View principal: todas as operações fiscais (mercadorias, frete, energia, comunicações)
CREATE MATERIALIZED VIEW mv_mercadorias_agregada (
    company_id UUID,
    filial_nome VARCHAR,     -- Nome da empresa/filial
    filial_cnpj VARCHAR,     -- CNPJ da filial
    mes_ano VARCHAR,         -- Período 'MM/YYYY' ex: '05/2024'
    ano INTEGER,
    tipo VARCHAR,            -- 'ENTRADA' (compras) ou 'SAIDA' (vendas/faturamento)
    tipo_cfop CHAR(1),       -- 'T'=Transferência,'O'=Operacional,'R'=Revenda,'C'=Consumo,'A'=Ativo,'D'=Devolução
    valor_contabil DECIMAL,  -- Valor total das operações
    vl_icms_origem DECIMAL   -- ICMS nas notas fiscais
);

-- View de compras com fornecedores do Simples Nacional (sem transferência de crédito)
CREATE MATERIALIZED VIEW mv_operacoes_simples (
    company_id UUID,
    fornecedor_nome VARCHAR,  -- Nome do fornecedor Simples Nacional
    fornecedor_cnpj VARCHAR,
    mes_ano VARCHAR,          -- 'MM/YYYY'
    ano INTEGER,
    origem VARCHAR,           -- 'C100'=Mercadorias, 'D100'=Frete, 'C500'=Energia
    valor_contabil DECIMAL,   -- Valor da compra
    vl_icms_origem DECIMAL    -- Crédito ICMS perdido (= prejuízo Simples Nacional)
);

-- Operações detalhadas por parceiro comercial
CREATE TABLE operacoes_comerciais (
    job_id UUID,
    filial_cnpj VARCHAR,
    cod_part VARCHAR,
    mes_ano VARCHAR,
    ind_oper CHAR(1),          -- '0'=Entrada, '1'=Saída
    vl_doc DECIMAL,
    vl_icms DECIMAL,
    vl_ibs_projetado DECIMAL,  -- IBS projetado (Reforma Tributária)
    vl_cbs_projetado DECIMAL   -- CBS projetado (Reforma Tributária)
);

-- Parceiros/fornecedores (nome e CNPJ)
CREATE TABLE participants (
    job_id UUID,
    cod_part VARCHAR,
    nome VARCHAR,
    cnpj VARCHAR
);

-- Importações de SPED realizadas
CREATE TABLE import_jobs (
    id UUID PRIMARY KEY,
    company_id UUID,
    company_name VARCHAR,
    cnpj VARCHAR,
    periodo VARCHAR,  -- 'MM/YYYY'
    status VARCHAR    -- 'completed','processing','failed'
);

-- Alíquotas da Reforma Tributária por ano
CREATE TABLE tabela_aliquotas (
    ano INTEGER,
    aliquota_ibs DECIMAL,
    aliquota_cbs DECIMAL
);`

var (
	reSQLBlock  = regexp.MustCompile("(?is)```(?:sql)?\\s*([\\s\\S]+?)```")
	reDangerous = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE|TRUNCATE|GRANT|REVOKE)\b`)
	reSelect    = regexp.MustCompile(`(?i)^\s*(SELECT|WITH)\b`)
)

// BuildTextToSQLPrompt builds the full user prompt for the AI.
func BuildTextToSQLPrompt(pergunta string) string {
	return fmt.Sprintf("%s\n\nPergunta: %s", dbSchemaContext, pergunta)
}

// ExtractSQL extracts and validates SQL from an AI response.
// Returns the SQL string or an error if it's invalid/unsafe.
func ExtractSQL(aiResponse string) (string, error) {
	sql := aiResponse

	// Try to extract from a markdown ```sql ... ``` block
	if matches := reSQLBlock.FindStringSubmatch(aiResponse); len(matches) > 1 {
		sql = strings.TrimSpace(matches[1])
	}

	sql = strings.TrimSpace(sql)
	if sql == "" {
		return "", fmt.Errorf("nenhum SQL encontrado na resposta da IA")
	}

	// Must be a SELECT or WITH query
	if !reSelect.MatchString(sql) {
		return "", fmt.Errorf("apenas queries SELECT são permitidas")
	}

	// Block dangerous operations
	if reDangerous.MatchString(sql) {
		return "", fmt.Errorf("query contém operações não permitidas")
	}

	return sql, nil
}
