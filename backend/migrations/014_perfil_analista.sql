-- 014: perfil `analista` em users.perfil (AD-11, história 4.2). Idempotente.
-- Troca a CHECK de perfil por uma nomeada que aceita também 'analista'. Não toca
-- em nenhum usuário existente: ninguém vira analista por esta migration.

DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT conname
          FROM pg_constraint
         WHERE conrelid = 'public.users'::regclass
           AND contype = 'c'
           AND pg_get_constraintdef(oid) LIKE '%perfil%'
    LOOP
        EXECUTE format('ALTER TABLE public.users DROP CONSTRAINT %I', r.conname);
    END LOOP;

    ALTER TABLE public.users ADD CONSTRAINT users_perfil_check
        CHECK (perfil IN ('analista','auditoria','controladoria','diretoria','gestor'));
END $$;
