-- Aumenta limite da coluna descricao_cfop de VARCHAR(100) para VARCHAR(255)
-- Necessário para acomodar descrições longas no seed de CFOPs
ALTER TABLE cfop ALTER COLUMN descricao_cfop TYPE VARCHAR(255);
