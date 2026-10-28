---
title: 'Story 4.7: Escopo por departamento que nega por padrão (FR-18, AD-10)'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '489a454'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/spec-4-5-usuarios-com-departamentos.md'
  - '{project-root}/_bmad-output/implementation-artifacts/spec-4-3-rotas-por-capacidade.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O `gestor` está fechado às visões por departamento e ao detalhe (403, fail closed da 3.5/3.9/3.10), mas alcança o DRE do grupo e a rota antiga de lançamentos: sem escopo real, ou ele vê o que não deve ou não vê nada.

**Approach:** Tipo `dre.EscopoDepto{Irrestrito bool; Deptos []string}` calculado no servidor (`handlers/escopo.go`, `escopoDe`) e aplicado nas rotas que devolvem valor por conta. O valor zero não dá acesso. O Gerente passa a ter `ler_no_escopo` com escopo; as rotas do grupo exigem `ler`.

## Boundaries & Constraints

**Always:** Escopo calculado só no servidor, a partir da capacidade: `ler` (controladoria, analista, diretoria, auditoria, admin de plataforma) = irrestrito; só `ler_no_escopo` (gestor) = códigos de `usuario_departamentos` da empresa ativa; nenhuma das duas = valor zero. Casamento de conta por prefixo em Go (`MelhorPadrao`, via `ResolvedorDepartamento`; mês fechado usa o departamento gravado em `dre_fechamento_contas.departamento`); o código do departamento do escopo é comparado por igualdade; nenhum `LIKE`. Gerente: só as contas dos seus departamentos; sem departamento recebe resposta vazia (visão, contas) ou `403` (lançamentos de uma conta); departamento alheio pedido direto (`?departamento=`) ou conta fora do escopo `403` registrado como `negado`; nunca vê "Sem departamento"; a visão por departamento não traz a base do grupo (`pct_status: "restrito"`, `faturamento_liquido: 0`, percentuais nulos). DRE do grupo (`/dre`, `/dre/trimestres`) e a rota antiga `/dre/lancamentos`: capacidade `ler` na rota e no handler (`exigeLer`): Gerente `403` sem corpo de dados.

**Never:** Não confiar em parâmetro do cliente para o escopo. Não criar rota de valor por conta sem escopo (a varredura falha).

## Mapeamento (mudanças de 4.3)

| Rota | Antes | Agora |
|---|---|---|
| `/api/aud/dre` | ler_no_escopo | ler |
| `/api/aud/dre/lancamentos` (antiga) | ler_no_escopo | ler |
| `/api/aud/dre/departamentos` | ler | ler_no_escopo + escopo (`?departamento=` opcional) |
| `/api/aud/dre/contas`, `contas/lancamentos` | ler | ler_no_escopo + escopo |
| `/api/aud/dre/trimestres` | ler | ler (sem mudança; é DRE do grupo) |

## Decisões

- Nome `EscopoDepto` (já existia `dre.Escopo`, da ingestão). Os métodos `ContasDoEscopo`, `ContaNoEscopo` e `ContasDaLinha(…, esc)` em `dre/escopo.go`/`detalhe.go` exigem o escopo como argumento; com o valor zero devolvem vazio sem consultar.
- Conta pedida (`contas/lancamentos`) está no escopo se tem valor no período em algum departamento do escopo (no snapshot, em mês fechado). Conta que não existe, que tem padrão `%`/curinga ou é de outro departamento: `403`. Conta com departamento diferente entre meses (dicionário mudou depois do fechamento): vale se algum mês for do escopo e os lançamentos listados são os da conta inteira — registrado como ponto frágil.
- Gerente sem departamento: visão e contas respondem `200` vazio ("resposta vazia"); a conta isolada responde `403` (não existe conta "dele").
- Gerente não recebe o faturamento líquido nem percentuais: a base é do grupo (AD-10 "sem receber a base"); a visão do Gerente não chama `LerFiliais`.
- Rotas de cadastro e estrutura que o Gerente já lia (filiais, agrupamentos, departamentos, dicionário, linhas, mapa, situação dos fechamentos) não devolvem valor e ficam em `semValorPorConta` no teste de varredura; o dicionário mostra a todos os perfis o departamento de cada conta (sem valor), como desde a 2.10.
- Frontend: o Gerente começa no DRE por departamento; as abas e rotas "DRE" deixam de existir para ele e "DRE por departamento" passa a ser dele. A tela "Meu departamento" é a 4.8.

## Tasks & Acceptance

- [x] `backend/dre/escopo.go`, `detalhe.go`, `percentual.go`
- [x] `backend/handlers/escopo.go`; `dre.go` (departamentos, DRE do grupo, rota antiga), `dre_detalhe.go` (contas, lançamentos da conta, trimestres)
- [x] `backend/main.go`, `perfis.go` (comentários), tabela de `rotas_capacidades_test.go`
- [x] `backend/handlers/escopo_test.go` -- A e B (aberto e fechado), sem departamento vazio, departamento alheio 403, DRE do grupo 403, perfis sem restrição, "Sem departamento" nunca para Gerente, código com `%`, prefixo `%` não vaza (4.15 de D2 não vaza pelo 4.1 de D1), outra empresa
- [x] `backend/dre/escopo_test.go` -- valor zero nega
- [x] `backend/escopo_rotas_test.go` -- varredura: toda rota `ler_no_escopo` aplica `escopoDe` ou está na lista sem valor; todo handler que lê valor usa `escopoDe` ou `exigeLer`
- [x] `frontend` -- rotas, menu e tipo da visão por departamento

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...`
- `cd frontend && npm run typecheck && npm run build`

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Conta com departamentos diferentes ao longo dos meses libera todos os lançamentos da conta ao Gerente de um deles | low | defer | só ocorre se o dicionário mudar depois de fechamentos; `deferred-work.md` |
| `?departamento=D%` malformado é ignorado pelo Go (parâmetro descartado) e a visão volta no escopo normal | low | rejeitado | não vaza: continua restrita ao escopo; o teste usa `%25` |
| Varredura é por texto do handler (`escopoDe(`/`exigeLer(`), não prova que o filtro é aplicado | med | rejeitado | os testes de comportamento cobrem cada rota; a varredura só impede rota nova sem escopo |
| Dicionário (GET) mostra o departamento de todas as contas ao Gerente | low | defer | sem valor; decidir com o cliente se o Gerente deve ver só as contas dos seus departamentos |
| DRE.tsx ainda tem o caminho antigo `qLegado` do Gerente | low | defer | morto para o Gerente desde esta história; limpar com a 4.8 |
