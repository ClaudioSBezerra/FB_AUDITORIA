---
title: 'Story 1.8: Mês fechado lido do que foi congelado (AD-17)'
type: 'feature'
created: '2026-10-28'
status: 'approved'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O fechamento só congela o valor das linhas, e só o `DREHandler` mensal o lê. O acumulado, o ano anterior, o orçado e o detalhe por conta continuam ao vivo, então mudar o Mapa ou a estrutura depois muda um mês "fechado"; a tela ainda avisa que o detalhe pode incluir carga posterior.

**Approach:** Migration que cria `dre_fechamento_contas` e acrescenta orçado e ano anterior a `dre_fechamento_linhas`, ambas imutáveis por gatilho que respeita `imutavel_salvo_em_teste`, e atualiza `limpar_dados_teste`. Fechar grava linhas e contas. Toda leitura de competência fechada usa só o snapshot do fechamento ativo (1.7); competência aberta continua ao vivo.

## Boundaries & Constraints

**Always:** Valores em `NUMERIC(18,2)`. Snapshot completo (decisão do dono do produto): `dre_fechamento_linhas` e `dre_fechamento_contas` recebem `filial` na chave (consolidado do grupo e cada filial, congelados na mesma transação do `Fechar`), de modo que o filtro de filial em mês fechado também lê só do snapshot. `Fechar` grava linhas (com realizado, orçado e ano anterior, subtotais inclusive, congelados) e contas (conta, linha, realizado, ano anterior) na mesma transação. Competência fechada é lida do snapshot ativo em qualquer papel (realizado, orçado, ano anterior), no mensal, no acumulado (soma mês a mês: mês fechado do snapshot, mês aberto ao vivo) e no detalhe por conta; mês fechado não consulta `lancamentos`, `dre_mapa` nem `dre_linhas`. Competência reaberta (sem ativo) volta ao vivo. O aviso "o detalhe mostra lançamentos posteriores ao fechamento" sai da tela. Orçado vem do que existe hoje (`dre_orcado`, por linha).

**Never:** Não acrescentar colunas de orçado por conta, departamento nem ajustes: chegam nas 2.7, 3.5 e 5.4. Não recalcular snapshot de fechamento antigo nem fazer `UPDATE` em linha de fechamento já gravada; fechamentos anteriores a esta migration não têm contas e continuam lidos como hoje (apenas linhas), sem inventar dados. Não mudar a regra de fechar da 1.7. Não editar a migration quando aplicada. Produção só recebe por push.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Mês fechado | ativo com snapshot | DRE e detalhe do snapshot; mudar Mapa não altera | N/A |
| Mês aberto ou reaberto | sem ativo | ao vivo, como hoje | N/A |
| Acumulado misto | meses fechados e abertos | soma do snapshot e do vivo, por mês | N/A |
| Fechamento antigo | sem `dre_fechamento_contas` | linhas como hoje; detalhe sem contas, com aviso curto | N/A |
| Alterar snapshot | `UPDATE` ou `DELETE` nas tabelas congeladas | recusado | exceção do gatilho, salvo teste com limpeza |

</frozen-after-approval>

## Open Questions

_Nenhuma._

## Code Map

- `backend/migrations/003_dre.sql` -- `dre_fechamento_linhas` (`fechamento_id`, `linha_codigo`, `descricao`, `nivel`, `ordem`, `valor`; cascata no fechamento). Nova migration (próximo número livre): `CREATE TABLE IF NOT EXISTS dre_fechamento_contas`, `ADD COLUMN IF NOT EXISTS` de orçado e ano anterior, gatilhos `BEFORE UPDATE OR DELETE` que chamam `imutavel_salvo_em_teste`, e nova versão de `auditoria.limpar_dados_teste` (CREATE OR REPLACE da 1.2). **Gatilho de imutabilidade e cascata: o `DELETE` do fechamento dispara os gatilhos das tabelas filhas; a limpeza de teste precisa liberá-los.**
- `backend/dre/fechamento.go` (1.5, 1.7) -- `Fechar` grava as contas e as colunas novas; `backend/dre/realizado.go` -- `Realizado` por mês; novo `backend/dre/leitura.go` -- `Leitor` que decide por mês: ativo -> snapshot, senão ao vivo.
- `backend/handlers/dre.go` -- `DREHandler` (bloco "Período congelado", só no mensal e por `filial`) e `DRELancamentosHandler` (lista `lancamentos`, a única origem do detalhe): ambos passam pelo `Leitor`. Para mês fechado o detalhe devolve as contas do snapshot no mesmo JSON (`id` 0, sem documento nem histórico, `origem` de snapshot).
- `frontend/src/pages/aud/DRE.tsx` -- remover o aviso do `Dialog` (linhas ~147-151); o detalhe por conta aproveita a mesma tabela.
- `backend/handlers/dre_fechamento_test.go`, `backend/dre/leitura_test.go` -- novos; `backend/internal/testdb` da 1.1.
- Não mudar: `DREMapaHandler`, `DREOrcadoHandler`, a regra de fechar e reabrir.

## Tasks & Acceptance

**Execution:**
- [ ] `backend/migrations/00N_fechamento_snapshot.sql` -- tabela, colunas, gatilhos, `limpar_dados_teste` atualizada -- AD-17 e AD-18
- [ ] `backend/dre/fechamento.go` -- gravar linhas e contas congeladas -- snapshot completo
- [ ] `backend/dre/leitura.go` e `backend/handlers/dre.go` -- leitura por mês do snapshot ou ao vivo; detalhe por conta -- regra única
- [ ] `frontend/src/pages/aud/DRE.tsx` -- sem o aviso -- coerência
- [ ] testes -- fechar, mudar o Mapa, ler e ver o mesmo resultado; acumulado misto; reaberto volta ao vivo; gatilho recusa `UPDATE` e `DELETE`; a limpeza de teste apaga fechamentos e as filhas; fechamento antigo sem contas -- prova

**Acceptance Criteria:**
- Dado a migration, então existe `dre_fechamento_contas` (conta, linha, realizado, ano anterior) e `dre_fechamento_linhas` ganha orçado e ano anterior, ambas imutáveis por gatilho que respeita `imutavel_salvo_em_teste` e incluídas em `limpar_dados_teste`.
- Dado Fechar, quando grava o snapshot, então grava linhas e contas; as colunas de orçado por conta, departamento e ajustes são acrescentadas pelas histórias 2.7, 3.5 e 5.4.
- Dado competência fechada, quando o DRE (mensal, acumulado, detalhe por conta) é consultado, então lê só do snapshot, e mudar Mapa ou estrutura depois não altera o resultado.
- Dado competência aberta, quando o DRE é consultado, então lê ao vivo como hoje, e o aviso da tela "o detalhe mostra lançamentos posteriores ao fechamento" deixa de existir.

## Implementation Notes

Tamanho do plano: cerca de 2050 tokens (estimativa), acima de 1600, porque a história cruza banco, backend e testes; sem divisão proposta.

## Design Notes

Os subtotais são congelados como valor, e não recalculados pela estrutura atual; por isso o acumulado soma linha a linha (subtotal com subtotal) em vez de recomputar a partir das contas. O drill-down de mês fechado perde o nível de lançamento: era um recurso que o requisito do fechamento chama de dado posterior ao fechamento, e a fonte atual já é agregada por mês.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -p 1 ./...` -- expected: sem erro
- `cd frontend && npm run typecheck` -- expected: só o erro anterior em `ResetPassword.tsx(13,9)`, que não é desta história (`npm run lint` não roda)
