---
title: 'Story 3.9: Detalhe até o lançamento em mês fechado'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '475ce61'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O gerente não desce do grupo à conta e da conta ao lançamento em mês fechado: a 1.8 deixou o detalhe fechado só até a conta, e o detalhe por lançamento só aceita `?filial=`.

**Approach:** Dois endpoints novos: `GET /api/aud/dre/contas` (contas de uma linha ou subtotal, com o valor do DRE) e `GET /api/aud/dre/contas/lancamentos` (lançamentos da conta). Ambos respeitam o recorte `filial | agrupamento | grupo` e o período mensal ou acumulado. Em mês fechado o TOTAL da conta é o do snapshot; os lançamentos são lidos ao vivo de `lancamentos` (a guarda de período, 1.6, impede escrita em mês fechado). Se a soma ao vivo de um mês fechado divergir do congelado, a resposta traz `divergente`, `aviso_divergencia` e `divergencias`, em vez de esconder.

## Boundaries & Constraints

**Always:** Sem migration: `lancamentos` não tem data de pagamento nem fornecedor; usa-se o que existe (`competencia`, `tipo_doc`, `doc_ref`, `historico`, `filial`) e o histórico é mostrado como veio. Perfil `gestor` (e qualquer fora de controladoria, auditoria, diretoria) recebe `403` nos dois endpoints, na rota e no handler, registrado em `audit_log` (`dre_contas`/`dre_lancamentos`, `negado`), como na 3.5; vira escopo por departamento na 4.7. Acesso a contas e lançamentos grava `visualizar` na trilha. Paginação: `limite` (padrão 200, máximo 1000) e `offset`; `total_lancamentos` e `soma_lancamentos` valem para todo o recorte, não só a página. Valores no sinal do Mapa (como o DRE). Conta fora do Mapa: `404`. Tela do DRE e do DRE por departamento: clicar na linha abre as contas, clicar na conta abre os lançamentos (data, histórico, nota, filial, valor).

**Never:** Não alterar `DRELancamentosHandler` (segue servindo o gestor na tela do DRE) nem o snapshot. Nenhum dado real em arquivo versionado.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Mês aberto | lançamentos da conta | total = soma ao vivo, lançamentos listados | N/A |
| Mês fechado igual | ao vivo = snapshot | total do snapshot, `divergente=false` | N/A |
| Mês fechado divergente | lançamento novo por fora da guarda | total do snapshot; `divergente=true` e aviso com os meses | N/A |
| Mês reaberto | reaberto e atualizado | total ao vivo, lançamentos novos | N/A |
| Recorte | agrupamento ou grupo | soma das filiais do ERP do recorte | `400` combinação, `404` agrupamento |
| Fechamento legado | só linhas, sem contas | lançamentos ao vivo, sem valor congelado | aviso em `avisos` |
| Gestor | `perfil=gestor` | nada | `403` + `negado` na trilha |
| Conta sem Mapa | conta fora do DRE | nada | `404` |

</frozen-after-approval>

## Code Map

- `backend/dre/detalhe.go` -- `Leitor.ContasDaLinha` (reusa `ContasPorDepartamentoFiliais`, sem expor departamento) e `Leitor.DetalheConta` (total por mês, divergência, página).
- `backend/handlers/dre_detalhe.go` -- `DREContasHandler`, `DREContaLancamentosHandler`, `exigeDetalhe`; `backend/main.go` -- rotas.
- `frontend/src/pages/aud/DetalheDRE.tsx` (diálogos), `DRE.tsx`, `DREDepartamentos.tsx`.
- Testes: `backend/handlers/dre_detalhe_test.go`.

## Tasks & Acceptance

- [x] Funções de leitura, endpoints, rotas, telas, testes.

**Acceptance Criteria:**
- Given um mês fechado, when abro a conta, then vejo os lançamentos e o total do snapshot; se a soma divergir, a resposta e a tela avisam.
- Given um mês reaberto e atualizado, when abro a conta, then vejo os lançamentos novos.
- Given gestor, when chama as rotas, then `403`.
- Given `go build`, `go vet`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`), when rodam, then passam.

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo; build passa

## Implementation Notes

- Colunas: `lancamentos` não tem fornecedor nem data de pagamento (migration 003). "Data" é a competência (AAAA-MM) e "fornecedor" é o histórico; documento é `tipo_doc` + `doc_ref`. Migration para data de pagamento e fornecedor fica para a fonte Oracle (`deferred-work.md`).
- A divergência é avaliada por mês fechado, na soma de todas as filiais do recorte (a leitura do snapshot usa a mesma chave de filial do DRE). Mês de fechamento legado não tem valor congelado da conta: sem checagem, só aviso.
- A página de lançamentos é por `competencia, id`; `soma_lancamentos` é a soma ao vivo do período todo, para o usuário comparar com `total`.
- O gestor continua vendo o detalhe antigo (`/api/aud/dre/lancamentos`) na tela do DRE, que não foi alterado; fechar isso é da 4.7.
- A lista de contas de uma linha custa uma leitura por departamento (resolução em lote); aceitável no volume atual.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Sem fornecedor nem data de pagamento em `lancamentos` | medium | defer | `deferred-work.md`; usa histórico, documento e competência |
| Gestor ainda alcança `/api/aud/dre/lancamentos` | medium | defer | endpoint antigo intocado; 4.7 |
| Divergência escondida pelo total do snapshot | high | tratado | `divergente` + `aviso_divergencia`; testado |
| Total e soma só da página | medium | corrigido | `total_lancamentos`/`soma_lancamentos` do recorte; teste de paginação |
