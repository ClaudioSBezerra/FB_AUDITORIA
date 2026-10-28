---
title: 'Story 1.1: Testes que registram o fechamento e a importação de hoje'
type: 'chore'
created: '2026-10-28'
status: 'done'
baseline_commit: '07b2ef806a35665b847ecfddbb9199b648e89bec'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O fechamento (`handlers/dre.go`) e a importação CSV (`handlers/dre_import.go`) não têm teste algum; só existem testes do cadastro público. Sem eles, mover esse código nas histórias 1.4 a 1.8 pode quebrar o que o Controller já usa sem ninguém notar.

**Approach:** Criar testes Go de caracterização, contra um banco PostgreSQL local de teste, que registram o comportamento de hoje (incluindo o que vai mudar). O banco é preparado pelo próprio teste, com migrations 001 a 003, sem tocar `fb_auditoria`. Nenhum código de produção muda.

## Boundaries & Constraints

**Always:** Sem PostgreSQL acessível, `go test` falha com mensagem clara (decisão do dono do produto). O nome do banco de teste termina em `_test` e o teste aborta com mensagem clara se não terminar, ou se for `fb_auditoria`. Usuário e senha locais vêm de variável de ambiente, com padrão `postgres`/`postgres` em `localhost:5432`. O banco `fb_auditoria_test` é criado se não existir e recriado limpo a cada execução de `go test`. Os testes chamam os handlers reais (`DREFechamentoHandler`, `DREImportarHandler`) com `AudContext` no contexto da requisição, sem passar pelo JWT. Cada teste cria a sua empresa e o seu usuário, para não depender de ordem. O que o código faz hoje e vai mudar fica marcado no nome e no comentário do teste ("muda na 1.6").

**Never:** Não alterar código de produção, migrations, `docker-compose.yml` nem `fb_auditoria`. Não usar `DATABASE_URL` do ambiente como banco de teste (poderia apontar para um banco real). Não recriar nem apagar nenhum banco cujo nome não termine em `_test`. Não depender de rede nem de Oracle.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Fechar mês mapeado | lançamentos todos mapeados | `201`, versão 1, linhas congeladas com os valores esperados, 1 registro `dre_fechamento/fechar` na trilha | N/A |
| Fechar com conta sem mapa | um lançamento sem mapeamento | `409`, nenhuma linha em `dre_fechamentos` nem `dre_fechamento_linhas` | nada gravado |
| Fechar de novo | mês já fechado | `201`, versão 2 (hoje permite; muda na 1.7) | N/A |
| Importar `;` e `,` | dois CSVs equivalentes | linhas entram com `carga_id`; trilha `lancamentos/importar` | N/A |
| Importar em mês fechado | competência com fechamento | aceita (muda na 1.6) | N/A |
| Banco com nome inválido | nome sem `_test` | o teste aborta antes de qualquer comando | mensagem clara |

</frozen-after-approval>

## Open Questions

_Nenhuma._

## Code Map

- `backend/handlers/dre.go` -- `DREFechamentoHandler` (GET lista; POST fecha: `somaPorLinha`, `409` se houver conta sem mapa, `MAX(versao)+1` por filial, grava linhas e trilha); `somaPorLinha`, `resolverConta`, `aplicarSubtotais`. Só ler, não mudar.
- `backend/handlers/dre_import.go` -- `DREImportarHandler` (multipart `arquivo`, detecta `;` ou `,`, insere sem substituir, `origem` fica no padrão `csv`). Só ler.
- `backend/handlers/audit_log.go` -- `writeAuditLogTx`; as consultas de trilha do teste leem `auditoria.audit_log`.
- `backend/handlers/auditoria_auth.go` -- `AudContext{UserID, EmpresaID, Perfil, Areas, Admin}` e `AudContextKey`; o teste monta o contexto direto.
- `backend/main.go` -- o runner de migrations (`onDBConnected`) está no pacote `main` e não pode ser importado; o helper repete a regra: aplica `backend/migrations/*.sql` em ordem de nome, uma `Exec` por arquivo.
- `backend/migrations/001` a `003` -- criam `environments`, `enterprise_groups`, `companies`, `users` e o schema `auditoria`; o fixture precisa de ambiente, grupo, empresa e usuário (`password_hash` é obrigatório).
- `backend/handlers/auth_register_test.go` e `backend/public_routes_test.go` -- únicos testes atuais, sem banco; manter passando.
- Novo `backend/internal/testdb/testdb.go` -- helper compartilhado (as histórias 1.4 a 1.8 o reutilizam): valida o nome, conecta em `postgres` para recriar o banco uma vez por execução (`sync.Once`), aplica as migrations e entrega `*sql.DB` mais um criador de fixture.

## Tasks & Acceptance

**Execution:**
- [ ] `backend/internal/testdb/testdb.go` -- criar: recusa nome sem `_test` ou igual a `fb_auditoria`; recria o banco (`DROP DATABASE IF EXISTS ... WITH (FORCE)` e `CREATE`); aplica as migrations; fixture de empresa e usuário `controladoria` (hash bcrypt de custo mínimo) -- isola o teste do banco real
- [ ] `backend/internal/testdb/testdb_test.go` -- criar: o nome inválido é recusado antes de qualquer comando -- prova a guarda sem Postgres
- [ ] `backend/handlers/dre_fechamento_test.go` -- criar: fechamento feliz (versão 1, linhas, trilha), `409` sem mapa e sem gravação, segundo fechamento (versão 2, "muda na 1.7") -- registra o hoje
- [ ] `backend/handlers/dre_import_test.go` -- criar: CSV com `;` e com `,`, `carga_id`, trilha; importação em mês fechado aceita ("muda na 1.6") -- registra o hoje

**Acceptance Criteria:**
- Dado um PostgreSQL local, quando `go test ./...` roda em `backend/`, então o banco `fb_auditoria_test` é recriado limpo e todos os testes passam.
- Dado um nome de banco sem `_test`, quando o helper abre o banco, então o teste aborta com mensagem clara e nada é executado.
- Dado `fb_auditoria`, quando o helper roda, então esse banco nunca é conectado nem alterado.
- Dado o código atual, quando os testes rodam, então `go build ./...` e `go vet ./...` seguem passando e nenhum arquivo fora dos novos foi alterado.

## Implementation Notes

Tamanho do plano: cerca de 1900 tokens (estimativa), acima de 1600, porque a história cruza banco, backend e testes; sem divisão proposta.

## Design Notes

`go test ./...` roda pacotes em paralelo e todos usariam o mesmo banco recriado; hoje só `handlers` usa banco. Quando a 1.4 criar testes em `dre`, rodar com `go test -p 1 ./...` ou dar a cada pacote o seu banco (`..._dre_test`). O hash bcrypt do fixture usa o custo mínimo porque `CheckPasswordHash` aceita qualquer custo, e o custo 14 do login deixaria a suíte lenta.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./...` -- expected: sem erro
- `cd backend && go test -p 1 ./...` -- expected: todos passam; `fb_auditoria_test` recriado
- `psql -h localhost -U postgres -lqt | cut -d'|' -f1 | grep fb_auditoria` -- expected: `fb_auditoria` com a mesma data de antes e `fb_auditoria_test` presente

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Guarda valida só o nome, não o host (DROP DATABASE em servidor remoto) | medium | patch | `Abrir` aceitava qualquer `TEST_DB_HOST`; agora só localhost, 127.0.0.1 e ::1 |
| `part.Write` e `mw.Close` sem checagem de erro em `importar` | low | patch | corrigido com `t.Fatal` |
| `"versao":1` por substring | low | patch | substituído por decodificação JSON (`versaoDaResposta`) |
| Nenhum teste passa pelo `AuditoriaAuthMiddleware`; sem isolamento entre empresas | medium | defer | a spec fixa os handlers chamados direto; vai para as histórias de acesso (Épico 4) |
| Cobertura fina (filial, sinal negativo, mes inválido, CSV malformado, reimportação) | low | rejeitado | fora da matriz da spec; as histórias 1.4 a 1.6 cobrem esses ramos ao mover o código |
| `sync.Once` ignora o nome e não fecha o pool | low | rejeitado | só há um banco e um pacote; a nota de `-p 1` já está na spec |
| Senha com caracteres especiais no DSN | low | rejeitado | padrão local simples; correção acrescenta complexidade |
| `DROP ... WITH (FORCE)` exige Postgres 13 | false | rejeitado | produção e local usam Postgres 15 |
| Helper não usa `schema_migrations` | low | rejeitado | já declarado na spec (helper repete só a ordem dos arquivos) |
| Guarda de nome aceita outro `*_test` | low | rejeitado | a spec define o padrão `_test`; o host local limita o dano |
| Teste de nome acoplado ao texto "inacessível" | low | rejeitado | negligível |
| Demais (limpeza de fixtures, mensagem do 409, `-trimpath`) | low | rejeitado | sem dano real demonstrado |
