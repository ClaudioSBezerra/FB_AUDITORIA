-- 001: Baseline de autenticação e multi-empresa (mesmo modelo do FB_FAROL).
--
-- Hierarquia de tenant: environments → enterprise_groups → companies.
-- O usuário chega a uma empresa via user_environments (+ preferred_company_id).
-- users.role = 'admin' é o admin de plataforma (MASTER); o perfil funcional
-- dentro da empresa é users.perfil (ver CHECK abaixo).

CREATE TABLE IF NOT EXISTS environments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS enterprise_groups (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    environment_id UUID NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    name           VARCHAR(255) NOT NULL,
    description    TEXT,
    created_at     TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_enterprise_groups_env ON enterprise_groups(environment_id);

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    full_name     VARCHAR(255) NOT NULL,
    is_verified   BOOLEAN DEFAULT FALSE,
    trial_ends_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    role          VARCHAR(50) DEFAULT 'user',
    -- Perfil funcional (papéis do módulo de Controladoria/Auditoria).
    --   auditoria     → atua nas exceções; leitura ampla, sem editar mapa do DRE
    --   controladoria → mantém mapa de-para, orçado e fecha períodos
    --   diretoria     → leitura estratégica + briefing
    --   gestor        → leitura do que é da sua área (segmentada por areas)
    perfil        TEXT NOT NULL DEFAULT 'gestor'
                  CHECK (perfil IN ('auditoria','controladoria','diretoria','gestor')),
    -- Áreas do menu visíveis: administrativo | operacional | estrategico.
    areas         TEXT[] NOT NULL DEFAULT ARRAY['estrategico','operacional','administrativo'],
    telefone      TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_lower ON users (lower(email));

CREATE TABLE IF NOT EXISTS companies (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id    UUID NOT NULL REFERENCES enterprise_groups(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    trade_name  VARCHAR(255),
    cnpj        VARCHAR(14),
    owner_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    blocked_at     TIMESTAMPTZ,
    blocked_reason TEXT,
    blocked_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    logo_data   BYTEA,
    logo_mime   VARCHAR(50),
    logo_nome   VARCHAR(255),
    created_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_companies_group ON companies(group_id);
CREATE INDEX IF NOT EXISTS idx_companies_owner ON companies(owner_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_companies_cnpj_unique ON companies(cnpj) WHERE cnpj IS NOT NULL;

CREATE TABLE IF NOT EXISTS user_environments (
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    environment_id       UUID NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    role                 VARCHAR(50) DEFAULT 'admin',
    preferred_company_id UUID REFERENCES companies(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, environment_id)
);
CREATE INDEX IF NOT EXISTS idx_user_environments_user_id ON user_environments(user_id);

CREATE TABLE IF NOT EXISTS verification_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token      VARCHAR(255) NOT NULL UNIQUE,
    type       VARCHAR(50) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used       BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_verification_tokens_token ON verification_tokens(token);
