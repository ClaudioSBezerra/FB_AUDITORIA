---
title: 'Story 1.6: Guarda de período e bloqueio de escrita em mês fechado (FR-11, AD-8)'
type: 'feature'
created: '2026-10-28'
status: 'ready-for-dev'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Hoje a importação aceita carga em mês fechado, então o detalhe passa a mostrar dado posterior ao fechamento, e nada impede uma carga de entrar entre o cálculo do snapshot e a confirmação do fechamento.

**Approach:** `dre.Periodo.Guardar(tx, empresa, competência)` toma `pg_advisory_xact_lock(dre.ChaveLock(...))`, confere se existe fechamento da competência e recusa se existir. O Ingestor chama `Guardar` antes de qualquer escrita em `lancamentos`, e `Fechar` toma o mesmo lock. Fechar e importar na mesma competência passam a ser serializados.

## Boundaries & Constraints

**Always:** Nesta história "fechado" significa que existe linha em `dre_fechamentos` para `(empresa, competência)`, de qualquer filial ou versão; a reabertura chega na 1.7. As transações usam `READ COMMITTED` explícito (`BeginTx` com `sql.LevelReadCommitted`). `ChaveLock` é a única função de chave de lock (hash estável de empresa e competência para `bigint`), usada por `Guardar` e `Fechar`. O lock é tomado antes da conferência. Um arquivo com várias competências trava todas em ordem crescente (evita impasse) e, se alguma estiver fechada, recusa o arquivo inteiro com `409`, listando os escopos recusados (competência e filial) e dizendo que é preciso reabrir o fechamento; nada é gravado. Competência aberta importa normalmente. O teste da 1.1 que registrava "aceita em mês fechado" é invertido.

**Never:** Não cobrir `dre_orcado` (legado, sai no Épico 2) nem ajustes (Épico 5) nesta história. Não implementar reabertura nem a noção de fechamento "ativo" (1.7). Não aplicar `Guardar` a leitura. Não usar `SERIALIZABLE` nem lock de tabela. Não mudar o contrato de sucesso da importação.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Mês aberto | competência sem fechamento | importa | N/A |
| Mês fechado | competência com fechamento | nada gravado | `409` com a lista de escopos e o pedido de reabrir |
| Arquivo misto | uma competência aberta e uma fechada | nada gravado | `409` |
| Corrida | fechar e importar a mesma competência juntos | a carga entra antes do snapshot, ou é recusada depois; nunca fica dentro dos lançamentos e fora do snapshot | o perdedor espera o lock |

</frozen-after-approval>

## Code Map

- `backend/dre/` (1.4 e 1.5) -- `Ingestor.Importar` (grava lançamentos: ponto da guarda) e `Fechamento.Fechar` (precisa do mesmo lock e do mesmo isolamento).
- Novo `backend/dre/periodo.go` -- `Periodo.Guardar(ctx, tx, empresa, competência) error`, erro tipado `ErrPeriodoFechado` com os escopos; `ChaveLock(empresa string, competência time.Time) int64`.
- `backend/handlers/dre_import.go` -- traduz `ErrPeriodoFechado` em `409`; texto simples, como as demais respostas.
- `backend/handlers/dre_import_test.go` (1.1) -- inverter o caso "importar em mês fechado aceita" e retirar a marca "muda na 1.6".
- `backend/dre/periodo_test.go` -- novos: guarda aberta e fechada; chave estável; bloqueio real (uma transação segura o lock com `Guardar`, uma goroutine chama `Fechar`, e o teste confere que ela só termina depois do `Commit`); corrida repetida N vezes entre `Fechar` e `Importar`, conferindo o invariante da matriz.
- `backend/arquitetura_test.go` (1.4) -- acrescentar: toda função de `backend/dre` que escreve em `lancamentos` chama `Guardar` (análise por `go/parser`, não por regex).
- `backend/handlers/dre.go` -- `DREOrcadoHandler` continua escrevendo `dre_orcado` sem guarda; é legado e sai no Épico 2 (AR-3). Não mudar.
- `backend/main.go` -- nada: a transação e o isolamento ficam dentro de `dre`.

## Tasks & Acceptance

**Execution:**
- [ ] `backend/dre/periodo.go` -- `Guardar`, `ChaveLock`, `ErrPeriodoFechado` -- ponto único do AD-8
- [ ] `backend/dre/ingest.go` e `backend/dre/fechamento.go` -- `BeginTx` com `READ COMMITTED`; `Importar` chama `Guardar` por competência antes de escrever; `Fechar` toma o lock com a mesma chave -- serialização
- [ ] `backend/handlers/dre_import.go` -- `409` com a lista de escopos e a mensagem de reabrir -- FR-11
- [ ] `backend/dre/periodo_test.go`, `backend/handlers/dre_import_test.go`, `backend/arquitetura_test.go` -- casos da matriz, inversão do teste da 1.1, regra de que escrita em `lancamentos` passa por `Guardar` -- prova

**Acceptance Criteria:**
- Dado competência com fechamento, quando o analista importa para ela, então `409` listando os escopos recusados, com a mensagem de que é preciso reabrir, e nada é gravado.
- Dado competência aberta, quando importa, então funciona.
- Dado `dre.Periodo.Guardar(tx, empresa, competência)` com `pg_advisory_xact_lock(dre.ChaveLock)` e isolamento `READ COMMITTED`, quando fechar e importar a mesma competência rodam juntas, então são serializadas: a carga entra antes do snapshot ou é recusada depois, nunca fica de fora do snapshot e dentro dos lançamentos.
- Dado o código de `dre`, quando o teste de arquitetura roda, então toda escrita em `lancamentos` passa por `Guardar`, e o teste da 1.1 que registrava "aceita em mês fechado" está invertido.

## Implementation Notes

Tamanho do plano: cerca de 1750 tokens (estimativa), acima de 1600, porque a história cruza banco, backend e testes; sem divisão proposta.

## Design Notes

O lock é de transação (`xact`), então se solta no `Commit` ou `Rollback`, sem `unlock` manual e sem prender conexão do pool. Depois de tomar o lock, a conferência do fechamento é um `SELECT` novo; em `READ COMMITTED` ele enxerga o `Commit` de um `Fechar` que tenha acabado de liberar o lock. Essa é a razão de a ordem ser lock primeiro, conferência depois.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -p 1 ./...` -- expected: sem erro
- `cd backend && go test -p 1 -count=20 -run Corrida ./dre/` -- expected: passa nas 20 repetições
