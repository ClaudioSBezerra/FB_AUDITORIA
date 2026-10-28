---
title: 'Story 2.15: Agrupamentos de filiais gerenciais'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: 'b4e1397'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Na planilha do cliente há visões nomeadas que somam algumas filiais gerenciais (Balcões, Distribuição) e a visão `Grupo` (todas as filiais). O DRE só filtra por uma filial do ERP.

**Approach:** Cadastro de agrupamentos (nome e filiais gerenciais membros, uma filial gerencial em vários agrupamentos), resolvedor `agrupamento -> filiais gerenciais -> filiais do ERP` (mapa da 2.14) e `?agrupamento=<codigo>` e `?grupo=1` nos endpoints do DRE, somando as filiais pedidas sem mudar a lógica de leitura nem o snapshot. A visão Grupo não tem cadastro.

## Boundaries & Constraints

**Always:** Migration `013_agrupamentos_filiais.sql`: `auditoria.agrupamentos_filiais (id, empresa_id, codigo, nome, UNIQUE(empresa_id,codigo), UNIQUE(id,empresa_id))` e `agrupamentos_filiais_membros (empresa_id, agrupamento_id, filial_gerencial_id)`, PK composta, FKs compostas (mesma empresa; membros caem com o agrupamento, a filial gerencial não cai com o membro); nova `limpar_dados_teste` (cópia da 010, membros antes dos agrupamentos, antes das filiais gerenciais, contagens no JSON). Mensagens de erro sem `already exists` nem `duplicate`. Serviço `dre.Agrupamentos` (`Listar`, `Salvar`, `Excluir`) com trilha na mesma transação (entidade `agrupamento_filiais`); escrita só `controladoria` (`403` para os demais); `GET` para os perfis da área `configuracoes`; rota `/api/aud/agrupamentos-filiais`. Resolvedor `dre.FiliaisDoAgrupamento(ctx, q, empresa, codigo)`. Visão Grupo: `dre.FilialGrupo` (filtro `""`, que a leitura já entende como todas). Código de agrupamento não colide com código de filial gerencial (nem o inverso) nem com a palavra reservada `GRUPO` (sem diferenciar maiúsculas): `409`. Membro inexistente na empresa: `400`. Excluir filial gerencial que é membro: `409` citando o agrupamento. DRE (`GET /api/aud/dre` e `/api/aud/dre/departamentos`): `?agrupamento=` soma as filiais do ERP do agrupamento (`LerFiliais`, `ContasPorDepartamentoFiliais`), `?grupo=1` equivale a todas; no máximo um entre `filial`, `agrupamento`, `grupo` (`400`); agrupamento inexistente `404`; sem filial do ERP `409`.

**Never:** Código real de filial ou agrupamento em arquivo versionado (use `G1`, `A1`). Não mudar `Leitor.Ler`, o snapshot, o `Fechar` nem a ingestão. Não editar migration já aplicada.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Criar | controladoria, `A1` com `G1`,`G2` | grava, trilha registra | N/A |
| Filial em dois agrupamentos | `G2` em `A1` e `A2` | permitido | N/A |
| Resolver | `A1` | filiais gerenciais e ERP de `G1`+`G2` | inexistente: `ErrAgrupamentoNaoEncontrado` |
| Código colide | `A1` igual a `G1`, ou `GRUPO` | nada gravado | `409` |
| Membro inexistente | `ZZ` | nada gravado (atômico) | `400` |
| Perfil sem escrita | diretoria `POST` | `403`, nada gravado | N/A |
| DRE por agrupamento | `?agrupamento=A1` | soma das filiais do ERP de `A1` | `404`, `409` (vazio) |
| DRE do grupo | `?grupo=1` | soma de todas as filiais | `400` se combinado |
| Mês fechado | fechado e lançamento novo depois | soma do snapshot por filial do ERP; não muda | N/A |
| Excluir membro | `G1` está em `A1` | nada muda | `409` |

</frozen-after-approval>

## Code Map

- `backend/migrations/013_agrupamentos_filiais.sql` -- tabelas e `limpar_dados_teste` (base: `010_dicionario_contas.sql`).
- `backend/dre/agrupamentos.go` -- serviço, resolvedor, `LerFiliais`, `ContasPorDepartamentoFiliais`, `Ativo.OrigemFiliais`; padrão de `backend/dre/filiais.go`.
- `backend/dre/filiais.go` -- `Salvar` recusa código de agrupamento e `GRUPO`; `Excluir` recusa membro de agrupamento.
- `backend/handlers/agrupamentos_filiais.go` -- CRUD fino (padrão de `filiais_gerenciais.go`); `backend/handlers/recorte_filial.go` -- `resolverRecorte` (parâmetros e erros); `backend/handlers/dre.go` -- os dois endpoints usam o recorte; `backend/main.go` -- rota.
- `frontend/src/pages/aud/AdmAgrupamentos.tsx`, `App.tsx`, `lib/navigation.ts` -- aba "Agrupamentos" em Configurações, seleção por checkbox.
- Testes: `backend/dre/agrupamentos_test.go`, `backend/handlers/agrupamentos_filiais_test.go` (reusam `testdb`, `reqFilial`, `estruturaPct`, `cenarioDep`).

## Tasks & Acceptance

**Execution:**
- [x] Migration, serviço, resolvedor, rota e handler
- [x] `?agrupamento=` e `?grupo=1` em `/api/aud/dre` e `/api/aud/dre/departamentos`
- [x] Testes (matriz, isolamento por empresa, snapshot, limpeza, trilha)
- [x] Aba "Agrupamentos"

**Acceptance Criteria:**
- Given um agrupamento, when o resolvedor recebe o código, then devolve as filiais gerenciais e as filiais do ERP.
- Given `?agrupamento=A1`, when o DRE é lido, then soma as filiais do ERP do agrupamento, no vivo e no snapshot.
- Given `go build`, `go vet`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`), when rodam, then passam.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- expected: passam
- `cd frontend && npm run typecheck && npm run build` -- expected: só o erro anterior em `ResetPassword.tsx(13,9)`; build passa

## Implementation Notes

Decisões: palavra reservada `GRUPO` (o parâmetro `?grupo=1` é separado do código, então a reserva só evita ambiguidade na tela). `?grupo=1` usa o filtro `""` já existente (soma de tudo, inclusive filial solta e lançamento sem cadastro); por isso não há `TodasAsFiliais`. Agrupamento soma só as filiais do ERP ligadas às suas filiais gerenciais (filial solta fora do mapa não entra). Agrupamento sem filial do ERP responde `409` em vez de zeros, para não parecer "sem movimento". `LerFiliais` chama `Ler` uma vez por filial do ERP e soma (os subtotais são aditivos); a estrutura congelada usa a primeira filial porque o snapshot completo vale para todas.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Uma consulta de leitura por filial do ERP (N chamadas de `Ler`) | low | defer | `deferred-work.md`; mesmo custo do resolvedor da 2.14 |
| Seletor de filial/agrupamento/grupo na tela do DRE | low | defer | `deferred-work.md`; só o backend nesta história |
| Fechamento legado por filial misturado com agrupamento | low | aceito | `OrigemFiliais` devolve Legado se alguma filial for legada; sem contas por departamento |
| Excluir filial gerencial removia a ligação em silêncio | medium | patch | FK sem cascata e `409` com o agrupamento |
