-- 017: comparativo de fechamento (planilha de análise do cliente x sistema) - história 2.18.
--
-- A planilha de análise é enviada e o conteúdo relevante fica gravado aqui, em
-- tabelas próprias: nada disso entra em `lancamentos`, então o DRE não soma. São
-- configuração de validação, mas a limpeza de teste as cobre (nova versão da
-- função, com as três tabelas, filha antes da mãe). Reenviar o mesmo ano
-- substitui o escopo (a regra é do serviço, não do banco).

CREATE TABLE IF NOT EXISTS auditoria.comparativo_externo_cargas (
    id         BIGSERIAL PRIMARY KEY,
    empresa_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    arquivo    TEXT NOT NULL,
    ano        INT NOT NULL,
    visao      TEXT NOT NULL DEFAULT '',
    filial     TEXT NOT NULL DEFAULT '',
    criado_em  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comparativo_externo_cargas_empresa ON auditoria.comparativo_externo_cargas (empresa_id, ano, id DESC);

-- Linhas do DRE da planilha (aba MENSAL), por competência e filial.
CREATE TABLE IF NOT EXISTS auditoria.comparativo_externo_linhas (
    empresa_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    carga_id    BIGINT NOT NULL REFERENCES auditoria.comparativo_externo_cargas(id),
    competencia DATE NOT NULL CHECK (EXTRACT(DAY FROM competencia) = 1),
    filial      TEXT NOT NULL DEFAULT '',
    linha_chave TEXT NOT NULL,
    descricao   TEXT NOT NULL DEFAULT '',
    valor       NUMERIC(18,2) NOT NULL,
    PRIMARY KEY (empresa_id, competencia, filial, linha_chave)
);

CREATE INDEX IF NOT EXISTS idx_comparativo_externo_linhas_escopo ON auditoria.comparativo_externo_linhas (empresa_id, competencia, filial);
CREATE INDEX IF NOT EXISTS idx_comparativo_externo_linhas_carga ON auditoria.comparativo_externo_linhas (carga_id);

-- Soma por conta, mês e loja da aba Real_AA da planilha.
CREATE TABLE IF NOT EXISTS auditoria.comparativo_externo_contas (
    empresa_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    carga_id    BIGINT NOT NULL REFERENCES auditoria.comparativo_externo_cargas(id),
    competencia DATE NOT NULL CHECK (EXTRACT(DAY FROM competencia) = 1),
    filial      TEXT NOT NULL DEFAULT '',
    conta       TEXT NOT NULL,
    valor       NUMERIC(18,2) NOT NULL,
    PRIMARY KEY (empresa_id, competencia, filial, conta)
);

CREATE INDEX IF NOT EXISTS idx_comparativo_externo_contas_escopo ON auditoria.comparativo_externo_contas (empresa_id, competencia, filial);
CREATE INDEX IF NOT EXISTS idx_comparativo_externo_contas_carga ON auditoria.comparativo_externo_contas (carga_id);

CREATE OR REPLACE FUNCTION auditoria.limpar_dados_teste(p_empresa UUID) RETURNS jsonb AS $$
DECLARE
    v_amb TEXT;
    n_lanc BIGINT; n_orc BIGINT; n_fech BIGINT; n_cargas BIGINT; n_fl BIGINT; n_fc BIGINT; n_fge BIGINT; n_fg BIGINT;
    n_dep BIGINT; n_cd BIGINT; n_cdep BIGINT; n_am BIGINT; n_ag BIGINT;
    n_cel BIGINT; n_cec BIGINT; n_ceca BIGINT;
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
                              'comparativo_externo_cargas', n_ceca);
END $$ LANGUAGE plpgsql;
