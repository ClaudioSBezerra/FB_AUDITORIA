-- 011: papel de uma linha do DRE - história 3.4.
--
-- papel identifica a linha que serve de base para os percentuais do DRE (hoje só
-- 'faturamento_liquido'). É configuração da empresa, nunca um código fixo no
-- programa: no máximo uma linha por empresa tem cada papel (índice parcial).
-- dre_fechamento_linhas congela o papel junto com a estrutura do fechamento;
-- fechamentos anteriores ficam com papel NULL (não é reescrita de snapshot).

ALTER TABLE auditoria.dre_linhas ADD COLUMN IF NOT EXISTS papel TEXT;
ALTER TABLE auditoria.dre_linhas DROP CONSTRAINT IF EXISTS dre_linhas_papel_check;
ALTER TABLE auditoria.dre_linhas ADD CONSTRAINT dre_linhas_papel_check CHECK (papel IS NULL OR papel IN ('faturamento_liquido'));
CREATE UNIQUE INDEX IF NOT EXISTS uq_dre_linhas_papel ON auditoria.dre_linhas (empresa_id, papel) WHERE papel IS NOT NULL;

ALTER TABLE auditoria.dre_fechamento_linhas ADD COLUMN IF NOT EXISTS papel TEXT;
