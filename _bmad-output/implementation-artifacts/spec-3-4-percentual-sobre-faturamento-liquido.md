---
title: 'Story 3.4: DRE com percentual sobre o Faturamento Líquido'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '02e56a5'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O DRE mostra valores, mas não quanto cada linha pesa sobre o Faturamento Líquido, e nada identifica qual linha é o Faturamento Líquido.

**Approach:** Papel configurável na linha (`dre_linhas.papel = 'faturamento_liquido'`, uma por empresa), marcado em Mapa do DRE. `GET /api/aud/dre` devolve `pct_faturamento` por linha, calculado do realizado exibido (vivo ou snapshot); o papel é congelado no fechamento junto com a estrutura. A tela ganha a coluna "% Fat. Líq.".

## Boundaries & Constraints

**Always:** Migration `011_papel_linha.sql`: `dre_linhas.papel TEXT NULL` (CHECK em `faturamento_liquido`), índice único parcial `(empresa_id, papel) WHERE papel IS NOT NULL`, e `dre_fechamento_linhas.papel TEXT NULL`. Nenhum código de linha fixo no programa. Percentual = valor / Faturamento Líquido x 100, calculado dos valores já exibidos (no mês fechado, do snapshot). Subtotal é a soma exata das linhas: `AplicarSubtotais` arredonda a soma de valores de 2 casas para centavos (elimina o resíduo binário de 0,1+0,2). `PUT /api/aud/dre/linhas/papel {codigo, papel}` só `controladoria`; troca a marca na mesma transação e grava `audit_log` (`writeAuditLogTx`). Resposta do DRE traz `pct_status` (`ok` | `sem_papel` | `zero`), `faturamento_liquido_linha` e `aviso_faturamento`.

**Never:** Não semear o papel na estrutura padrão (não se adivinha qual linha é o Faturamento Líquido). Não reescrever snapshot já gravado. Não calcular percentual sem a linha marcada.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Normal | linha marcada, FL > 0 | cada linha traz `pct_faturamento`; a linha-base dá 100 | N/A |
| FL zero | linha marcada, FL = 0 | `pct_status = zero`, percentuais `null` ("não calculável") | N/A |
| Sem papel | nenhuma linha marcada | `pct_status = sem_papel`, `aviso_faturamento`, percentuais `null` | tela avisa |
| Subtotal com centavos | RB 0,10 + 0,20; DED 0,07 | FL = 0,23 exato | N/A |
| Várias UFs na mesma filial | várias linhas de lançamento, mesma filial e conta | a filial soma todas | N/A |
| Acumulado / filial / grupo | `visao=acumulada`, `filial=` | percentual sobre o FL do próprio recorte | N/A |
| Mês fechado | marca e lançamentos mudam depois | valores e percentuais do snapshot não mudam | N/A |
| Fechamento anterior à 3.4 | snapshot sem papel | usa a marca viva se a linha existir no snapshot | sem marca: `sem_papel` |
| Trocar a marca | marcar outra linha | a anterior é desmarcada; trilha registra | perfil sem escrita 403; papel inválido 400; linha inexistente 404 |

</frozen-after-approval>

## Code Map

- `backend/migrations/011_papel_linha.sql` -- colunas e índice parcial.
- `backend/dre/realizado.go` (`Linha.Papel`, arredondamento em `AplicarSubtotais`), `backend/dre/percentual.go` (cálculo), `backend/dre/leitura.go` (estrutura congelada com papel), `backend/dre/fechamento.go` (grava o papel).
- `backend/handlers/dre.go` (`pct_faturamento`, `DRELinhaPapelHandler`), `backend/main.go` (rota).
- `frontend/src/pages/aud/DRE.tsx` (coluna e aviso), `DREMapa.tsx` (marcar a linha).
- Testes: `backend/handlers/dre_percentual_test.go`.

## Tasks & Acceptance

- [x] Migration, cálculo, snapshot, endpoint do papel, tela, testes.

**Acceptance Criteria:**
- Given a tela do DRE, when aberta, then cada linha mostra o percentual (ou "não calculável" / aviso).
- Given `go build`, `go vet`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`), when rodam, then passam.

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo; `npm run lint` falha por falta de configuração do ESLint (já registrado em `deferred-work.md`)

## Implementation Notes

- Não existia "papel": `dre_linhas` só tem `tipo`. Marcar pela tela do Mapa (link na estrutura) ou pelo endpoint; a 3.1 (estrutura editável) pode absorver a marcação depois.
- Lançamento não tem UF: "várias UFs na mesma filial" são várias linhas de lançamento com a mesma filial, somadas pelo `GROUP BY conta`; a unificação de filiais por código de gerencial é da 2.14.
- Percentual de orçado e ano anterior não faz parte da história; só o realizado.
- Percentual não é arredondado no backend (a tela formata com `pctFmt`).

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Arredondar subtotal muda a aritmética compartilhada (orçado e ano anterior) | low | aceito | entradas e snapshot já são NUMERIC(18,2); só remove o resíduo binário |
| Fechamento anterior à 3.4 não tem papel gravado | medium | tratado | usa a marca viva se a linha existir na estrutura congelada; testado |
| Papel não é semeado no DRE padrão | low | aceito | empresa nova vê o aviso até a controladoria marcar |
