-- 005: ids de área novos (AD-12). Idempotente; irreversível depois de aplicada em produção.
--   estrategico    -> empresa        (rótulo "Administrativo")
--   administrativo -> configuracoes
--   operacional    -> operacao
-- A ordem de users.areas é preservada. Ninguém recebe 'comercial' aqui.

UPDATE users
   SET areas = ARRAY(
         SELECT CASE a
                  WHEN 'estrategico'    THEN 'empresa'
                  WHEN 'administrativo' THEN 'configuracoes'
                  WHEN 'operacional'    THEN 'operacao'
                  ELSE a
                END
           FROM unnest(areas) WITH ORDINALITY AS t(a, ord)
          ORDER BY ord)
 WHERE areas && ARRAY['estrategico','administrativo','operacional'];

ALTER TABLE users ALTER COLUMN areas SET DEFAULT ARRAY['empresa','configuracoes','comercial','operacao'];
