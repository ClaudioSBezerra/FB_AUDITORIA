---
title: 'Story 1.7: Um fechamento ativo por mês e reabertura com motivo (FR-9, FR-10, AD-9)'
type: 'feature'
created: '2026-10-28'
status: 'approved'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Hoje cada `POST` de fechamento cria uma versão nova por `(empresa, competência, filial)`, sem noção de "ativo" nem de reabertura: dá para fechar o mesmo mês várias vezes e por filial, e não há registro de por que se refez. A 1.6 só consegue dizer "existe fechamento".

**Approach:** Migration com `reaberto_em`, `reaberto_por`, `motivo_reabertura` e índice único parcial em `(empresa, competência) WHERE reaberto_em IS NULL`. Fechar com ativo devolve `409`; `dre.Fechamento.Reabrir` (motivo obrigatório, com o lock da 1.6) marca o ativo; novo Fechar cria `MAX+1` por `(empresa, competência)`. A guarda da 1.6 passa a olhar só o fechamento ativo. O fechamento é sempre do grupo inteiro.

## Boundaries & Constraints

**Always:** Antes de criar o índice, a migration confere se há mais de um fechamento sem `reaberto_em` para a mesma `(empresa, competência)` (inclui fechamentos por filial). Por empresa: se o ambiente dela (1.2) for `teste`, consolida (decisão do dono do produto): marca como reabertos, com motivo fixo "consolidado na migração, ambiente de teste", todos os fechamentos por filial e todas as versões consolidadas exceto a de maior versão, que fica ativa; se for `producao`, aborta com uma mensagem clara sem alterar dado. A consolidação roda antes de o gatilho de imutabilidade existir e nunca apaga linha. A mensagem de aborto NÃO pode conter `already exists` nem `duplicate`, porque o runner trata esses textos como "migration já aplicada" e seguiria em frente sem o índice. Reabrir e fechar tomam `ChaveLock`; só `controladoria` (ou admin de plataforma) fecha e reabre, `403` para os demais; motivo vazio ou só espaços é recusado (`400`). `reaberto_*` mudam uma única vez, por gatilho que também bloqueia `DELETE`, ambos liberados só quando `auditoria.imutavel_salvo_em_teste(empresa_id)` for verdadeira (1.2). A trilha registra `fechar` e `reabrir`. A versão anterior continua legível. O corpo do `POST` com `filial` preenchida é recusado (`400`: o fechamento é do grupo inteiro); fechamentos por filial já gravados continuam legíveis e aparecem na listagem.

**Never:** A migration só faz `UPDATE` na consolidação de empresa em `teste` descrita acima; em `producao` não altera dado nenhum. Nunca `DELETE`; não apagar versões. Não mexer em `dre_fechamento_linhas` nem na leitura do DRE (1.8). Não criar migration que dependa de dado de produção que não se conhece. Produção só recebe a migration por push; decisão humana.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Fechar de novo | mês com ativo | nada gravado | `409` |
| Reabrir | ativo, motivo informado | ativo recebe `reaberto_*`; competência aceita carga; trilha | N/A |
| Reabrir sem motivo | motivo vazio | nada muda | `400` |
| Fechar após reabrir | sem ativo | versão `MAX+1` da competência | N/A |
| Sem permissão | diretoria | nada muda | `403` |
| Reabrir o que não está fechado | sem ativo | nada muda | `409` |
| Dados antigos | mais de um sem `reaberto_em` na mesma competência | migration aborta, nada alterado | mensagem clara |

</frozen-after-approval>

## Open Questions

_Nenhuma._

## Code Map

- `backend/migrations/003_dre.sql` -- `dre_fechamentos` com `UNIQUE (empresa_id, competencia, filial, versao)`; `fechado_por` tem `ON DELETE SET NULL`, que é um `UPDATE` e passará a colidir com o gatilho (usuário que fechou um mês deixa de poder ser apagado; coerente com "usuários são desativados, nunca apagados", e é risco a registrar). Nova migration (próximo número livre): colunas, pré-checagem com `RAISE EXCEPTION`, gatilho, índice. **Irreversível em produção.**
- `backend/main.go` -- runner das migrations (`already exists` e `duplicate` viram "skipped"); registrar a rota `/api/aud/dre/fechamentos/reabrir`.
- `backend/handlers/dre.go` -- `DREFechamentoHandler` (POST/GET) fino desde a 1.5; GET passa a devolver também `ativo`, `reaberto_em`, `motivo_reabertura`; `DREHandler` lê o fechamento por `filial` e última versão: passa a ler o ativo consolidado da competência (a leitura completa do snapshot é a 1.8).
- `backend/dre/fechamento.go` (1.5) -- `Fechar` ganha o `409` com ativo e `MAX+1` por competência; novo `Reabrir`; `backend/dre/periodo.go` (1.6) -- `Guardar` passa a considerar só o ativo.
- `backend/handlers/dre_fechamento_test.go` e `dre_import_test.go` (1.1, 1.6) -- ajustar o caso "fechar de novo" (era versão 2; vira `409`); manter o resto.
- `frontend/src/pages/aud/DRE.tsx` -- `fechar()` envia `filial` do filtro: passar a fechar sempre o grupo e dizer isso no texto; botão "Reabrir período" com motivo (`window.prompt`, como o fechar). `DREFechamentos.tsx` -- colunas de situação (ativo ou reaberto, motivo).
- `backend/internal/testdb` (1.1) -- reaplicar a migration sobre dados antigos no teste de aborto.

## Tasks & Acceptance

**Execution:**
- [ ] `backend/migrations/00N_fechamento_reabertura.sql` -- colunas, pré-checagem (mensagem sem as palavras do runner), gatilho de uma só mudança e sem `DELETE` (com `imutavel_salvo_em_teste`), índice único parcial -- AD-9
- [ ] `backend/dre/fechamento.go`, `periodo.go` -- `Reabrir`, `409` com ativo, `MAX+1` por competência, guarda só do ativo, `filial` recusada -- regra
- [ ] `backend/handlers/dre.go`, `backend/main.go` -- rota de reabertura, listagem com situação, `403`/`400` -- adaptador
- [ ] `backend/dre/fechamento_test.go`, testes de handler -- toda a matriz; aborto da migration com texto sem `already exists`/`duplicate` (a migration é executada no banco de teste com dados repetidos); gatilho recusa segunda reabertura e `DELETE` fora de teste -- prova
- [ ] `frontend/src/pages/aud/DRE.tsx`, `DREFechamentos.tsx` -- sem escolha de filial, reabrir com motivo, situação -- interface

**Acceptance Criteria:**
- Dado a migration, então `dre_fechamentos` ganha `reaberto_em`, `reaberto_por`, `motivo_reabertura` e um índice único parcial em `(empresa, competência) WHERE reaberto_em IS NULL`; e, se houver mais de um sem `reaberto_em` na mesma competência, a migration aborta com mensagem, sem alterar dado.
- Dado competência fechada e ativa, quando o Controller tenta fechar de novo, então `409`.
- Dado motivo informado, quando o Controller reabre, então o ativo recebe `reaberto_*`, a competência volta a aceitar escrita, a versão anterior fica legível e a trilha registra.
- Dado reabrir sem motivo, quando o Controller envia, então recusa.
- Dado competência reaberta, quando o Controller fecha de novo, então a versão é `MAX+1` por `(empresa, competência)`, mantendo as anteriores.
- Dado um usuário sem o perfil `controladoria` (por exemplo, diretoria), quando tenta fechar ou reabrir, então `403`; `reaberto_*` mudam uma só vez, por gatilho que respeita `imutavel_salvo_em_teste`; e a tela de fechamento deixa de oferecer escolha de filial, com fechamentos por filial já gravados ainda legíveis.

## Implementation Notes

Tamanho do plano: cerca de 2400 tokens (estimativa), acima de 1600, porque a história cruza banco, backend e testes; sem divisão proposta.

## Design Notes

O índice parcial é por `(empresa, competência)` e não inclui `filial`: um fechamento por filial antigo e um consolidado do mesmo mês colidem, e é por isso que a pré-checagem conta por `(empresa, competência)`. Para o gatilho aceitar apenas a transição `reaberto_em` de nulo para preenchido, com os três campos juntos, ele compara `OLD` e `NEW` nas demais colunas.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -p 1 ./...` -- expected: sem erro
- `cd frontend && npm run typecheck` -- expected: só o erro anterior em `ResetPassword.tsx(13,9)`, que não é desta história (`npm run lint` não roda)

**Manual checks (if no CLI):**
- No banco local de teste com duas versões do mesmo mês, subir a API e ver o aborto da migration; antes do push, o dono do produto confere a tabela de produção.
