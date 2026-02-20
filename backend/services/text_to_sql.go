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
2. mv_mercadorias_agregada e mv_operacoes_simples têm company_id — filtre diretamente: WHERE company_id = '__COMPANY_ID__'.
3. operacoes_comerciais e participants NÃO têm company_id. Sempre JOIN com import_jobs: JOIN import_jobs j ON j.id = oc.job_id WHERE j.company_id = '__COMPANY_ID__'.
4. participants requer JOIN duplo: JOIN participants p ON p.job_id = oc.job_id AND p.cod_part = oc.cod_part.
5. Use APENAS SELECT. Jamais use INSERT, UPDATE, DELETE, DROP, ALTER, CREATE, TRUNCATE.
6. Inclua LIMIT 100 no final.
7. Use aliases em português (ex: AS fornecedor, AS valor_total, AS periodo).
8. mes_ano está no formato 'MM/YYYY' — contém datas reais dos dados importados, não anos futuros.
9. vl_ibs_projetado e vl_cbs_projetado em operacoes_comerciais são projeções calculadas sobre os dados reais.
10. "Prejuízo do Simples Nacional" = vl_icms_origem da mv_operacoes_simples.
11. Faturamento/vendas = tipo = 'SAIDA'. Compras = tipo = 'ENTRADA' (mv_mercadorias_agregada).
12. Ordene por valor DESC quando relevante.`

// dbSchemaContext describes the key tables/views the AI can query.
const dbSchemaContext = `
-- Schema PostgreSQL do FBTax Cloud (multi-empresa)
-- IMPORTANTE: operacoes_comerciais e participants NÃO têm company_id diretamente.
-- Para filtrar por empresa, sempre JOIN com import_jobs usando job_id.

-- View principal agregada: operações fiscais por filial/período (TEM company_id direto)
CREATE MATERIALIZED VIEW mv_mercadorias_agregada (
    company_id UUID,         -- filtrar aqui diretamente
    filial_nome VARCHAR,
    filial_cnpj VARCHAR,
    mes_ano VARCHAR,         -- 'MM/YYYY' ex: '05/2024' (dados reais importados)
    ano INTEGER,
    tipo VARCHAR,            -- 'ENTRADA' (compras) ou 'SAIDA' (vendas)
    tipo_cfop CHAR(1),       -- 'T'=Transferência,'O'=Operacional,'R'=Revenda,'C'=Consumo,'A'=Ativo,'D'=Devolução
    valor_contabil DECIMAL,
    vl_icms_origem DECIMAL
);

-- View de compras com fornecedores do Simples Nacional (TEM company_id direto)
CREATE MATERIALIZED VIEW mv_operacoes_simples (
    company_id UUID,         -- filtrar aqui diretamente
    fornecedor_nome VARCHAR,
    fornecedor_cnpj VARCHAR,
    mes_ano VARCHAR,         -- 'MM/YYYY'
    ano INTEGER,
    origem VARCHAR,          -- 'C100', 'D100', 'C500'
    valor_contabil DECIMAL,
    vl_icms_origem DECIMAL   -- crédito ICMS perdido = prejuízo do Simples
);

-- Operações por parceiro (NÃO tem company_id — filtrar via import_jobs)
-- vl_ibs_projetado e vl_cbs_projetado são valores projetados da Reforma Tributária
-- calculados sobre os dados reais importados (mes_ano refere-se ao período real, ex: '01/2024')
CREATE TABLE operacoes_comerciais (
    job_id UUID,             -- JOIN com import_jobs para obter company_id
    filial_cnpj VARCHAR,
    cod_part VARCHAR,        -- código do parceiro
    mes_ano VARCHAR,         -- período real 'MM/YYYY' dos dados importados
    ind_oper CHAR(1),        -- '0'=Entrada, '1'=Saída
    vl_doc DECIMAL,
    vl_icms DECIMAL,
    vl_ibs_projetado DECIMAL,
    vl_cbs_projetado DECIMAL
);

-- Parceiros/fornecedores (NÃO tem company_id — filtrar via import_jobs)
-- JOIN obrigatório em DOIS campos: job_id AND cod_part
CREATE TABLE participants (
    job_id UUID,             -- JOIN com import_jobs para obter company_id
    cod_part VARCHAR,
    nome VARCHAR,
    cnpj VARCHAR
);

-- Importações de SPED (bridge para company_id)
CREATE TABLE import_jobs (
    id UUID PRIMARY KEY,
    company_id UUID,         -- chave de empresa
    company_name VARCHAR,
    cnpj VARCHAR,
    periodo VARCHAR,         -- 'MM/YYYY'
    status VARCHAR
);

-- Alíquotas da Reforma Tributária
CREATE TABLE tabela_aliquotas (
    ano INTEGER,
    aliquota_ibs DECIMAL,
    aliquota_cbs DECIMAL
);

-- EXEMPLOS DE JOIN CORRETO:
-- Para consultar operacoes_comerciais com filtro de empresa:
--   FROM operacoes_comerciais oc
--   JOIN import_jobs j ON j.id = oc.job_id
--   JOIN participants p ON p.job_id = oc.job_id AND p.cod_part = oc.cod_part
--   WHERE j.company_id = '__COMPANY_ID__'
--
-- Para consultar mv_mercadorias_agregada:
--   FROM mv_mercadorias_agregada WHERE company_id = '__COMPANY_ID__'`

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
