---
title: 'Story 2.17: Importar a Entrada_Dados do Marcos'
type: 'feature'
created: '2026-10-29'
status: 'done'
baseline_commit: 'e9a1f15'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
  - '{project-root}/_bmad-output/implementation-artifacts/spec-2-2-planilha-csv-xlsx.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O arquivo de carga mensal do Marcos (aba `Entrada_Dados`) tem duas tabelas na mesma aba, cada uma com o seu cabeçalho (`Código, Conta, ...` e depois `CONTA, DESCRIÇÃO, ...`), subtotais sem código, linhas de conferência (`TOTAL`, prova real) sem `Mês` e `Loja`, e `Mês`/`Loja` como fórmulas. O importador da 2.2 escolhia só o primeiro cabeçalho, lia `Conta` como código (era a descrição), tratava o segundo cabeçalho como dado e rejeitava o arquivo pelas linhas finais.

**Approach:** No formato da planilha, varrer todas as linhas e refazer o mapeamento de colunas a cada cabeçalho repetido; importar só linha com código, valor, `Mês` e `Loja`; ignorar e contar o resto; conferir a linha `TOTAL` contra a soma das contas da mesma tabela. Os formatos da 2.2 e do sistema não mudam.

## Boundaries & Constraints

**Always:** `Código`/`Codigo` é o código da conta. Com `Código` e `Conta`, `Código` é a conta e `Conta` é a descrição; só com `Conta`, `Conta` é o código e a descrição vem de `DESCRIÇÃO` (2.2). Cada linha que satisfaz o conjunto obrigatório (conta ou código, valor realizado, mês, loja) refaz o mapeamento dali em diante, e o cabeçalho repetido nunca vira dado (só no formato da planilha; o formato do sistema segue com um cabeçalho). Linha de dado: tem código, valor numérico, `Mês` e `Loja`. Linha sem código (subtotal) é ignorada e contada em `linhas_ignoradas` (importar subtotal dobraria a contagem). Linha com código mas sem `Mês` e sem `Loja` (conferência, prova real) é ignorada e contada. Só um dos dois vazio, `Mês` inválido ou valor não numérico em linha com `Mês`/`Loja` é erro de linha. `Ajuste` e `Ajustes` são aceitos; ajuste não é importado, só contado. Linha cujo código é `TOTAL` (sem acento, maiúsculas indiferentes) nunca é importada; com valor, gera `conferencia_total {esperado, calculado, ok}` contra a soma das linhas importadas da mesma tabela, com tolerância de um centavo; divergência é aviso e não bloqueia. Se nenhuma linha foi importada e há linhas com código e valor mas `Mês` e `Loja` vazios (XLSX salvo sem o valor calculado das fórmulas), o erro diz: "Mês e Loja vazios: as fórmulas não têm valor calculado; abra o arquivo no Excel e salve de novo, ou preencha Mês e Loja". Código de conta de texto é aceito; no XLSX o código numérico de célula (`123.0`) vira `123`. A resposta de sucesso ganha `linhas_ignoradas` e, quando há `TOTAL`, `conferencia_total`; a tela mostra os dois (aviso na divergência).

**Never:** Nenhum valor, código, nome de conta, de filial ou de cliente reais em teste, fixture ou documento (a fixture é gerada em código, com a mesma estrutura e dados fictícios). Não importar subtotais. Não gravar ajuste. Não mudar o Ingestor, a guarda de período nem o formato do sistema.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior |
|----------|--------------|---------------------------|
| Duas tabelas | cabeçalho, resumo, linha em branco, segundo cabeçalho, contas | as duas importadas; cabeçalho repetido não vira dado |
| Subtotal | linha sem código | ignorada e contada |
| Conferência | `TOTAL`, prova real, sem `Mês` e `Loja` | ignoradas e contadas |
| TOTAL confere / diverge | soma das contas igual / diferente | `conferencia_total.ok` verdadeiro / falso, sem bloquear |
| `Ajuste` singular | coluna `Ajuste` com valor | contado em `ajustes_ignorados` |
| XLSX sem cache das fórmulas | `Mês` e `Loja` vazios em todas as linhas com código e valor | `400` com a mensagem das fórmulas |
| Só a `Loja` vazia | uma conta com `Mês` | `400` com `Loja vazia` e o número da linha |
| Reenvio | mesmo arquivo duas vezes | substitui o escopo, sem duplicar |
| Mês fechado | competência com fechamento ativo | `409`, nada gravado |
| Formatos da 2.2 e do sistema | como antes | mesmo comportamento (testes antigos passam) |

</frozen-after-approval>

## Code Map

- `backend/dre/fontes/csv/csv.go` -- `ehCabecalho` reconhece `Código`; `Ler` desvia o formato da planilha para `lerPlanilha`, que varre todas as linhas, refaz `mapaPlanilha` a cada cabeçalho, conta `LinhasIgnoradas()` e monta `Conferencia()`.
- `backend/dre/fontes/csv/entrada_dados_test.go` -- fixture fictícia (XLSX com fórmulas e valor em cache, e o mesmo em CSV) e os casos da matriz.
- `backend/handlers/dre_import.go` e `dre_import_test.go` -- resposta com `linhas_ignoradas` e `conferencia_total`; testes com banco (reenvio, 409, 400).
- `frontend/src/pages/aud/AdmCarga.tsx` -- descrição do formato e avisos.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo de `ResetPassword.tsx` no typecheck

## Implementation Notes

Decisões de detalhe (pelo padrão do código):
- A linha `TOTAL` é sempre de conferência, mesmo que traga `Mês` e `Loja`: nunca vira lançamento.
- Linha sem código é ignorada mesmo com valor, `Mês` e `Loja` preenchidos (é o caso dos subtotais da tabela 1, que têm `Mês` e `Loja` por fórmula).
- A conferência soma as contas importadas desde o último cabeçalho; com mais de um `TOTAL`, vale o primeiro que diverge, senão o último.
- No CSV, a checagem de número de colunas só vale para linhas que seriam dado (com `Mês` ou `Loja`), para não rejeitar o arquivo pelas linhas de prova, que podem ter largura diferente.
- Normalização do código por `inteiroTexto` só no XLSX; no CSV o código é lido como está.
- Pontos frágeis: se a coluna `Loja` do arquivo já vier consolidada por filial gerencial (pergunta aberta), o importador a grava como filial, sem aplicar o mapa de filiais (2.14); linha sem código mas com valor some em silêncio (só contada).

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Conferência do TOTAL com ruído de ponto flutuante (250,2499...) | medium | patch | valores arredondados a centavos antes de comparar e devolver |
| Prova real com largura diferente rejeitaria o CSV | medium | patch | checagem de colunas só em linhas de dado |
| Subtotal com valor e sem código é descartado sem aviso individual | low | rejeitado | decisão do dono do produto: subtotal não se importa; a quantidade vai em `linhas_ignoradas` |
| Filial da planilha não passa pelo mapa de filiais | low | defer | depende da resposta do Marcos sobre a coluna `Loja` consolidada |
