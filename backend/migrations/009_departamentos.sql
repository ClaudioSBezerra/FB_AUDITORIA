-- 009: cadastro de departamentos - história 2.9.
--
-- Departamento é uma unidade de custo da empresa (o grupo diz a qual frente
-- pertence). Os códigos e nomes reais entram pela tela, nunca por migration.
-- Nova versão da limpeza de teste (008), com a tabela nova.

CREATE TABLE IF NOT EXISTS auditoria.departamentos (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id UUID NOT NULL REFERENCES companies(id),
    codigo     TEXT NOT NULL,
    nome       TEXT NOT NULL,
    grupo      TEXT NOT NULL CHECK (grupo IN ('Comercial', 'Logística', 'Administrativo')),
    UNIQUE (empresa_id, codigo),
    UNIQUE (id, empresa_id)
);

CREATE OR REPLACE FUNCTION auditoria.limpar_dados_teste(p_empresa UUID) RETURNS jsonb AS $$
DECLARE
    v_amb TEXT;
    n_lanc BIGINT; n_orc BIGINT; n_fech BIGINT; n_cargas BIGINT; n_fl BIGINT; n_fc BIGINT; n_fge BIGINT; n_fg BIGINT;
    n_dep BIGINT;
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
    DELETE FROM auditoria.filiais_gerenciais_erp WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_fge = ROW_COUNT;
    DELETE FROM auditoria.filiais_gerenciais WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_fg = ROW_COUNT;
    DELETE FROM auditoria.departamentos WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_dep = ROW_COUNT;

    PERFORM set_config('auditoria.limpando_teste', '', true);

    RETURN jsonb_build_object('lancamentos', n_lanc, 'dre_orcado', n_orc,
                              'dre_fechamento_contas', n_fc, 'dre_fechamento_linhas', n_fl,
                              'dre_fechamentos', n_fech, 'cargas', n_cargas,
                              'filiais_gerenciais_erp', n_fge, 'filiais_gerenciais', n_fg,
                              'departamentos', n_dep);
END $$ LANGUAGE plpgsql;
