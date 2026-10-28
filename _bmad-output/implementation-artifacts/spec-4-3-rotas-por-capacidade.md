---
title: 'Story 4.3: Migrar as rotas para capacidades (AD-11)'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '5f313a4'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/spec-4-2-perfil-analista-matriz.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Cada rota ainda decide por lista de perfis (no `main.go` e dentro dos handlers), e o analista da 4.2 não alcança nenhuma.

**Approach:** Toda rota `/api/aud/*` declara uma capacidade; a matriz (`perfis.go`) decide. Os perfis que já existiam mantêm exatamente o comportamento de hoje; o analista entra onde a matriz o inclui. A assinatura com lista de perfis (`Tem`, `perfis ...string`) é removida.

## Boundaries & Constraints

**Always:** `withAud(f, area, caps...)` e `AuditoriaAuthMiddleware(db, next, area, caps...)`: sem `caps` = só a área; `caps[0]` leitura (todos os métodos se for a única); `caps[1]` escrita (POST/PUT/DELETE/PATCH). Os handlers continuam checando a capacidade por dentro (`c.Pode`), como antes checavam o perfil: defesa em profundidade e `403` com `negado` na trilha onde já havia (`atualizacao_dados`, `dre_departamentos`, `dre_contas`, `dre_lancamentos`, `dre_trimestres`). Gestor não muda: as visões por departamento e o detalhe seguem fail-closed (exigem `ler`, que o gestor não tem); o escopo por departamento fica para a 4.7. Acompanhamento continua só por área. Rotas sem área (`me`, `ambiente/limpar`, `ambiente/virar`) seguem sem capacidade (o admin é checado no handler).

**Never:** Não mudar expectativa dos testes existentes. Não dar ao analista fechar, reabrir, usuários nem trilha.

## Mapeamento rota → capacidade (leitura / escrita)

| Rota | Leitura | Escrita | Analista ganha? |
|---|---|---|---|
| `/api/aud/usuarios` | gerir_usuarios | (mesma) | não |
| `/api/aud/audit-log` | ler_trilha | (mesma) | não |
| `/api/aud/filiais-gerenciais`, `agrupamentos-filiais`, `departamentos` | ler_no_escopo | editar_estrutura | sim (escrita) |
| `/api/aud/dicionario` (GET) | ler_no_escopo | (mesma) | lê, como todos |
| `/api/aud/dicionario/carga`, `dicionario/conta` | editar_dicionario | (mesma) | sim |
| `/api/aud/dre`, `dre/filiais`, `dre/linhas`, `dre/lancamentos` | ler_no_escopo | (mesma) | lê, como todos |
| `/api/aud/dre/linhas/papel` | editar_estrutura | (mesma) | sim |
| `/api/aud/dre/departamentos`, `dre/trimestres`, `dre/contas`, `dre/contas/lancamentos` | ler | (mesma) | lê (tem `ler`); gestor segue 403 |
| `/api/aud/dre/fechamentos` | ler_no_escopo | fechar | não (403 no POST) |
| `/api/aud/dre/fechamentos/reabrir` | reabrir | (mesma) | não (403) |
| `/api/aud/dre/mapa` | ler_no_escopo | editar_mapa | sim (escrita) |
| `/api/aud/dre/orcado`, `dados/atualizar`, `dre/lancamentos/importar` | carregar | (mesma) | sim |
| `/api/aud/acompanhamento`, `me`, `ambiente/limpar`, `ambiente/virar` | só área / handler | | n/a |

Rotas que o analista ganha, exatamente: importar lançamentos, atualizar dados, orçado, escrita do Mapa, papel da linha, escrita de departamentos / filiais gerenciais / agrupamentos, carga e edição de conta do dicionário, mais a leitura das visões `ler` (departamentos, trimestres, contas, lançamentos).

## Decisões

- `dicionario/carga` e `dicionario/conta` usam `editar_dicionario` (e não `carregar`): é o cadastro do dicionário, não a carga de lançamentos; a matriz dá as duas ao analista, então o efeito é o mesmo.
- O `403` das rotas protegidas só pelo middleware (já era assim para `dados/atualizar`, `dre/departamentos`, `dre/trimestres` etc.) não grava `negado` na trilha: o middleware nega antes do handler. Só o caminho direto ao handler (testes) grava. Comportamento igual ao de hoje; registrado como ponto frágil.
- Frontend: Analista entra nas abas e rotas de Carga, Filiais, Agrupamentos, Departamentos, Dicionário, Mapa, DRE por departamento; `editar` das telas de cadastro inclui o analista; `podeFechar` não. Usuários e Trilha ficam sem analista. O backend é quem decide; o menu só reflete.

## Tasks & Acceptance

- [x] `backend/main.go` -- `withAud` por capacidade; todas as rotas migradas
- [x] `backend/handlers/*.go` -- `c.Tem(...)` trocado por `c.Pode(cap)`; `Tem` e a assinatura legada removidos
- [x] `backend/rotas_capacidades_test.go` -- (1) falha se rota usa lista de perfis ou fica sem capacidade; (2) toda rota de escrita recusa diretoria, auditoria e gestor; (3) tabela do comportamento de hoje para os quatro perfis antigos e o que o analista ganha; (4) analista carrega/edita e recebe 403 em fechar e reabrir
- [x] `backend/handlers/perfis_test.go` -- handlers checam a capacidade
- [x] `frontend` -- analista nas abas, rotas e botões de edição

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...`
- `cd frontend && npm run typecheck && npm run build`

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| `negado` não é gravado quando o middleware nega antes do handler | low | defer | igual ao comportamento anterior; avaliar log no middleware numa história de trilha |
| Teste de rotas depende do formato de uma linha por rota em `main.go` | low | rejeitado | falha alto (`Fatalf`) se a linha não for lida |
| Frontend ainda lista perfis por aba em vez de ler `capacidades` de `/api/aud/me` | low | defer | `/me` já devolve `capacidades`; migrar o menu é melhoria futura |
| Regra "fail-closed do gestor" agora é `ler` | ok | rejeitado | `gestor` não tem `ler`; a 4.7 troca por escopo |
