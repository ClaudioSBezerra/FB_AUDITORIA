-- 018: ingestão do ERP (Oracle) — histórias 2.1, 2.3, 2.4 e 2.22.

-- UF de destino que a view consolidada traz (AD-3); vazia nas demais origens.
ALTER TABLE auditoria.lancamentos ADD COLUMN IF NOT EXISTS uf TEXT NOT NULL DEFAULT '';

-- Uma conta, uma fonte (AD-16). É configuração: limpar_dados_teste não a apaga.
CREATE TABLE IF NOT EXISTS auditoria.conta_fonte (
    empresa_id   UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    conta_padrao TEXT NOT NULL,
    origem       TEXT NOT NULL,
    criado_em    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (empresa_id, conta_padrao)
);

-- Registro de cada importação do ERP (AD-15).
CREATE TABLE IF NOT EXISTS auditoria.execucoes (
    id         BIGSERIAL PRIMARY KEY,
    empresa_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    gatilho    TEXT NOT NULL,                 -- manual | atualizar
    de         DATE NOT NULL,
    ate        DATE NOT NULL,
    inicio     TIMESTAMPTZ NOT NULL DEFAULT now(),
    fim        TIMESTAMPTZ,
    status     TEXT NOT NULL DEFAULT 'executando',
    mensagem   TEXT NOT NULL DEFAULT '',
    resumo     JSONB NOT NULL DEFAULT '[]'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_execucoes_empresa ON auditoria.execucoes (empresa_id, id DESC);

-- A limpeza de teste passa a apagar o registro das importações (conta_fonte fica).
CREATE OR REPLACE FUNCTION auditoria.limpar_dados_teste(p_empresa UUID) RETURNS jsonb AS $$
DECLARE
    v_amb TEXT;
    n_lanc BIGINT; n_orc BIGINT; n_fech BIGINT; n_cargas BIGINT; n_fl BIGINT; n_fc BIGINT; n_fge BIGINT; n_fg BIGINT;
    n_dep BIGINT; n_cd BIGINT; n_cdep BIGINT; n_am BIGINT; n_ag BIGINT;
    n_cel BIGINT; n_cec BIGINT; n_ceca BIGINT; n_exec BIGINT;
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
    DELETE FROM auditoria.comparativo_externo_linhas WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_cel = ROW_COUNT;
    DELETE FROM auditoria.comparativo_externo_contas WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_cec = ROW_COUNT;
    DELETE FROM auditoria.comparativo_externo_cargas WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_ceca = ROW_COUNT;
    DELETE FROM auditoria.execucoes WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_exec = ROW_COUNT;
    DELETE FROM auditoria.cargas WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_cargas = ROW_COUNT;
    DELETE FROM auditoria.agrupamentos_filiais_membros WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_am = ROW_COUNT;
    DELETE FROM auditoria.agrupamentos_filiais WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_ag = ROW_COUNT;
    DELETE FROM auditoria.filiais_gerenciais_erp WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_fge = ROW_COUNT;
    DELETE FROM auditoria.filiais_gerenciais WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_fg = ROW_COUNT;
    DELETE FROM auditoria.conta_departamento WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_cdep = ROW_COUNT;
    DELETE FROM auditoria.conta_dicionario WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_cd = ROW_COUNT;
    DELETE FROM auditoria.departamentos WHERE empresa_id = p_empresa;
    GET DIAGNOSTICS n_dep = ROW_COUNT;

    PERFORM set_config('auditoria.limpando_teste', '', true);

    RETURN jsonb_build_object('lancamentos', n_lanc, 'dre_orcado', n_orc,
                              'dre_fechamento_contas', n_fc, 'dre_fechamento_linhas', n_fl,
                              'dre_fechamentos', n_fech, 'cargas', n_cargas,
                              'filiais_gerenciais_erp', n_fge, 'filiais_gerenciais', n_fg,
                              'departamentos', n_dep, 'conta_dicionario', n_cd,
                              'conta_departamento', n_cdep,
                              'agrupamentos_filiais_membros', n_am, 'agrupamentos_filiais', n_ag,
                              'comparativo_externo_linhas', n_cel, 'comparativo_externo_contas', n_cec,
                              'comparativo_externo_cargas', n_ceca,
                              'execucoes', n_exec);
END $$ LANGUAGE plpgsql;
