-- Migration: Update Materialized View for Mercadorias (Remove PIS/COFINS references)
-- Up
DROP MATERIALIZED VIEW IF EXISTS mv_mercadorias_agregada;

CREATE MATERIALIZED VIEW mv_mercadorias_agregada AS
SELECT 
    j.company_name as filial_nome,
    TO_CHAR(c.dt_doc, 'MM/YYYY') as mes_ano,
    EXTRACT(YEAR FROM c.dt_doc)::INTEGER as ano,
    CASE WHEN c.ind_oper = '0' THEN 'ENTRADA' ELSE 'SAIDA' END as tipo,
    COALESCE(f.tipo, 'O') as tipo_cfop,
    -- Dados Originais (Base Histórica SPED)
    SUM(c190.vl_opr) as valor_contabil,
    SUM(c190.vl_icms) as vl_icms_origem
FROM reg_c190 c190
JOIN reg_c100 c ON c.id = c190.id_pai_c100
JOIN import_jobs j ON j.id = c.job_id
LEFT JOIN cfop f ON c190.cfop = f.cfop
GROUP BY 1, 2, 3, 4, 5
WITH NO DATA;

-- Create Unique Index to allow CONCURRENT REFRESH
CREATE UNIQUE INDEX idx_mv_mercadorias_agregada 
ON mv_mercadorias_agregada (filial_nome, mes_ano, ano, tipo, tipo_cfop);

-- Down
DROP MATERIALIZED VIEW IF EXISTS mv_mercadorias_agregada;
-- (Recreate old view if needed, but for now just drop)
