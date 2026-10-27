-- 002: schema próprio + trilha de auditoria imutável.

CREATE SCHEMA IF NOT EXISTS auditoria;

-- Log imutável: quem viu/aprovou/alterou, com carimbo de tempo. UPDATE e
-- DELETE são barrados por trigger (nem o app consegue reescrever história).
CREATE TABLE IF NOT EXISTS auditoria.audit_log (
    id          BIGSERIAL PRIMARY KEY,
    empresa_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    entidade    TEXT NOT NULL,
    entidade_id TEXT NOT NULL DEFAULT '',
    acao        TEXT NOT NULL,
    payload     JSONB,
    criado_em   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_log_empresa_data ON auditoria.audit_log (empresa_id, criado_em DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_entidade ON auditoria.audit_log (empresa_id, entidade, entidade_id);

CREATE OR REPLACE FUNCTION auditoria.audit_log_imutavel() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'auditoria.audit_log é imutável (% bloqueado)', TG_OP;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_audit_log_imutavel ON auditoria.audit_log;
CREATE TRIGGER trg_audit_log_imutavel
    BEFORE UPDATE OR DELETE ON auditoria.audit_log
    FOR EACH ROW EXECUTE FUNCTION auditoria.audit_log_imutavel();
