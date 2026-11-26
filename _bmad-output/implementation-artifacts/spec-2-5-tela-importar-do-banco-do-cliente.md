---
title: 'Story 2.5: Tela "Importar do banco do cliente"'
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

**Problem:** O analista precisa disparar a importação e ver o resultado.

**Approach:** `GET/POST /api/aud/importacao-erp` (capacidade `carregar`) e o bloco `ImportarERP.tsx` em Configurações > Carga.

## Boundaries & Constraints

**Always:**
- POST responde 202 e a importação segue no servidor (o túnel corta pedidos longos); a tela relê a lista até terminar.
- Sem configuração, o bloco diz o motivo e o POST devolve 503; perfil sem `carregar` recebe 403 registrado.

**Never:**
- Mostrar credencial ou endereço do banco.
- Nenhum dado real do cliente no repositório; o Oracle é testado por um leitor falso (a máquina de desenvolvimento não o alcança).
- Nunca fazer push sem autorização.

</frozen-after-approval>
