---
title: 'Story 1.4: Importação CSV passa pelo Ingestor (AD-1, AD-2, AD-3)'
type: 'refactor'
created: '2026-10-28'
status: 'done'
baseline_commit: '6a408df2776a1d98207b5b5cb0115e552d1406da'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** `handlers/dre_import.go` faz `INSERT` direto em `lancamentos` e só acrescenta: reenviar o mesmo CSV duplica os lançamentos, o que torna a carga de validação irrepetível. A regra de escrita está espalhada num handler.

**Approach:** Criar o pacote `backend/dre` com a porta `Fonte`, o `Ingestor` (único escritor de `lancamentos`) e `dre/fontes/csv`. O handler só decodifica o upload e chama o Ingestor, que substitui por `(empresa, origem, competência, filial)` numa transação. Um teste de arquitetura falha o build se outro pacote escrever em `lancamentos`.

## Boundaries & Constraints

**Always:** A origem gravada pelo CSV continua `csv` (decisão do dono do produto). Todas as linhas são validadas antes de abrir a transação; uma linha inválida ou com `filial` vazia rejeita o arquivo, listando as linhas com problema (as primeiras 20 e o total), e nada é gravado. Reimportar apaga e regrava apenas os escopos presentes no arquivo, na mesma transação da carga e da trilha; lançamentos de outra origem na mesma competência e filial permanecem. Arquivo sem linhas de dados não apaga nada e informa (`400`, como hoje). `dre` não importa `handlers` nem `fontes`; `fontes/csv` não importa `database/sql`. A trilha continua com `lancamentos/importar` e ganha os escopos substituídos; como `dre` não pode chamar `handlers.writeAuditLogTx`, a gravação da trilha passa para um pacote pequeno `backend/trilha`, e `writeAuditLogTx` vira um invólucro dele. Formatos aceitos do CSV (separador, `competencia`, valor `1.234,56`) não mudam.

**Never:** Não mudar a guarda de período (1.6), o fechamento (1.5), o contrato JSON de sucesso (`carga_id`, `linhas`) nem os formatos do cabeçalho. Não apagar lançamentos de outra origem. Não mexer em `dre_orcado` nem em `DREOrcadoHandler`. Nenhum `DELETE` fora de um escopo explícito: nunca "limpar a competência inteira". Sem migration nesta história.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Primeira carga | CSV novo | linhas com `carga_id`; trilha | N/A |
| Reenvio | mesmo CSV | escopo substituído, contagem igual à de antes | N/A |
| Outra origem | lançamentos de outra origem na mesma competência e filial | permanecem | N/A |
| Linha inválida ou `filial` vazia | CSV com erro | nada gravado | `400` listando linhas |
| Arquivo só com cabeçalho | sem linhas | escopos intactos | `400` informando |

</frozen-after-approval>

## Open Questions

_Nenhuma._

## Code Map

- `backend/handlers/dre_import.go` -- `DREImportarHandler`: de onde sai a leitura do CSV (`parseValorBR`, `parseCompetencia`, detecção de `;` ou `,`, colunas `competencia`, `conta`, `valor` obrigatórias) e o `INSERT`; reaproveitar essas regras na fonte CSV; o handler fica só com multipart, `GetAudContext` e `http.MaxBytesReader` (64 MB).
- Novo `backend/dre/fonte.go` -- porta `Fonte` (`Nome()`, `Ler(ctx, Escopo)`), tipos `Lancamento` e `Escopo`; novo `backend/dre/ingest.go` -- `Ingestor.Importar(ctx, empresa, user, fonte)`: valida, calcula os escopos, e numa transação grava a carga, `DELETE` por escopo, `INSERT`, e a trilha.
- Novo `backend/dre/fontes/csv/csv.go` -- implementa `Fonte` sobre um `io.Reader`; sem acesso a banco. Para o CSV, `Escopo` só traz a empresa: os escopos saem das próprias linhas.
- Novo `backend/trilha/trilha.go` -- `Gravar(tx, empresa, user, entidade, id, acao, payload)`; `backend/handlers/audit_log.go` -- `writeAuditLogTx` passa a chamá-lo, sem mudar a assinatura nem os demais chamadores.
- `backend/migrations/003_dre.sql` -- `lancamentos.origem` já existe (padrão `csv`), `carga_id` sem FK. Linhas antigas com `filial = ''` não pertencem a nenhum escopo novo e só saem pela limpeza de teste da 1.2.
- `backend/public_routes_test.go` -- modelo de teste de código-fonte lendo `.go` com regex; o novo teste de arquitetura segue o estilo.
- `backend/internal/testdb` (1.1) e os testes de importação da 1.1 -- devem continuar passando; `go test -p 1`.
- `frontend/src/pages/aud/AdmCarga.tsx` -- texto diz que `filial` é opcional; corrigir para obrigatória e explicar que reenviar substitui.

## Tasks & Acceptance

**Execution:**
- [ ] `backend/trilha/trilha.go` e `handlers/audit_log.go` -- extrair a gravação -- `dre` precisa gravar a trilha sem importar `handlers`
- [ ] `backend/dre/fonte.go`, `ingest.go`, `fontes/csv/csv.go` -- porta, Ingestor e fonte CSV -- AD-1 e AD-2
- [ ] `backend/handlers/dre_import.go` -- reduzir a decodificação do upload e chamada ao Ingestor; mapear erros para `400` -- sem `INSERT`
- [ ] `backend/dre/ingest_test.go` e `backend/arquitetura_test.go` -- testes da matriz (reenvio, outra origem, rejeição com lista, arquivo vazio) e do escritor único: nenhum `.go` fora de `backend/dre` (e de `_test.go`) contém `INSERT INTO`, `DELETE FROM`, `UPDATE` ou `TRUNCATE` sobre `lancamentos`; `fontes` não importa `database/sql` -- trava a regressão
- [ ] `frontend/src/pages/aud/AdmCarga.tsx` -- texto -- coerência com a regra

**Acceptance Criteria:**
- Dado o pacote `backend/dre` com a porta `Fonte` e `dre/fontes/csv`, quando o handler recebe um CSV, então só chama `dre.Ingestor`, sem `INSERT` em lançamentos.
- Dado um escopo `(empresa, origem, competência, filial)` já importado, quando o CSV é reenviado, então o escopo é substituído numa transação e não duplica.
- Dado lançamentos de outra origem na mesma competência e filial, quando reimporta, então eles permanecem.
- Dado linha inválida ou `filial` vazia, quando importa, então rejeita listando as linhas e nada é gravado.
- Dado arquivo sem linhas, quando importa, então não apaga o escopo e informa; e um teste falha o build se outro pacote tiver `INSERT` ou `DELETE` em `lancamentos`.

## Implementation Notes

Tamanho do plano: cerca de 2050 tokens (estimativa), acima de 1600, porque a história cruza banco, backend e testes; sem divisão proposta.

## Design Notes

O CSV define os próprios escopos (cada par competência e filial presente), diferente de uma fonte Oracle, que receberá o escopo pedido. Por isso `Ler` devolve linhas e o Ingestor deriva os escopos delas; a mesma lógica serve às duas fontes depois.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -p 1 ./...` -- expected: sem erro, incluindo os testes da 1.1
- `cd frontend && npm run typecheck` -- expected: só o erro anterior em `ResetPassword.tsx(13,9)`, que não é desta história (`npm run lint` não roda)
- `grep -rniE "(insert into|delete from|update|truncate)\s+(auditoria\.)?lancamentos" backend --include=*.go | grep -v "backend/dre/" | grep -v "_test.go"` -- expected: vazio (os testes podem semear lançamentos)

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Importações simultâneas do mesmo escopo duplicam linhas | medium | patch | `pg_advisory_xact_lock` por `(empresa, origem)`; `TestImportacoesConcorrentesNaoDuplicam` |
| Erro de INSERT por linha virou 500 sem linha | medium | patch | embrulhado em `ErroLinhas` com a linha; 400 |
| Arquivo sem coluna `filial` reporta erro por linha | medium | patch | `filial` na checagem do cabeçalho |
| `NaN` e `Inf` aceitos como valor | medium | patch | rejeitados em `parseValorBR` |
| Numeração de linha pelo contador, não pela posição do registro | low | patch | `cr.FieldPos(0)` e `ParseError.Line` |
| 500 sem log no handler | low | patch | `log.Printf` |
| Mapeamento 400 do handler e parser CSV sem teste | medium | patch | `TestImportar_Invalido_Retorna400ComMensagem` e `csv_test.go` |
| Linhas legadas com `filial` vazia nunca são substituídas | low | rejeitado | a spec as deixa intocadas |
| Teste de arquitetura contornável (aspas, COPY, MERGE, drivers) | low | rejeitado | guarda de regressão; reforço seria complexidade sem dano demonstrado |
| `trilha.Gravar` sem contexto; validação de conta duplicada; trim de filial | low | rejeitado | negligível |
| Sem confirmação nem contagem de substituídas na tela | low | rejeitado | fora da spec |
| Valor em `float64` | low | defer | herdado do handler antigo |
| Testes não rodam no `go build`; AGENTS.md ainda diz que não há testes Go | low | defer | já registrado |
