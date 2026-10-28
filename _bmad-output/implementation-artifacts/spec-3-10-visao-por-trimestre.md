---
title: 'Story 3.10: Visão por trimestre'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '4b0c596'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** A planilha do cliente não tem a visão por trimestre, e o gerente soma meses na mão para comparar T1 a T4.

**Approach:** `GET /api/aud/dre/trimestres?ano=AAAA` (+ `filial | agrupamento | grupo=1`) devolve, para cada linha do DRE, as colunas T1, T2, T3, T4 e Ano com realizado, orçado, ano anterior e percentual sobre o Faturamento Líquido. Cada trimestre é a soma de três meses lidos por `Leitor.LerFiliais` (mês fechado do snapshot, mês aberto ao vivo), sem reimplementar leitura. A tela do DRE ganha a alternância "Trimestres" e um seletor de recorte (Grupo, agrupamento, filial).

## Boundaries & Constraints

**Always:** Trimestre = 3 meses (T1 jan-mar etc.); `parcial` é verdadeiro se algum mês não tem fechamento ativo, e `meses_fechados` lista os fechados ("AAAA-MM"). Trimestre que começa depois do mês corrente vem `vazio` (valores zerados, sem erro); a coluna Ano é a soma das quatro, então Ano = T1+T2+T3+T4 exatamente (valores arredondados a centavos antes da soma). Subtotal é a soma dos subtotais mensais, que já são a soma das linhas do mês. O percentual de cada coluna usa o Faturamento Líquido da própria coluna, com a estrutura congelada quando todos os meses dela estão no snapshot completo (`pct_status` e linha-base por coluna). Recorte: mesmas regras de exclusão do DRE (`resolverRecorte`). `gestor` e perfis fora de controladoria, auditoria, diretoria: `403` na rota e no handler, registrado (`dre_trimestres`/`negado`), até o escopo por departamento (4.7). Trilha `visualizar`.

**Never:** Não reimplementar leitura de mês fechado nem de mês aberto. Não mudar `Leitor`, snapshot, Fechar. Nenhum dado real em arquivo versionado.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Trimestre aberto | meses ao vivo | soma dos três meses, `parcial=true` | N/A |
| Trimestre fechado | três meses com fechamento | valores do snapshot, `parcial=false`, três `meses_fechados`; Mapa/estrutura/lançamentos posteriores não mudam | N/A |
| Parcial | um ou dois meses fechados | `parcial=true`, lista os fechados | N/A |
| Trimestre futuro | começa depois do mês corrente | `vazio=true`, zeros | N/A |
| Recorte | filial, agrupamento ou grupo | mesmas regras do DRE | `400`, `404`, `409` |
| Faturamento Líquido zero | base do trimestre zero | percentuais `null`, `pct_status=zero` | N/A |
| Gestor | `perfil=gestor` | nada | `403` + `negado` |

</frozen-after-approval>

## Code Map

- `backend/dre/trimestres.go` -- `Leitor.LerTrimestres`, `ColunaTrimestre`.
- `backend/handlers/dre_detalhe.go` -- `DRETrimestresHandler`; `backend/main.go` -- rota.
- `frontend/src/pages/aud/Recorte.tsx` (`SeletorRecorte`, `useRecorte`), `DRETrimestres.tsx`, `DRE.tsx`, `DREDepartamentos.tsx`.
- Testes: `backend/handlers/dre_trimestres_test.go`.

## Tasks & Acceptance

- [x] Leitura por trimestre, endpoint, rota, aba Trimestres, seletor de recorte (DRE e DRE por departamento), testes.

**Acceptance Criteria:**
- Given um ano e um recorte, when escolho "Trimestres", then vejo T1 a T4 e Ano com realizado, orçado, ano anterior e %.
- Given um trimestre com mês aberto, when a tela o mostra, then marca "parcial" e diz os meses fechados (tooltip).
- Given um trimestre com os três meses fechados, when Mapa ou estrutura mudam, then o valor não muda.
- Given `go build`, `go vet`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`), when rodam, then passam.

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo; build passa

## Implementation Notes

- Decisão: o Ano soma só os trimestres iniciados; o orçado de trimestre futuro não entra (o pedido foi "vazio"). Se o cliente quiser ver o orçado do ano inteiro, é uma mudança de leitura do trimestre futuro.
- Linhas exibidas: a estrutura congelada quando o ano inteiro está fechado; senão a viva, mais as linhas que só existem no congelado de algum trimestre. A estrutura congelada não guarda `soma_de`, então subtotais não são recalculados: vêm da soma dos subtotais mensais, arredondados a centavos.
- Seletor de recorte: Grupo (`grupo=1`), agrupamentos (de `/api/aud/agrupamentos-filiais`, só se o perfil puder ler) e filiais do ERP (de `/api/aud/dre/filiais`). Filial gerencial isolada não é opção: o recorte do backend não tem esse parâmetro (`deferred-work.md`). O gestor vê só Grupo e filial e continua no detalhe antigo.
- Custo: quatro leituras (uma por trimestre) por filial do recorte; aceitável no volume atual.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Subtotal do trimestre fechado poderia mudar com a estrutura viva | high | tratado | soma dos subtotais do snapshot; teste muda Mapa e `soma_de` e compara T1 |
| Orçado de trimestre futuro fora do Ano | low | aceito | pedido de coluna vazia; nota acima |
| Filial gerencial isolada sem opção no seletor | low | defer | `deferred-work.md` |
| `gestor` sem escopo | high | tratado | 403 com trilha; 4.7 abre o escopo |
