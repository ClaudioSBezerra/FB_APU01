-- Migration: Remove PIS/COFINS from Materialized View (Tax Reform 2027+)
-- Optimized to run only if necessary (Idempotent)

DO $$
DECLARE
    view_exists BOOLEAN;
    column_exists BOOLEAN;
BEGIN
    -- Check if view exists
    SELECT EXISTS (
        SELECT 1 FROM pg_matviews WHERE matviewname = 'mv_mercadorias_agregada'
    ) INTO view_exists;

    -- Check if old column exists (indicating old version)
    SELECT EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'mv_mercadorias_agregada' 
        AND column_name = 'vl_pis_origem'
    ) INTO column_exists;

    -- If view exists AND has old column, drop it
    IF view_exists AND column_exists THEN
        RAISE NOTICE 'Dropping old version of mv_mercadorias_agregada...';
        DROP MATERIALIZED VIEW mv_mercadorias_agregada;
    END IF;
END $$;

-- Create if not exists (either it was dropped above, or never existed)
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_mercadorias_agregada AS
SELECT 
    j.company_name as filial_nome,
    TO_CHAR(c.dt_doc, 'MM/YYYY') as mes_ano,
    EXTRACT(YEAR FROM c.dt_doc)::INTEGER as ano,
    CASE WHEN c.ind_oper = '0' THEN 'ENTRADA' ELSE 'SAIDA' END as tipo,
    COALESCE(f.tipo, 'O') as tipo_cfop,
    SUM(c190.vl_opr) as valor_contabil,
    SUM(c190.vl_icms) as vl_icms_origem
FROM reg_c190 c190
JOIN reg_c100 c ON c.id = c190.id_pai_c100
JOIN import_jobs j ON j.id = c.job_id
LEFT JOIN cfop f ON c190.cfop = f.cfop
GROUP BY 1, 2, 3, 4, 5;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mv_mercadorias_agregada 
ON mv_mercadorias_agregada (filial_nome, mes_ano, ano, tipo, tipo_cfop);
