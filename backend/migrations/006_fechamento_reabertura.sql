-- 006: um fechamento ativo por (empresa, competência) e reabertura com motivo (FR-9, FR-10, AD-9).
--
-- ATENÇÃO: irreversível depois de aplicada em produção; não há migration de volta.
--
-- 1. Colunas de reabertura (aditivas).
-- 2. Pré-checagem: se alguma (empresa, competência) tiver mais de um fechamento
--    sem reaberto_em (inclui fechamentos por filial):
--      - empresa em 'producao': aborta com mensagem clara, sem alterar dado;
--      - empresa em 'teste': consolida (decisão do dono do produto): marca como
--        reabertos todos os fechamentos por filial e todas as versões consolidadas
--        exceto a de maior versão, que fica ativa (sem consolidada, fica ativa a de
--        maior versão de qualquer filial). Nunca apaga linha.
--    A mensagem de aborto não usa as palavras que o runner trata como
--    "migration já aplicada" (ele seguiria em frente sem o índice).
-- 3. Gatilho: reaberto_* mudam uma única vez; DELETE bloqueado; ambos liberados
--    só quando auditoria.imutavel_salvo_em_teste(empresa_id) (migration 004).
-- 4. Índice único parcial: um só ativo por (empresa, competência).

ALTER TABLE auditoria.dre_fechamentos ADD COLUMN IF NOT EXISTS reaberto_em TIMESTAMPTZ;
ALTER TABLE auditoria.dre_fechamentos ADD COLUMN IF NOT EXISTS reaberto_por UUID REFERENCES users(id);
ALTER TABLE auditoria.dre_fechamentos ADD COLUMN IF NOT EXISTS motivo_reabertura TEXT;

DO $$
DECLARE
    r RECORD;
BEGIN
    -- Produção primeiro: se houver qualquer conflito, nada é alterado.
    FOR r IN
        SELECT f.empresa_id, f.competencia, count(*) AS n
          FROM auditoria.dre_fechamentos f
          JOIN companies c ON c.id = f.empresa_id
         WHERE f.reaberto_em IS NULL AND c.ambiente = 'producao'
         GROUP BY f.empresa_id, f.competencia
        HAVING count(*) > 1
         ORDER BY f.empresa_id, f.competencia
         LIMIT 1
    LOOP
        RAISE EXCEPTION 'migracao 006 abortada: a empresa % (ambiente producao) tem % fechamentos ativos na competencia %; deixe um so ativo por empresa e competencia antes de aplicar. Nenhum dado foi alterado.',
            r.empresa_id, r.n, to_char(r.competencia, 'YYYY-MM');
    END LOOP;

    -- Teste: consolida. Mantém ativa a consolidada (filial vazia) de maior versão.
    UPDATE auditoria.dre_fechamentos f
       SET reaberto_em = now(),
           motivo_reabertura = 'consolidado na migração, ambiente de teste'
      FROM companies c
     WHERE c.id = f.empresa_id
       AND c.ambiente = 'teste'
       AND f.reaberto_em IS NULL
       AND (f.empresa_id, f.competencia) IN (
            SELECT empresa_id, competencia
              FROM auditoria.dre_fechamentos
             WHERE reaberto_em IS NULL
             GROUP BY empresa_id, competencia
            HAVING count(*) > 1)
       -- fica ativa a consolidada de maior versão; sem consolidada, a de maior versão
       -- de qualquer filial (reserva, para não destravar o mês)
       AND f.id <> (
            SELECT g.id FROM auditoria.dre_fechamentos g
             WHERE g.empresa_id = f.empresa_id AND g.competencia = f.competencia
               AND g.reaberto_em IS NULL
             ORDER BY (COALESCE(g.filial,'') = '') DESC, g.versao DESC
             LIMIT 1);
END $$;

CREATE OR REPLACE FUNCTION auditoria.dre_fechamentos_imutavel() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF auditoria.imutavel_salvo_em_teste(OLD.empresa_id) THEN
            RETURN OLD;
        END IF;
        RAISE EXCEPTION 'fechamento nao pode ser apagado';
    END IF;

    IF auditoria.imutavel_salvo_em_teste(OLD.empresa_id) THEN
        RETURN NEW;
    END IF;
    IF OLD.reaberto_em IS NOT NULL THEN
        RAISE EXCEPTION 'fechamento ja reaberto; nao muda de novo';
    END IF;
    IF NEW.reaberto_em IS NULL OR OLD.reaberto_por IS NOT NULL OR OLD.motivo_reabertura IS NOT NULL
       OR NEW.reaberto_por IS NULL OR NEW.motivo_reabertura IS NULL OR btrim(NEW.motivo_reabertura) = '' THEN
        RAISE EXCEPTION 'fechamento so muda na reabertura, com motivo';
    END IF;
    IF (OLD.id, OLD.empresa_id, OLD.competencia, OLD.versao, OLD.filial, OLD.fechado_por, OLD.fechado_em, OLD.observacao)
       IS DISTINCT FROM
       (NEW.id, NEW.empresa_id, NEW.competencia, NEW.versao, NEW.filial, NEW.fechado_por, NEW.fechado_em, NEW.observacao) THEN
        RAISE EXCEPTION 'fechamento so muda na reabertura; demais colunas sao imutaveis';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_dre_fechamentos_imutavel ON auditoria.dre_fechamentos;
CREATE TRIGGER trg_dre_fechamentos_imutavel
    BEFORE UPDATE OR DELETE ON auditoria.dre_fechamentos
    FOR EACH ROW EXECUTE FUNCTION auditoria.dre_fechamentos_imutavel();

CREATE UNIQUE INDEX IF NOT EXISTS ux_dre_fechamentos_ativo
    ON auditoria.dre_fechamentos (empresa_id, competencia)
    WHERE reaberto_em IS NULL;
