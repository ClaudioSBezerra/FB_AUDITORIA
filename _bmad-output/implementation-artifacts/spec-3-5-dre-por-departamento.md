---
title: 'Story 3.5: DRE por departamento'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: 'd18c3bb'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O Controller não vê onde cada área gastou: o DRE é por linha, e o departamento das contas (2.10) não aparece em lugar nenhum nem é congelado no fechamento.

**Approach:** `GET /api/aud/dre/departamentos` agrupa as contas do período por departamento e por grupo (Comercial, Logística, Administrativo), com total e percentual sobre o Faturamento Líquido (3.4). Fechar passa a gravar o departamento de cada conta no snapshot; mês fechado lê dele. Tela "DRE por departamento" na área Administrativo.

## Boundaries & Constraints

**Always:** Migration `012_snapshot_departamento.sql`: `dre_fechamento_contas.departamento TEXT NOT NULL DEFAULT ''` (código do departamento ou ''), sem tabela nova (`limpar_dados_teste` não muda). Departamento resolvido EM LOTE: `dre.CarregarResolvedorDepartamento` lê os vínculos uma vez e usa `MelhorPadrao` (no Fechar e na leitura ao vivo); `DepartamentoDe` fica só para uma conta. Valor da conta com o sinal de `dre_mapa` (`RealizadoPorConta`). Mês fechado: valor e departamento do snapshot, sem lançamentos, mapa nem vínculos; mês aberto ou reaberto, ao vivo. Fechamento sem departamento gravado cai em "Sem departamento". Perfil `gestor` (e qualquer perfil fora de controladoria, auditoria, diretoria): `403` com mensagem, fail closed, registrado em `audit_log` (`dre_departamentos`/`negado`); mesma regra na rota e no handler. "Sem departamento" só aparece a esses perfis (todos sem restrição até o épico 4, quando vira checagem de escopo). Percentual: base é o Faturamento Líquido do mesmo recorte (3.4); sem linha marcada, aviso e percentuais `null`; zero, `null` ("não calculável").

**Never:** Não chamar `DepartamentoDe` em laço. Não reescrever snapshot já gravado. Nenhum departamento ou conta real em arquivo versionado.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Contas ligadas | vínculos exato e por prefixo | cada departamento no seu grupo com contas, total e % | N/A |
| Conta sem departamento | sem vínculo | vai a `sem_departamento` | N/A |
| Gestor | `perfil=gestor` | nada de departamento | `403` + `negado` na trilha |
| Fechar | contas ligadas | `dre_fechamento_contas.departamento` = código ('' se sem) | N/A |
| Mês fechado | vínculos, dicionário e lançamentos mudam depois | valores e departamentos não mudam | N/A |
| Fechamento antigo | `departamento = ''` | contas em "Sem departamento" | N/A |
| Fechamento legado (sem contas) | só linhas | mês fora da visão, com aviso | aviso em `avisos` |
| Acumulado misto | meses fechados e abertos | fechados do snapshot, abertos ao vivo | N/A |
| Departamento removido do cadastro depois do fechamento | código do snapshot sem cadastro | conta cai em "Sem departamento" | N/A |

</frozen-after-approval>

## Code Map

- `backend/migrations/012_snapshot_departamento.sql` -- coluna.
- `backend/dre/dicionario_contas.go` (`ResolvedorDepartamento`, `DescritorConta`), `backend/dre/fechamento.go` (grava o departamento), `backend/dre/departamentos_visao.go` (leitura por mês e montagem da visão), `backend/dre/percentual.go` (`CodigoBase`, extraído do DRE).
- `backend/handlers/dre.go` (`DREDepartamentosHandler`), `backend/main.go` (rota).
- `frontend/src/pages/aud/DREDepartamentos.tsx`, `App.tsx`, `lib/navigation.ts` -- tela em `/empresa/dre-departamentos`.
- Testes: `backend/handlers/dre_departamentos_test.go`, `backend/dre/departamentos_visao_test.go`.

## Tasks & Acceptance

- [x] Migration, resolução em lote, snapshot, endpoint, tela, testes.

**Acceptance Criteria:**
- Given contas ligadas, when abro a visão, then cada departamento aparece no seu grupo com contas, total e %.
- Given gestor, when chama a API, then 403 e nenhum dado.
- Given `go build`, `go vet`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`), when rodam, then passam.

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo; `npm run lint` falha por falta de configuração do ESLint (já registrado em `deferred-work.md`)

## Implementation Notes

- Nome, grupo e descrição da conta vêm do cadastro e do dicionário vivos; o snapshot guarda só o código do departamento (como pedido). Renomear departamento ou trocar o grupo dele muda a apresentação de meses fechados; os valores e a ligação conta-departamento não mudam.
- Só contas com mapa entram (as sem mapa ficam fora do DRE, e o Fechar as recusa). Conta sem lançamento no período não aparece.
- Uma conta pode aparecer em dois departamentos num acumulado se o vínculo mudou entre meses (mês fechado versus aberto).
- O percentual por departamento pode passar de 100 ou ser negativo (a conta de receita ligada a um departamento soma positivo, a de custo com sinal do Mapa); é o valor do Mapa sobre o Faturamento Líquido, sem reinterpretação.
- A visão consulta o DRE inteiro do período (para o Faturamento Líquido) e as contas: custo de duas leituras; aceitável no volume atual.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Grupo e nome do departamento não são congelados | medium | aceito | pedido: coluna só com o código; ponto frágil registrado |
| Descrição da conta de mês fechado vem do dicionário vivo | low | aceito | é rótulo; teste compara valores e departamentos |
| N consultas por conta | high | corrigido | `ResolvedorDepartamento`; teste conta 1 leitura de vínculos para 150 lançamentos |
| `gestor` sem escopo | high | tratado | 403 no handler e na rota, com trilha; testado |
