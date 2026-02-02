-- Migration 014: Make CNPJ optional in companies table
-- Reason: User requested that CNPJ is not mandatory, only company name.

ALTER TABLE companies ALTER COLUMN cnpj DROP NOT NULL;
ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_cnpj_key;
