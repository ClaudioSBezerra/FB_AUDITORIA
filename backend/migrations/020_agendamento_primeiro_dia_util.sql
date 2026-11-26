-- 020: o agendamento pode rodar também no primeiro dia útil do mês (história 2.13).
ALTER TABLE auditoria.agendamento_erp ADD COLUMN IF NOT EXISTS primeiro_util BOOLEAN NOT NULL DEFAULT false;
