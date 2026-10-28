---
title: 'Story 2.2: CSV e XLSX no formato da planilha atual (FR-3)'
type: 'feature'
created: '2026-10-28'
status: 'draft'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O importador só aceita o formato do sistema (`competencia`, `filial`, `conta`, `valor`) e só CSV. As planilhas do cliente têm `CONTA, DESCRIÇÃO, VL. Realizado, Ajustes, Saldo, Grupo, Mês, Loja`, em CSV ou XLSX, e a carga falha com "coluna obrigatória ausente: competencia".

**Approach:** Aceitar CSV e XLSX, reconhecendo os dois formatos de cabeçalho no mesmo importador. O formato da planilha grava com origem `csv.contas`; o antigo segue com origem `csv`. Ambos passam pelo `dre.Ingestor` (substituição por escopo, guarda de período), sem mudar suas regras.

## Boundaries & Constraints

**Always:** Detecção do formato pelo conteúdo, não só pela extensão: arquivo que começa com a assinatura ZIP (`PK`) é lido como XLSX; o resto como CSV (`;` ou `,`, BOM aceito, como hoje). XLSX com `github.com/xuri/excelize/v2` (versão fixa, já resolvida com `go get`), lendo a primeira planilha que tenha a linha de cabeçalho, procurada nas primeiras 20 linhas; células lidas como o texto exibido; limite de tamanho descompactado configurado na biblioteca (`UnzipSizeLimit`) e o `MaxBytesReader` atual de 64 MB mantido. As linhas (CSV ou XLSX) viram `[][]string` e uma única rotina decide o formato pelo cabeçalho: se houver `competencia`, é o formato antigo (comportamento e origem `csv` inalterados); senão, é o formato da planilha. Cabeçalho normalizado (minúsculas, sem acento, sem pontos nem espaços repetidos): `conta`; `descricao`; `vl. realizado`, `vl realizado`, `valor realizado` ou `realizado` para o valor; `ajustes`; `saldo`; `grupo`; `mes`; `loja`. Obrigatórias no formato da planilha: `conta`, `valor realizado`, `mes` e `loja`; faltando, `ErroEntrada` listando as esperadas. `Mês`: AAAAMM, em texto ou número (`202609`); outro formato rejeita a linha com o seu número. `Loja` vira `filial`: se for só dígitos, remove zeros à esquerda (`01` e `1` são a mesma filial, como o `CODFILIAL` do ERP) e aceita número de célula (`11` ou `11.0` vira `11`); vazia rejeita a linha. `VL. Realizado` vira o valor (número da célula ou texto `1.234,56` e `1234.56`; `NaN` e `Inf` rejeitados) e `DESCRIÇÃO` vira `conta_desc`. O sinal é o que a planilha traz (o sinal do DRE vem de `dre_mapa.sinal`). `Ajustes` não é importado: a resposta de sucesso ganha `ajustes_ignorados` com a quantidade de linhas com ajuste diferente de zero; `Saldo` e `Grupo` são ignorados. Linhas totalmente vazias são puladas. As linhas com problema são reunidas e rejeitam o arquivo inteiro (até 20 listadas e o total, como hoje). Competência fechada continua recusada (409, guarda da 1.6). Tela `AdmCarga.tsx`: aceita `.csv` e `.xlsx`, descreve os dois formatos e mostra `ajustes_ignorados` quando maior que zero.

**Never:** Nenhuma planilha, valor, nome ou código real do cliente em teste, fixture ou documento (gerar os arquivos de teste em código com linhas fictícias, inclusive o XLSX). Não gravar ajuste. Não ler macros, fórmulas externas nem mais de uma planilha. Não mudar a substituição por escopo do Ingestor nem a guarda de período. Não tocar em migrations. Produção só recebe por push; decisão humana.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Planilha CSV | cabeçalho `CONTA;DESCRIÇÃO;VL. Realizado;Ajustes;Saldo;Grupo;Mês;Loja` | grava com origem `csv.contas`, `Mês` vira competência, `Loja` filial | N/A |
| Planilha XLSX | mesmas colunas numa planilha | idem CSV | N/A |
| Cabeçalho depois de título | 3 linhas de título antes do cabeçalho | acha o cabeçalho nas primeiras 20 linhas | N/A |
| Ajustes preenchidos | 4 linhas com ajuste ≠ 0 | importa o valor, ignora o ajuste; `ajustes_ignorados: 4` | N/A |
| Mês numérico | célula `202609` | competência 2026-09 | N/A |
| Mês inválido | `2026-09` ou `13/2026` na planilha | linha rejeitada com o número | `400` com a lista |
| Loja vazia | célula vazia | linha rejeitada com o número | `400` |
| Loja `01` e `1` | duas linhas | mesma filial `1` | N/A |
| Formato antigo CSV | `competencia;filial;conta;valor` | como hoje, origem `csv` | N/A |
| Formato antigo em XLSX | mesmas colunas num XLSX | aceito, origem `csv` | N/A |
| Sem colunas reconhecidas | cabeçalho qualquer | rejeita, lista as esperadas | `400` |
| Mês fechado | competência com fechamento ativo | recusa o arquivo inteiro | `409` |
| Reenvio | mesmo arquivo duas vezes | substitui o escopo, sem duplicar | N/A |
| Arquivo corrompido | ZIP truncado | rejeita com mensagem clara | `400` |

</frozen-after-approval>

## Code Map

- `backend/dre/fontes/csv/csv.go` -- fonte atual (cabeçalho antigo, `parseValorBR`, `parseCompetencia`, linha por `FieldPos`); base para a rotina comum de linhas; manter o comportamento do formato antigo e seus testes em `csv_test.go`.
- `backend/dre/fonte.go` e `backend/dre/ingest.go` -- porta `Fonte`, `Lancamento`, `ErroEntrada`, `ErroLinhas` e `Importar`; a origem vem da fonte (`Nome()`); não mudar as regras do Ingestor.
- `backend/handlers/dre_import.go:26` -- multipart, `MaxBytesReader` de 64 MB e chamada ao Ingestor; resposta de sucesso `carga_id` e `linhas` (acrescentar `ajustes_ignorados`).
- `backend/dre/fontes/` -- criar `planilha/` (ou estender `csv/`) com leitura XLSX e a rotina comum; decidir o menor desenho que mantenha `fontes` sem `database/sql` (teste de arquitetura em `backend/arquitetura_test.go`).
- `frontend/src/pages/aud/AdmCarga.tsx` -- tela de carga.
- `backend/go.mod`, `backend/go.sum` -- acrescentar `github.com/xuri/excelize/v2 v2.9.0` por `go get` e `go mod tidy`.
- `backend/handlers/dre_import_test.go` e `backend/dre/ingest_test.go` -- padrões de teste com banco (`importar`, `testdb`) para reusar.

## Tasks & Acceptance

**Execution:**
- [ ] `backend/dre/fontes/...` -- leitura XLSX e CSV para `[][]string`, detecção do cabeçalho nas primeiras 20 linhas, mapeamento dos dois formatos, normalização de `Mês`, `Loja` e valor, contagem de `ajustes_ignorados`
- [ ] `backend/handlers/dre_import.go` -- passar o nome do arquivo e a fonte nova; devolver `ajustes_ignorados`
- [ ] `backend/dre/fontes/.../*_test.go` e `backend/handlers/dre_import_test.go` -- cobrir a matriz, gerando o XLSX em código com excelize
- [ ] `frontend/src/pages/aud/AdmCarga.tsx` -- `.csv,.xlsx`, descrição dos formatos e aviso de ajustes ignorados

**Acceptance Criteria:**
- Given uma planilha CSV ou XLSX no formato do cliente, when o analista a importa, then os lançamentos entram com origem `csv.contas` e a resposta informa quantas linhas tinham ajuste.
- Given o formato antigo, when importado, then continua igual (origem `csv`).
- Given `go build ./...`, `go vet ./...`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`), when rodam, then passam.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- expected: passam
- `cd frontend && npm run typecheck && npm run build` -- expected: só o erro antigo do typecheck; o build passa

## Implementation Notes

Tamanho acima de 1600 tokens: cruza leitura de arquivo, backend e tela; sem divisão proposta. Decisão a revisar: `Loja` só com dígitos perde os zeros à esquerda para casar com o `CODFILIAL` do ERP; o formato antigo não faz essa normalização. Os nomes exatos das colunas da planilha real ainda não foram confirmados pelo cliente; os apelidos do cabeçalho cobrem as variações prováveis.
