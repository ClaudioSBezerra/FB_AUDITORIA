-- 003: DRE automatizado.
--
-- Camada analítica própria (não onera o Winthor): lançamentos contábeis/
-- gerenciais entram em auditoria.lancamentos (CSV agora; ingestão Oracle em
-- janelas incrementais depois, gravando na MESMA tabela via origem/doc_ref).
-- A estrutura do DRE e o de-para conta→linha são dados, editáveis pela
-- controladoria sem depender de deploy.

-- Estrutura do DRE (árvore + subtotais).
--   tipo 'conta'    → soma os lançamentos mapeados para a linha
--   tipo 'subtotal' → soma das linhas listadas em soma_de (códigos)
CREATE TABLE IF NOT EXISTS auditoria.dre_linhas (
    id         BIGSERIAL PRIMARY KEY,
    empresa_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    codigo     TEXT NOT NULL,
    descricao  TEXT NOT NULL,
    nivel      INT  NOT NULL DEFAULT 1,
    ordem      INT  NOT NULL,
    tipo       TEXT NOT NULL DEFAULT 'conta' CHECK (tipo IN ('conta','subtotal')),
    soma_de    TEXT[] NOT NULL DEFAULT '{}',
    ativo      BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE (empresa_id, codigo)
);

-- De-para configurável: conta contábil/gerencial → linha do DRE.
-- conta_padrao aceita prefixo com '%' (ex.: '3.1.%'); a mais específica
-- (maior comprimento sem '%') vence.
CREATE TABLE IF NOT EXISTS auditoria.dre_mapa (
    id           BIGSERIAL PRIMARY KEY,
    empresa_id   UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    conta_padrao TEXT NOT NULL,
    linha_codigo TEXT NOT NULL,
    sinal        SMALLINT NOT NULL DEFAULT 1 CHECK (sinal IN (-1, 1)),
    UNIQUE (empresa_id, conta_padrao)
);

-- Lançamentos analíticos (competência mensal). valor segue a convenção do
-- plano de contas de origem; o sinal do mapa normaliza para o DRE.
CREATE TABLE IF NOT EXISTS auditoria.lancamentos (
    id           BIGSERIAL PRIMARY KEY,
    empresa_id   UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    competencia  DATE NOT NULL CHECK (EXTRACT(DAY FROM competencia) = 1),
    filial       TEXT NOT NULL DEFAULT '',
    centro_custo TEXT NOT NULL DEFAULT '',
    canal        TEXT NOT NULL DEFAULT '',
    conta        TEXT NOT NULL,
    conta_desc   TEXT NOT NULL DEFAULT '',
    valor        NUMERIC(18,2) NOT NULL,
    -- rastreabilidade para o drill-down até o documento original
    origem       TEXT NOT NULL DEFAULT 'csv',     -- csv | winthor
    tipo_doc     TEXT NOT NULL DEFAULT '',        -- nota | titulo | pedido | ...
    doc_ref      TEXT NOT NULL DEFAULT '',
    historico    TEXT NOT NULL DEFAULT '',
    carga_id     BIGINT,
    criado_em    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_lanc_empresa_comp ON auditoria.lancamentos (empresa_id, competencia, filial);
CREATE INDEX IF NOT EXISTS idx_lanc_conta ON auditoria.lancamentos (empresa_id, conta);

CREATE TABLE IF NOT EXISTS auditoria.cargas (
    id         BIGSERIAL PRIMARY KEY,
    empresa_id UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    arquivo    TEXT NOT NULL,
    linhas     INT NOT NULL DEFAULT 0,
    criado_em  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Orçado por linha/filial/mês (comparativo orçado × realizado).
CREATE TABLE IF NOT EXISTS auditoria.dre_orcado (
    empresa_id   UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    competencia  DATE NOT NULL CHECK (EXTRACT(DAY FROM competencia) = 1),
    filial       TEXT NOT NULL DEFAULT '',
    linha_codigo TEXT NOT NULL,
    valor        NUMERIC(18,2) NOT NULL,
    PRIMARY KEY (empresa_id, competencia, filial, linha_codigo)
);

-- Fechamento mensal: "congela" o período e versiona. Reabrir cria nova
-- versão; a anterior permanece (os números apresentados não mudam).
CREATE TABLE IF NOT EXISTS auditoria.dre_fechamentos (
    id          BIGSERIAL PRIMARY KEY,
    empresa_id  UUID NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    competencia DATE NOT NULL CHECK (EXTRACT(DAY FROM competencia) = 1),
    versao      INT  NOT NULL,
    filial      TEXT NOT NULL DEFAULT '',   -- '' = consolidado
    fechado_por UUID REFERENCES users(id) ON DELETE SET NULL,
    fechado_em  TIMESTAMPTZ NOT NULL DEFAULT now(),
    observacao  TEXT NOT NULL DEFAULT '',
    UNIQUE (empresa_id, competencia, filial, versao)
);

CREATE TABLE IF NOT EXISTS auditoria.dre_fechamento_linhas (
    fechamento_id BIGINT NOT NULL REFERENCES auditoria.dre_fechamentos(id) ON DELETE CASCADE,
    linha_codigo  TEXT NOT NULL,
    descricao     TEXT NOT NULL,
    nivel         INT  NOT NULL,
    ordem         INT  NOT NULL,
    valor         NUMERIC(18,2) NOT NULL,
    PRIMARY KEY (fechamento_id, linha_codigo)
);
