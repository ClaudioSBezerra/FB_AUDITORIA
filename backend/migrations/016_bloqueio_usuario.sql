-- 016: bloqueio de usuário no lugar da expiração - história 4.6 (FR-22, AD-19).
--
-- O acesso passa a terminar por decisão (bloqueio com motivo), não por prazo.
-- trial_ends_at NÃO é apagada: guarda dados existentes e sai numa limpeza futura;
-- nenhum código a lê nem grava mais. bloqueado_por usa ON DELETE SET NULL (a FK é
-- de users para users; o trigger de imutabilidade é só do audit_log).

ALTER TABLE public.users ADD COLUMN IF NOT EXISTS bloqueado_em     TIMESTAMPTZ;
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS bloqueado_por    UUID REFERENCES public.users(id) ON DELETE SET NULL;
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS motivo_bloqueio  TEXT;
