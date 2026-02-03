-- Migration: Create Materialized Views for Reports (Optimized for Tax Reform 2027+)
-- Up
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_mercadorias_agregada AS
SELECT 
    j.company_name as filial_nome,
    TO_CHAR(c.dt_doc, 'MM/YYYY') as mes_ano,
    EXTRACT(YEAR FROM c.dt_doc)::INTEGER as ano,
    CASE WHEN c.ind_oper = '0' THEN 'ENTRADA' ELSE 'SAIDA' END as tipo,
    COALESCE(f.tipo, 'O') as tipo_cfop,
    -- Dados Originais (Base Histórica SPED)
    SUM(c190.vl_opr) as valor_contabil,
    SUM(c190.vl_icms) as vl_icms_origem,
    -- Mantemos PIS/COFINS apenas como referência histórica (Antes da Reforma)
    SUM(COALESCE(c.vl_pis, 0) * CASE WHEN c.vl_doc > 0 THEN c190.vl_opr / c.vl_doc ELSE 0 END) as vl_pis_origem,
    SUM(COALESCE(c.vl_cofins, 0) * CASE WHEN c.vl_doc > 0 THEN c190.vl_opr / c.vl_doc ELSE 0 END) as vl_cofins_origem
FROM reg_c190 c190
JOIN reg_c100 c ON c.id = c190.id_pai_c100
JOIN import_jobs j ON j.id = c.job_id
LEFT JOIN cfop f ON c190.cfop = f.cfop
GROUP BY 1, 2, 3, 4, 5;

-- Create Unique Index to allow CONCURRENT REFRESH
CREATE UNIQUE INDEX IF NOT EXISTS idx_mv_mercadorias_agregada 
ON mv_mercadorias_agregada (filial_nome, mes_ano, ano, tipo, tipo_cfop);

-- Down
-- DROP MATERIALIZED VIEW IF EXISTS mv_mercadorias_agregada;
