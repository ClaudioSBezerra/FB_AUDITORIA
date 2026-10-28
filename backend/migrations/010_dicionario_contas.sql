-- 010: dicionário de contas por arquivo - história 2.10.
--
-- conta_dicionario guarda conta, descrição e grupo (a conta pode terminar em '%',
-- como no Mapa). conta_departamento liga a conta a um departamento: 'grupo' quando
-- deriva do grupo da conta (departamento cujo código é o grupo), 'manual' quando
-- foi corrigido conta a conta ou informado no arquivo. Não toca em dre_mapa.
-- Nova versão da limpeza de teste (009), com as duas tabelas novas.

CREATE TABLE IF NOT EXISTS auditoria.conta_dicionario (
    empresa_id UUID NOT NULL REFERENCES companies(id),
    conta      TEXT NOT NULL,
    descricao  TEXT NOT NULL DEFAULT '',
    grupo      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (empresa_id, conta)
);

CREATE TABLE IF NOT EXISTS auditoria.conta_departamento (
    empresa_id      UUID NOT NULL REFERENCES companies(id),
    conta_padrao    TEXT NOT NULL,
    departamento_id UUID NOT NULL,
    origem          TEXT NOT NULL CHECK (origem IN ('grupo', 'manual')),
    PRIMARY KEY (empresa_id, conta_padrao),
    FOREIGN KEY (departamento_id, empresa_id) REFERENCES auditoria.departamentos(id, empresa_id)
);

CREATE INDEX IF NOT EXISTS idx_conta_departamento_depto ON auditoria.conta_departamento (departamento_id);

CREATE OR REPLACE FUNCTION auditoria.limpar_dados_teste(p_empresa UUID) RETURNS jsonb AS $$
DECLARE
    v_amb TEXT;
    n_lanc BIGINT; n_orc BIGINT; n_fech BIGINT; n_cargas BIGINT; n_fl BIGINT; n_fc BIGINT; n_fge BIGINT; n_fg BIGINT;
    n_dep BIGINT; n_cd BIGINT; n_cdep BIGINT;
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
                              'conta_departamento', n_cdep);
END $$ LANGUAGE plpgsql;
