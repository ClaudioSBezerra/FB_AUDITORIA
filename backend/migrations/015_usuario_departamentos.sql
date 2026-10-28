-- 015: departamentos de cada Gerente - história 4.5 (FR-18, AD-10).
--
-- Liga um usuário a um ou mais departamentos da empresa; só faz sentido para o
-- perfil `gestor` (a regra vive no backend). O departamento é da mesma empresa
-- do vínculo (FK composta). É configuração, não dado de teste: limpar_dados_teste
-- não a apaga, mas o vínculo some junto com o departamento (ON DELETE CASCADE),
-- que a limpeza apaga. Nenhum vínculo é criado por esta migration.

CREATE TABLE IF NOT EXISTS auditoria.usuario_departamentos (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    departamento_id UUID NOT NULL,
    empresa_id      UUID NOT NULL REFERENCES companies(id),
    PRIMARY KEY (user_id, departamento_id),
    FOREIGN KEY (departamento_id, empresa_id) REFERENCES auditoria.departamentos(id, empresa_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_usuario_departamentos_empresa ON auditoria.usuario_departamentos (empresa_id, user_id);
