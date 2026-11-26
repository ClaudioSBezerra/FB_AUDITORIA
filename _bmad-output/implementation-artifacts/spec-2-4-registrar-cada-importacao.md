---
title: 'Story 2.4: Registrar cada importação e a posse das contas'
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

**Problem:** Sem registro, não se sabe o que foi importado, pulado ou recusado.

**Approach:** Orquestrador `dre.ImportacaoERP` com `auditoria.execucoes`, trilha e advisory lock em conexão dedicada.

## Boundaries & Constraints

**Always:**
- Uma transação por competência e origem; mês fechado é pulado; carga recusada não impede as demais; falha de leitura interrompe o restante e mantém os dados anteriores.
- Mensagem de falha em palavras (rede, serviço, credencial, objeto), sem endereço nem senha; o log também as oculta.
- Só a empresa de `JC_EMPRESA_ID`. `execucoes` entra em `limpar_dados_teste`.
- No máximo 12 meses por pedido. "Atualizar dados" relê o mês anterior e o corrente.

**Never:**
- Agendador (história 2.13).
- Orçado (história 2.6).
- Nenhum dado real do cliente no repositório; o Oracle é testado por um leitor falso (a máquina de desenvolvimento não o alcança).
- Nunca fazer push sem autorização.

</frozen-after-approval>
