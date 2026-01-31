INSERT INTO tabela_aliquotas (ano, perc_ibs_uf, perc_ibs_mun, perc_cbs, perc_reduc_icms, perc_reduc_piscofins)
VALUES 
(2027, 0.05, 0.05, 8.80, 0.00, 100.00),
(2028, 0.05, 0.05, 8.80, 0.00, 100.00)
ON CONFLICT (ano) DO UPDATE SET
  perc_ibs_uf = EXCLUDED.perc_ibs_uf,
  perc_ibs_mun = EXCLUDED.perc_ibs_mun,
  perc_cbs = EXCLUDED.perc_cbs,
  perc_reduc_icms = EXCLUDED.perc_reduc_icms,
  perc_reduc_piscofins = EXCLUDED.perc_reduc_piscofins;