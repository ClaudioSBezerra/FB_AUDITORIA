---
title: 'Story 2.1: Uma conta, uma fonte (AD-16)'
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

**Problem:** A mesma conta vinda do banco do cliente e de um arquivo somaria em dobro no DRE.

**Approach:** Tabela `auditoria.conta_fonte`; o Ingestor dá dona à conta na primeira carga e recusa outra origem.

## Boundaries & Constraints

**Always:**
- A posse é por conta exata, registrada na transação da carga e na trilha; carga com conta de outra dona não grava nada e cita a dona. A simulação também recusa.
- A posse é configuração: `limpar_dados_teste` não a apaga.
- Não há preenchimento retroativo: cargas anteriores à migração não criam dona.
- A posse sozinha não protege dados antigos, então a fonte do ERP também é recusada numa competência que já tem as mesmas contas de origem não ERP, em qualquer filial.

**Never:**
- Apagar lançamentos de outra origem.
- Trocar a dona (história 2.8, não feita).
- Nenhum dado real do cliente no repositório; o Oracle é testado por um leitor falso (a máquina de desenvolvimento não o alcança).
- Nunca fazer push sem autorização.

</frozen-after-approval>
