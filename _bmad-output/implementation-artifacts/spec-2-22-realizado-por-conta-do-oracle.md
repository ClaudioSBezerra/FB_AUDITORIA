---
title: 'Story 2.22: Ler o realizado por conta do Oracle'
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

**Problem:** O realizado por conta gerencial só entra por arquivo.

**Approach:** Segunda fonte em `dre/fontes/oracle`, origem `oracle.realizado`, lendo `CONTR_DRE_REALIZ`.

## Boundaries & Constraints

**Always:**
- Objeto e coluna de data por `JC_ORACLE_REALIZ_OBJETO` e `JC_ORACLE_REALIZ_DATA` (padrões `CONTR_DRE_REALIZ` e `DTLANC`), validados como identificadores.
- Soma por filial, conta e dia; valor com o sinal da fonte; data no histórico; filial como vem (o Mapa de filiais consolida).
- Nenhuma conta é filtrada no código; contas sem Mapa (como as 8001xxx) ficam pendentes de decisão.
- Com coluna de data de pagamento, o que não tem pagamento fica fora do mês.

**Never:**
- Composição de filiais no código.
- Concatenar texto livre em SQL.
- Nenhum dado real do cliente no repositório; o Oracle é testado por um leitor falso (a máquina de desenvolvimento não o alcança).
- Nunca fazer push sem autorização.

</frozen-after-approval>
