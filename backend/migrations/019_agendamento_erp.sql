-- 019: importação automática do banco do cliente — história 2.13 (AD-15).
-- É configuração: limpar_dados_teste não a apaga. Nasce desligada.
CREATE TABLE IF NOT EXISTS auditoria.agendamento_erp (
    empresa_id     UUID PRIMARY KEY REFERENCES companies(id) ON DELETE CASCADE,
    ativo          BOOLEAN NOT NULL DEFAULT false,
    frequencia     TEXT NOT NULL DEFAULT 'mensal' CHECK (frequencia IN ('diaria','semanal','mensal')),
    dia            INT NOT NULL DEFAULT 5,        -- mensal: 1 a 28; semanal: 0 (domingo) a 6
    hora           TEXT NOT NULL DEFAULT '06:30', -- America/Sao_Paulo
    desde          TIMESTAMPTZ NOT NULL DEFAULT now(), -- horário anterior a isto não dispara
    ultimo_horario TIMESTAMPTZ,                   -- último horário previsto já disparado
    atualizado_por UUID REFERENCES users(id) ON DELETE SET NULL,
    atualizado_em  TIMESTAMPTZ NOT NULL DEFAULT now()
);
