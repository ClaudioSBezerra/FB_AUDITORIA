---
title: 'Story 1.2: Marcador Teste/Produção e apagar os dados de teste (FR-21, AD-18)'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '45d8fb4e248f69bd33f6c3d389c44a152c33baf2'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Não há como separar a validação com planilhas (dado descartável) do uso real, nem como começar do zero sem risco de apagar dado real. Hoje nada marca o ambiente da empresa e não existe rota para limpar os dados de teste.

**Approach:** Migration `companies.ambiente` (`teste` ou `producao`, padrão `teste`) com gatilho de mão única; funções `auditoria.limpar_dados_teste(empresa)` e `auditoria.imutavel_salvo_em_teste(empresa)`; rota só para admin de plataforma que apaga em `teste` depois de conferir a senha, com registro na trilha. Só backend; a tela vem na 1.3.

## Boundaries & Constraints

**Always:** Gatilho `BEFORE DELETE` em `companies` que recusa `producao` (decisão do dono do produto), na migration desta história; em `teste` a exclusão em cascata segue permitida. A senha é conferida com `CheckPasswordHash` contra `users.password_hash` do admin logado (o mesmo bcrypt do login); admin de plataforma é `users.role = 'admin'` (`AudContext.Admin`); senha ausente ou errada devolve `403`. A função de limpeza recusa fora de `teste` mesmo se chamada direto por SQL. Apaga `lancamentos`, `dre_orcado`, `dre_fechamentos` (e as linhas por cascata) e `cargas` da empresa; mantém `dre_linhas`, `dre_mapa` e usuários; nunca toca `audit_log`. A rota grava na trilha, na mesma transação, com `writeAuditLogTx` (quantas linhas saíram de cada tabela). `imutavel_salvo_em_teste(empresa)` é verdadeira só com ambiente `teste` e com a sessão marcada por `set_config(..., true)` dentro da função de limpeza. A migration é idempotente.

**Never:** Nenhum `UPDATE` de `ambiente` na migration: todas as empresas existentes ficam em `teste` pelo padrão. Não apagar `audit_log` nem alterar o gatilho dele. Não apagar de outra empresa além da ativa. Não criar rota para voltar a `teste`. Nesta história não há tela, não há virada para produção (1.3) e não há gatilho de imutabilidade em fechamento (1.7). A migration só roda no boot da API; push é decisão humana.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Apagar em teste | admin, senha certa, `teste` | dados de domínio saem; estrutura, Mapa, usuários e `audit_log` ficam; trilha registra | N/A |
| Senha errada ou ausente | admin, `teste` | nada apagado | `403` |
| Produção | `producao`, qualquer chamada, inclusive função por SQL | nada apagado | `409` na rota; exceção na função |
| Não admin | perfil `controladoria` sem `role admin` | nada apagado | `403` |
| Voltar a teste | `producao` para `teste` por SQL | recusado | exceção do gatilho |

</frozen-after-approval>

## Open Questions

_Nenhuma._

## Code Map

- `backend/migrations/001_baseline_auth.sql` -- `companies` (sem coluna de ambiente); `002` -- `audit_log` e seu gatilho de imutabilidade (modelo do estilo, não mudar); `003` -- `lancamentos`, `dre_orcado`, `dre_fechamentos`, `dre_fechamento_linhas` (cascata por `fechamento_id`), `cargas`.
- Nova migration, próximo número livre (hoje `004`): `ALTER TABLE ... ADD COLUMN IF NOT EXISTS ambiente` com `CHECK`, função do gatilho `producao` para `teste`, `DROP TRIGGER IF EXISTS` e `CREATE TRIGGER`, as duas funções. Usar `CREATE OR REPLACE`. **Irreversível depois de aplicada em produção: não há migration de volta.**
- `backend/main.go` -- runner: erro com `already exists` ou `duplicate` é tratado como "já aplicada"; escrever a migration sem depender disso. Registrar a rota perto de `/api/aud/me`, com `withAud(handler, "")` (sem área) e checagem de `c.Admin` no handler.
- `backend/handlers/auth.go` -- `CheckPasswordHash`; reaproveitar sem mudar.
- `backend/handlers/audit_log.go` -- `writeAuditLogTx`; reaproveitar.
- Novo `backend/handlers/ambiente.go` -- `AmbienteLimparHandler` (POST `/api/aud/ambiente/limpar`, corpo `{senha}`), sem SQL de limpeza própria: chama `SELECT auditoria.limpar_dados_teste($1)` dentro da transação.
- Novo `backend/handlers/ambiente_test.go` -- usa `internal/testdb` da 1.1.
- `backend/handlers/environment.go` -- handlers de `DELETE` de empresa, grupo e ambiente (a proteção de Produção vem do gatilho em `companies`); não mudar.

## Tasks & Acceptance

**Execution:**
- [ ] `backend/migrations/00N_ambiente.sql` -- coluna, gatilho de mão única, `limpar_dados_teste`, `imutavel_salvo_em_teste` -- base do AD-18
- [ ] `backend/handlers/ambiente.go` -- rota de limpeza: só admin, senha, transação, trilha -- NFR11
- [ ] `backend/main.go` -- registrar `/api/aud/ambiente/limpar` -- exposição
- [ ] `backend/handlers/ambiente_test.go` -- uma linha de teste por linha da matriz, mais "a função recusa em `producao` chamada por SQL" e "`audit_log` mantém as linhas" -- prova do requisito

**Acceptance Criteria:**
- Dado as empresas existentes, quando a migration roda, então `companies.ambiente` existe com padrão `teste` e todas ficam em `teste`.
- Dado uma empresa em `producao`, quando alguém tenta voltar para `teste` por SQL ou API, então o banco recusa.
- Dado o admin de plataforma e a senha correta, quando pede para apagar uma empresa em `teste`, então `limpar_dados_teste` apaga lançamentos, orçado, fechamentos e cargas, mantém estrutura do DRE, Mapa, usuários e `audit_log`, e grava na trilha.
- Dado senha errada ou ausente, quando pede para apagar, então `403` e nada é apagado.
- Dado empresa em `producao`, quando chama apagar (inclusive direto na API), então a função recusa e nada é apagado.
- Dado perfil que não é admin de plataforma, quando chama apagar, então `403`; e `imutavel_salvo_em_teste(empresa)` é verdadeira só em `teste` com a sessão aberta pela função de limpeza.

## Implementation Notes

Tamanho do plano: cerca de 2000 tokens (estimativa), acima de 1600, porque a história cruza banco, backend e testes; sem divisão proposta.

## Design Notes

Toda história futura com tabela de domínio inclui a tabela em `limpar_dados_teste` (nova versão da função com `CREATE OR REPLACE`). A tabela de fechamento ainda não tem gatilho de imutabilidade, então a limpeza funciona sem `set_config` por ora; a função já marca a sessão para quando a 1.7 e a 1.8 criarem os gatilhos.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./...` -- expected: sem erro
- `cd backend && go test -p 1 ./...` -- expected: todos passam, inclusive os da 1.1

**Manual checks (if no CLI):**
- Aplicar a migration duas vezes no banco de teste (`fb_auditoria_test`): a segunda não falha.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Erro de banco vira 404/403 e `Unmarshal` ignorado (trilha com `apagadas` nulo) | medium | patch | `ambiente.go` corrigido: 404 só para no-rows, 500 para o resto, rollback se as contagens não decodificam; corpo limitado a 1 KiB |
| Sem prova de isolamento entre empresas | medium | patch | `TestAmbiente_IsolamentoEntreEmpresas` |
| Cascata das linhas do fechamento sem prova | low | patch | `TestAmbiente_CascataLinhasFechamento` |
| Lista de `limpar_dados_teste` sem teste de cobertura de tabelas | medium | patch | `TestAmbiente_LimpezaCobreTodaTabelaDeDominio` |
| `imutavel_salvo_em_teste` sem gatilho que a use | low | rejeitado | a spec declara que os gatilhos vêm nas 1.7 e 1.8 |
| Empresas existentes viram `teste` e podem ser limpas | medium | defer | é o padrão decidido pelo dono do produto (produção hoje é Teste); a virada para Produção é a 1.3 |
| Marcador `limpando_teste` pode ser ligado por qualquer sessão | low | defer | exige desenho de gatilho; tratar ao criar os gatilhos da 1.7 |
| Sem limite de tentativas nem trilha de senha errada | low | defer | rota só para admin de plataforma; vai para o Épico 4 |
| Sem tela, banner nem rota para ler/virar ambiente | false | rejeitado | são a história 1.3 |
| Confirmação pelo nome da empresa, `REVOKE EXECUTE`, `TRUNCATE`, lock de tabelas, `search_path` | low | rejeitado | fora da spec, sem dano demonstrado; migrations existentes usam o mesmo padrão |
