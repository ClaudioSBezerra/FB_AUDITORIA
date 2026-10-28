---
title: 'Story 1.9: Nenhum endereço do banco do cliente no repositório (NFR7)'
type: 'chore'
created: '2026-10-28'
status: 'done'
baseline_commit: '26ef050b2c16b9ce76305d65ab40153e7f80c624'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** `docker-compose.yml`, `backend/.env.example` e `backend/cmd/jc-probe/main.go` trazem como valor padrão o IP e o nome de serviço do Oracle do cliente, e o `jc-probe` também o nome das views. Qualquer cópia do código revela a infraestrutura dele.

**Approach:** Remover esses valores padrão dos três lugares. `JC_ORACLE_HOST` e `JC_ORACLE_SERVICE` passam a obrigatórios; o `jc-probe` sem variáveis falha pedindo-as, sem endereço embutido. A história lista as variáveis a cadastrar no Coolify antes do push e marca o `go-ora` como dependência direta (`go mod tidy`).

## Boundaries & Constraints

**Always:** (decisões do dono do produto) `JC_ORACLE_HOST`, `JC_ORACLE_SERVICE` e `JC_ORACLE_DRE_VIEWS` já estão cadastradas no Coolify; ficam obrigatórias no compose com `${VAR:?mensagem}` e sem padrão no repositório. Só mudar valores padrão e mensagens; nomes das variáveis, a porta padrão `1521` (valor genérico) e o resto do `jc-probe` ficam como estão. O `.env.example` mostra as variáveis vazias com um comentário de que são obrigatórias e que a credencial fica só no `backend/.env` (fora do git) e no Coolify. A mensagem de falha do `jc-probe` cita só os nomes das variáveis que faltam. A lista de variáveis do Coolify vai no `Implementation Notes` e na mensagem de commit, sem valores. `go mod tidy` só altera `go.mod` e `go.sum` (o `go-ora` sai de `// indirect`).

**Never:** Não reescrever o histórico do git: o endereço continua nele, e a mitigação decidida é o repositório privado (hoje público, temporariamente, para o deploy). Não escrever o endereço, o service name nem os nomes das views reais em arquivo nenhum, nem nesta spec, em teste ou em comentário. Não fazer push nem tocar Coolify, Cloudflare ou o Oracle. Não ler dado do cliente. Não alterar `backend/.env` local, que não é versionado. Não mexer em `_bmad-output/planning-artifacts` (ver Design Notes).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| `jc-probe` sem variáveis | nenhuma `JC_*` | falha antes de conectar | pede `JC_ORACLE_HOST`, `JC_ORACLE_SERVICE`, usuário e senha pelo nome |
| `jc-probe` com host e service | variáveis definidas | conecta como hoje | N/A |
| Compose sem a variável | `JC_ORACLE_HOST` ausente | não renderiza | mensagem do compose |

</frozen-after-approval>

## Open Questions

_Nenhuma._

## Code Map

- `docker-compose.yml` -- serviço `api`, `environment`: as linhas `JC_ORACLE_HOST`, `JC_ORACLE_SERVICE` e `JC_ORACLE_DRE_VIEWS` têm padrão `:-valor`; `JC_ORACLE_PORT` (`:-1521`), `JC_ORACLE_USER`, `JC_ORACLE_PASS` e `JC_EMPRESA_ID` ficam. Achar por `grep -n "JC_ORACLE" docker-compose.yml`.
- `backend/.env.example` -- bloco "Oracle da JC": `JC_ORACLE_HOST=` e `JC_ORACLE_SERVICE=` com valor real e `JC_ORACLE_DRE_VIEWS=`; deixar vazios, com comentário.
- `backend/cmd/jc-probe/main.go` -- comentário de cabeçalho com os padrões (`default ...`) e a função `env(k, def)` usada com padrão real para host, service e views; passar a ler sem padrão e falhar listando o que falta (reaproveitar `fail`). Nome `env` mantido.
- `backend/go.mod` -- `github.com/sijms/go-ora/v2 v2.9.0 // indirect` mas importado diretamente por `jc-probe`; `go mod tidy`. `backend/Dockerfile` compila o `jc-probe` junto com o servidor: precisa continuar compilando.
- Achado fora do escopo: o mesmo endereço aparece em um arquivo de revisão em `_bmad-output/planning-artifacts/.../reviews/review-rubric.md`. Fica para a revisão de privacidade já combinada.
- Verificação do que sobrou: `git grep` pelo padrão de endereço IPv4 e pelo service name, sem digitar os valores na spec.

## Tasks & Acceptance

**Execution:**
- [ ] `docker-compose.yml` -- tirar os padrões do cliente (host, service, views, conforme as respostas) -- NFR7
- [ ] `backend/.env.example` -- variáveis vazias com comentário -- NFR7
- [ ] `backend/cmd/jc-probe/main.go` -- sem padrão embutido, falha pedindo as variáveis; atualizar o comentário de cabeçalho -- NFR7
- [ ] `backend/go.mod` e `go.sum` -- `go mod tidy` -- `go-ora` direto
- [ ] `Implementation Notes` e mensagem de commit -- lista das variáveis a cadastrar no Coolify antes do push (`JC_ORACLE_HOST`, `JC_ORACLE_SERVICE`, `JC_ORACLE_USER`, `JC_ORACLE_PASS`, `JC_EMPRESA_ID`, e `JC_ORACLE_DRE_VIEWS` se a pergunta mandar), sem valores -- evitar deploy quebrado

**Acceptance Criteria:**
- Dado `docker-compose.yml`, `backend/.env.example` e `backend/cmd/jc-probe`, quando revisados, então nenhum traz IP, service name ou schema do cliente como valor padrão, e `JC_ORACLE_HOST` e `JC_ORACLE_SERVICE` ficam obrigatórios.
- Dado o `jc-probe` sem variáveis, quando roda, então falha pedindo-as, sem endereço embutido.
- Dado o deploy, quando a história é entregue, então ela lista as variáveis a cadastrar no Coolify antes do push, e `go mod tidy` marca o `go-ora` como dependência direta.
- Dado o histórico do git, então o endereço continua nele; a mitigação decidida é o repositório privado, sem reescrever histórico.

## Implementation Notes

Tamanho do plano: cerca de 2000 tokens (estimativa), acima de 1600, porque a história cruza banco, backend e testes; sem divisão proposta.

Variáveis a cadastrar no Coolify antes do push (sem valores): `JC_ORACLE_HOST`, `JC_ORACLE_SERVICE`, `JC_ORACLE_DRE_VIEWS` (agora obrigatórias no compose), `JC_ORACLE_USER`, `JC_ORACLE_PASS`, `JC_EMPRESA_ID`. `JC_ORACLE_PORT` segue com padrão 1521. O `jc-probe` exige as cinco primeiras (host, service, views, usuário, senha).

Fora do escopo, deixado como está: as consultas do modo `-resumo` do `jc-probe` ainda citam por nome as views e o schema do cliente (literais em SQL, não valores padrão); entram na revisão de privacidade.

## Design Notes

O arquivo de revisão em `_bmad-output/planning-artifacts` mantém o endereço e não faz parte dos três arquivos do critério; mexer nele cabe a quem revisar a privacidade do repositório, não a esta história.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -p 1 ./...` -- expected: sem erro (inclui `./cmd/jc-probe`)
- `cd backend && go run ./cmd/jc-probe` sem variáveis `JC_*` -- expected: falha pedindo-as por nome, sem tentar conectar
- `cd backend && go mod tidy && git diff --stat go.mod go.sum` -- expected: só `go-ora` sai de `indirect`
- `docker compose config` sem as variáveis -- expected: erro do compose (variáveis obrigatórias)

## Review Triage Log

Revisão feita por mim sobre o diff (4,5 KB; o subagente já havia commitado em `ce75e21`, então os três revisores paralelos não rodaram). Sem achado que exija correção:

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Deploy no Coolify falha se `JC_ORACLE_HOST`, `SERVICE` ou `DRE_VIEWS` faltarem | medium | defer | é a regra escolhida (`${VAR:?}`); o dono do produto confirmou que as três já estão cadastradas |
| `-resumo` do `jc-probe` ainda traz nomes de view e schema do cliente em literais no SQL | low | defer | a spec manda deixar; entra na revisão de privacidade do Git |
| Endereço do banco continua no histórico do Git e em `review-rubric.md` | low | defer | decidido: limpeza só quando o dono do produto disser "concluímos" |
