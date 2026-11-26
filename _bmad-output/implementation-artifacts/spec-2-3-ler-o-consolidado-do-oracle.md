---
title: 'Story 2.3: Ler o realizado consolidado do Oracle'
type: 'feature'
created: '2026-11-26'
status: 'review'
baseline_commit: '1ce4233'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/epics.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Faturamento, devoluções, ST e desconto de custo só entram por arquivo.

**Approach:** Fonte em `dre/fontes/oracle` (go-ora v2, só leitura) que lê a view consolidada por `MES_ANO`.

## Boundaries & Constraints

**Always:**
- Correspondência coluna → conta em JSON (`contas_consolidado.json`, substituível por `JC_ORACLE_CONTAS_ARQUIVO`); contas iguais às do Mapa (1, 2, ST, DESC CUSTO). Isto substitui o `ERP.<COLUNA>` do critério original.
- Colunas de CMV de devolução com conta vazia (não importadas): a view não traz o CMV bruto e o arquivo mensal usa a conta 3 para os dois.
- Valor como a fonte entrega (positivo); o sinal é do Mapa. A conta de devoluções precisa de sinal -1 no Mapa.
- UF gravada em `lancamentos.uf`; filial normalizada ("01" = "1"); valor nulo não é gravado; `MES_ANO` inválido rejeita a carga do mês.
- Para o ERP, reimportar substitui a competência inteira daquela origem; leitura vazia não apaga.

**Never:**
- Consulta sem filtro de período.
- Inventar coluna de CMV.
- Nenhum dado real do cliente no repositório; o Oracle é testado por um leitor falso (a máquina de desenvolvimento não o alcança).
- Nunca fazer push sem autorização.

</frozen-after-approval>
