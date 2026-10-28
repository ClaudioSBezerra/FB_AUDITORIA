-- 007: snapshot completo do fechamento (AD-17, FR-11).
--
-- dre_fechamento_linhas ganha filial (na chave), orçado e ano anterior;
-- dre_fechamento_contas congela o realizado e o ano anterior por conta.
-- Ambas imutáveis por gatilho (liberado só por auditoria.imutavel_salvo_em_teste).
-- Fechamentos anteriores a esta migration não são alterados: ficam com
-- orcado/ano_anterior NULL (é o que os identifica como fechamento só de linhas)
-- e sem contas.
-- Orçado por conta, departamento e ajustes chegam nas histórias 2.7, 3.5 e 5.4.
-- ATENÇÃO: irreversível depois de aplicada em produção; não há migration de volta.

ALTER TABLE auditoria.dre_fechamento_linhas ADD COLUMN IF NOT EXISTS filial TEXT NOT NULL DEFAULT '';
ALTER TABLE auditoria.dre_fechamento_linhas ADD COLUMN IF NOT EXISTS orcado NUMERIC(18,2);
ALTER TABLE auditoria.dre_fechamento_linhas ADD COLUMN IF NOT EXISTS ano_anterior NUMERIC(18,2);
ALTER TABLE auditoria.dre_fechamento_linhas ADD COLUMN IF NOT EXISTS tipo TEXT;  -- 'conta' | 'subtotal' (estrutura congelada)

-- Chave com filial (só troca a chave; nenhuma linha gravada é alterada).
DO $$
DECLARE
    v_cols int;
BEGIN
    SELECT array_length(i.indkey::int2[], 1) INTO v_cols
      FROM pg_index i
     WHERE i.indrelid = 'auditoria.dre_fechamento_linhas'::regclass AND i.indisprimary;
    IF v_cols IS NOT NULL AND v_cols = 2 THEN
        ALTER TABLE auditoria.dre_fechamento_linhas DROP CONSTRAINT dre_fechamento_linhas_pkey;
        ALTER TABLE auditoria.dre_fechamento_linhas ADD PRIMARY KEY (fechamento_id, filial, linha_codigo);
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS auditoria.dre_fechamento_contas (
    fechamento_id BIGINT NOT NULL REFERENCES auditoria.dre_fechamentos(id) ON DELETE CASCADE,
    empresa_id    UUID   NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    filial        TEXT   NOT NULL DEFAULT '',   -- '' = consolidado do grupo
    conta         TEXT   NOT NULL,
    linha_codigo  TEXT   NOT NULL,
    realizado     NUMERIC(18,2) NOT NULL,       -- já normalizado pelo sinal do mapa
    ano_anterior  NUMERIC(18,2) NOT NULL,
    PRIMARY KEY (fechamento_id, filial, conta)
);

CREATE OR REPLACE FUNCTION auditoria.dre_fechamento_linhas_imutavel() RETURNS trigger AS $$
DECLARE
    v_emp UUID;
BEGIN
    SELECT empresa_id INTO v_emp FROM auditoria.dre_fechamentos WHERE id = OLD.fechamento_id;
    IF v_emp IS NULL THEN
        -- cascata de um fechamento que o seu próprio gatilho já autorizou apagar
        IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
        RAISE EXCEPTION 'linha de fechamento e imutavel';
    END IF;
    IF auditoria.imutavel_salvo_em_teste(v_emp) THEN
        IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'linha de fechamento e imutavel';
END $$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION auditoria.dre_fechamento_contas_imutavel() RETURNS trigger AS $$
BEGIN
    IF auditoria.imutavel_salvo_em_teste(OLD.empresa_id) THEN
        IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'conta de fechamento e imutavel';
END $$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_dre_fechamento_linhas_imutavel ON auditoria.dre_fechamento_linhas;
CREATE TRIGGER trg_dre_fechamento_linhas_imutavel
    BEFORE UPDATE OR DELETE ON auditoria.dre_fechamento_linhas
    FOR EACH ROW EXECUTE FUNCTION auditoria.dre_fechamento_linhas_imutavel();

DROP TRIGGER IF EXISTS trg_dre_fechamento_contas_imutavel ON auditoria.dre_fechamento_contas;
CREATE TRIGGER trg_dre_fechamento_contas_imutavel
    BEFORE UPDATE OR DELETE ON auditoria.dre_fechamento_contas
    FOR EACH ROW EXECUTE FUNCTION auditoria.dre_fechamento_contas_imutavel();

-- Nova versão da limpeza de teste (004): apaga também as filhas do fechamento,
-- explicitamente e antes do pai, com a sessão de limpeza aberta.
CREATE OR REPLACE FUNCTION auditoria.limpar_dados_teste(p_empresa UUID) RETURNS jsonb AS $$
DECLARE
    v_amb TEXT;
    n_lanc BIGINT; n_orc BIGINT; n_fech BIGINT; n_cargas BIGINT; n_fl BIGINT; n_fc BIGINT;
BEGIN
    SELECT ambiente INTO v_amb FROM companies WHERE id = p_empresa FOR UPDATE;
    IF v_amb IS NULL THEN
        RAISE EXCEPTION 'empresa nao encontrada';
    END IF;
    IF v_amb <> 'teste' THEN
        RAISE EXCEPTION 'limpeza recusada: empresa fora do ambiente de teste';
    END IF;

    PERFORM set_config('auditoria.limpando_teste', p_empresa::text, true);

    DELETE FROM auditoria.lancamentos WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_lanc = ROW_COUNT;
    DELETE FROM auditoria.dre_orcado WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_orc = ROW_COUNT;
    DELETE FROM auditoria.dre_fechamento_contas WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_fc = ROW_COUNT;
    DELETE FROM auditoria.dre_fechamento_linhas
     WHERE fechamento_id IN (SELECT id FROM auditoria.dre_fechamentos WHERE empresa_id = p_empresa);
    GET DIAGNOSTICS n_fl = ROW_COUNT;
    DELETE FROM auditoria.dre_fechamentos WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_fech = ROW_COUNT;
    DELETE FROM auditoria.cargas WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_cargas = ROW_COUNT;

    PERFORM set_config('auditoria.limpando_teste', '', true);

    RETURN jsonb_build_object('lancamentos', n_lanc, 'dre_orcado', n_orc,
                              'dre_fechamento_contas', n_fc, 'dre_fechamento_linhas', n_fl,
                              'dre_fechamentos', n_fech, 'cargas', n_cargas);
END $$ LANGUAGE plpgsql;
