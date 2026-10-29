---
title: 'Story 3.3: Gerador local da estrutura e do Mapa iniciais'
type: 'feature'
created: '2026-10-29'
status: 'review'
route: 'dispatch'
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

## Intent

**Problem:** sem estrutura e sem Mapa o DRE do cliente sai vazio, e digitar o DRE e centenas de contas à mão não é viável. A planilha de análise do cliente já tem a estrutura (aba do DRE mensal, com as fórmulas dos subtotais) e o dicionário (aba de métricas, com o grupo de cada conta).

**Approach:** um programa local, `backend/cmd/gerar-estrutura-inicial`, que lê essa planilha e grava, em pasta FORA do repositório, os CSVs que as telas carregam (3.2 para estrutura e Mapa, 2.10 para o dicionário, cadastro de departamentos). Só estrutura: nenhum valor é lido; nenhum nome de conta, código ou texto de linha do cliente está no programa nem no repositório.

## Uso

```
go run ./cmd/gerar-estrutura-inicial -arquivo "analise.xlsx" -saida /pasta/fora/do/repo
```

Opções: `-aba-estrutura` (padrão `MENSAL`), `-aba-dicionario` (padrão `METRICAS2`), `-coluna-valor` (padrão `D`). A saída traz só contagens. O programa recusa uma pasta de saída dentro de um repositório git.

## Regras

- **Estrutura (aba do DRE):** cabeçalho com `GRUPO` na coluna A e `DESCRIÇÃO` na C (nas 30 primeiras linhas). Cada linha abaixo com descrição vira uma linha do DRE. Coluna A = código da linha. Sem chave (vazia ou `0`, o caso dos subtotais), o código é o slug da descrição (sem marcas como `(=)`, sem acento, `_`; repetido ganha sufixo `_2`). Chave repetida é erro.
- **Subtotal:** a célula da coluna de valor tem fórmula de soma simples (`SUM(Dx:Dy[,Dz])` ou `Dx+Dy-Dz`). Intervalo contribui com as linhas do intervalo que existem no DRE (linha em branco é pulada); referência isolada a linha que não é do DRE, autossoma ou subtotal sem componentes são erros. Termo subtraído vira `-codigo`. Qualquer outra fórmula (SUMIFS, IF) faz da linha uma conta. Nível: 0 para subtotal, 1 para conta; ordem = posição x 10.
- **Papel:** `faturamento_liquido` no primeiro subtotal cuja descrição normalizada contém `VENDA LIQUIDA` ou `FATURAMENTO LIQUIDO`.
- **Dicionário (aba de métricas):** cabeçalho achado pelas células `CONTA` e `GRUPO` (e `DESCRIÇÃO`, `CENTRO DE CUSTO`). Linha sem conta (resumo) e linha `TOTAL` são ignoradas.
- **Mapa:** conta cujo GRUPO casa com o código de uma linha de conta (igual, ou igual depois de normalizar acento, caixa e espaços repetidos) vira `conta_padrao,linha_codigo,sinal=1` (valores já vêm com sinal e subtotais são somas simples). Sem GRUPO, ou GRUPO sem linha de conta (inclusive GRUPO que aponta para subtotal): `contas_sem_linha.csv` (`conta, descricao, grupo, centro_custo, motivo`), fora do Mapa. Conta repetida com GRUPOs diferentes: erro claro; repetida com o mesmo GRUPO: ignorada e contada.
- **Departamentos:** os blocos são os grupos do sistema (`dre.GruposDepartamento`). Para cada subtotal cuja descrição (sem marca) é o nome de um bloco, as linhas de conta somadas por ele (descendo por subtotais aninhados) viram um departamento: `codigo` = código da linha, `nome` = descrição, `grupo` = bloco.
- **Dicionário:** `conta, descricao, grupo, departamento, centro_custo`. `grupo` = código da linha mapeada (ou o GRUPO original se a conta ficou sem linha); `departamento` = esse código quando a linha é de um departamento, senão vazio. `centro_custo` é ignorada pela carga atual (3.13 a usará).
- **Arquivos:** CSV com `;`, UTF-8 com BOM, mais `contas_sem_linha.csv`. Limites das cargas (conta 60, descrição 200, código/grupo 40) são conferidos na geração.

## Ordem de carga em Teste

Estrutura (3.2) → Mapa (3.2) → Departamentos (cadastro na tela; não há carga por arquivo de departamentos, ver pontos frágeis) → Dicionário (2.10, exige os departamentos). Revisão do resultado com a área de negócio antes de carregar em produção; registrar quem validou e quando (3.3, critério de validação).

## Pontos frágeis

- Departamentos só podem ser cadastrados um a um na tela; uma carga por arquivo seria uma história pequena.
- Sinal fixo em 1; a paridade (3.7) confirma.
- Nada é inferido sobre as contas `ERP.*` nem sobre contas fora do dicionário da planilha.

## Verificação

`gerador_test.go` gera em código uma planilha fictícia mínima (abas do DRE e das métricas, fórmulas `SUM(Dx:Dy)`, `D#+D#`, subtração, linha em branco no intervalo, chave com espaço duplo, GRUPO com caixa diferente, conta sem GRUPO, GRUPO inexistente, GRUPO que aponta para subtotal, conta repetida) e confere os cinco CSVs; testa também os erros (conta repetida com GRUPOs diferentes, chave repetida, autossoma, referência fora do DRE) e a recusa de saída dentro de um repositório. Os CSVs do arquivo real foram conferidos contra as cargas das telas em teste descartável fora do repositório; só contagens foram registradas.
