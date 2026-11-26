-- 021: detalhe por lançamento vindo do banco do cliente (história 2.24).
ALTER TABLE auditoria.lancamentos ADD COLUMN IF NOT EXISTS data_lancamento DATE;
ALTER TABLE auditoria.lancamentos ADD COLUMN IF NOT EXISTS data_referencia DATE;       -- a data que definiu a competência
ALTER TABLE auditoria.lancamentos ADD COLUMN IF NOT EXISTS parceiro_tipo TEXT NOT NULL DEFAULT '';
ALTER TABLE auditoria.lancamentos ADD COLUMN IF NOT EXISTS parceiro_cod  TEXT NOT NULL DEFAULT '';
ALTER TABLE auditoria.lancamentos ADD COLUMN IF NOT EXISTS parceiro_nome TEXT NOT NULL DEFAULT '';
ALTER TABLE auditoria.lancamentos ADD COLUMN IF NOT EXISTS erp_id        TEXT NOT NULL DEFAULT '';
