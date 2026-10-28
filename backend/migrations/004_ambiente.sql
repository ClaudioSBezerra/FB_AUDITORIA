-- 004: marcador Teste/Produção por empresa (AD-18, FR-21).
--
-- Todas as empresas existentes ficam em 'teste' pelo padrão (nenhum UPDATE aqui).
-- A virada para 'producao' é de mão única: o gatilho recusa o caminho de volta.
-- ATENÇÃO: irreversível depois de aplicada em produção; não há migration de volta.

ALTER TABLE companies ADD COLUMN IF NOT EXISTS ambiente TEXT NOT NULL DEFAULT 'teste';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'companies_ambiente_chk') THEN
        ALTER TABLE companies ADD CONSTRAINT companies_ambiente_chk CHECK (ambiente IN ('teste','producao'));
    END IF;
END $$;

-- Mão única: producao -> teste é recusado.
CREATE OR REPLACE FUNCTION auditoria.companies_ambiente_mao_unica() RETURNS trigger AS $$
BEGIN
    IF OLD.ambiente = 'producao' AND NEW.ambiente IS DISTINCT FROM 'producao' THEN
        RAISE EXCEPTION 'empresa em producao nao volta para teste';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_companies_ambiente_mao_unica ON companies;
CREATE TRIGGER trg_companies_ambiente_mao_unica
    BEFORE UPDATE OF ambiente ON companies
    FOR EACH ROW EXECUTE FUNCTION auditoria.companies_ambiente_mao_unica();

-- Empresa em producao não pode ser excluída (em teste a cascata segue permitida).
CREATE OR REPLACE FUNCTION auditoria.companies_recusa_delete_producao() RETURNS trigger AS $$
BEGIN
    IF OLD.ambiente = 'producao' THEN
        RAISE EXCEPTION 'empresa em producao nao pode ser excluida';
    END IF;
    RETURN OLD;
END $$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_companies_recusa_delete_producao ON companies;
CREATE TRIGGER trg_companies_recusa_delete_producao
    BEFORE DELETE ON companies
    FOR EACH ROW EXECUTE FUNCTION auditoria.companies_recusa_delete_producao();

-- Verdadeira só em 'teste' e com a sessão marcada por limpar_dados_teste.
CREATE OR REPLACE FUNCTION auditoria.imutavel_salvo_em_teste(p_empresa UUID) RETURNS boolean AS $$
    SELECT COALESCE(current_setting('auditoria.limpando_teste', true), '') = p_empresa::text
       AND EXISTS (SELECT 1 FROM companies WHERE id = p_empresa AND ambiente = 'teste');
$$ LANGUAGE sql STABLE;

-- Apaga os dados de domínio da empresa (só em 'teste'). Mantém dre_linhas,
-- dre_mapa, usuários e audit_log. Devolve as linhas apagadas por tabela.
-- Toda tabela de domínio nova entra aqui (nova versão via CREATE OR REPLACE).
CREATE OR REPLACE FUNCTION auditoria.limpar_dados_teste(p_empresa UUID) RETURNS jsonb AS $$
DECLARE
    v_amb TEXT;
    n_lanc BIGINT; n_orc BIGINT; n_fech BIGINT; n_cargas BIGINT;
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
    DELETE FROM auditoria.dre_fechamentos WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_fech = ROW_COUNT;
    DELETE FROM auditoria.cargas WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_cargas = ROW_COUNT;

    PERFORM set_config('auditoria.limpando_teste', '', true);

    RETURN jsonb_build_object('lancamentos', n_lanc, 'dre_orcado', n_orc,
                              'dre_fechamentos', n_fech, 'cargas', n_cargas);
END $$ LANGUAGE plpgsql;
