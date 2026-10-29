---
title: 'Story 2.12: Relatório de conferência entre origens'
type: 'feature'
created: '2026-10-29'
status: 'done'
baseline_commit: '35a9dd8'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/epics.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Há mais de uma origem de lançamentos (`csv`, `csv.contas` e, depois, o Oracle) e nada compara uma com a outra; o DRE soma todas, então uma conta carregada duas vezes passa despercebida.

**Approach:** Relatório só de leitura que soma o valor bruto de `lancamentos` por (filial, conta) em cada origem, na mesma competência, e lista as diferenças. Funciona com qualquer par de origens existentes; não depende do Oracle.

## Boundaries & Constraints

**Always:** `GET /api/aud/conferencia` (área empresa, capacidade `ler`): `competencia` (AAAA-MM) obrigatória, `origem_a` e `origem_b` obrigatórias e diferentes, `filial` opcional, `tolerancia` (padrão 0,01; aceita vírgula), `limite` (padrão 200, máximo 1000) e `offset`, `formato=csv`. `GET /api/aud/conferencia/origens` lista as origens da empresa. Valor bruto contra valor bruto, sem o sinal do Mapa e sem passar pelo DRE. Filial normalizada como a ingestão (só dígitos perde os zeros à esquerda). Devolve totais por origem (soma, pares distintos, linhas), pares comparados e dentro da tolerância, diferenças acima da tolerância (ordenadas pelo maior valor absoluto), contas só numa origem, resumo por linha do DRE (Mapa, `dre.MelhorPadrao`; contas sem Mapa numa linha à parte) e `fechada`. Aviso de dupla contagem quando as duas origens têm lançamentos na competência. Trilha: `visualizar` e `exportar` com os parâmetros. CSV com BOM, `;`, vírgula decimal e apóstrofo em texto que comece com `=`, `+`, `-`, `@`, tabulação ou retorno. Gestor recebe `403` (e a tentativa vai à trilha como `negado`). Toda consulta filtra por `empresa_id`.

**Never:** Não escrever em `lancamentos` nem criar tabela. Não aplicar o sinal do Mapa. Não ler o Oracle. Nenhum dado real em teste ou documento.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior |
|----------|--------------|---------------------------|
| Dentro da tolerância | diferença de 0,01 com tolerância 0,01 | não aparece nas diferenças; conta em `iguais` |
| Fora da tolerância | diferença maior | aparece, com a diferença A − B |
| Só em um lado | conta presente numa origem | aparece em `so_em_a` ou `so_em_b` |
| Filial com zero à esquerda | `01` numa origem, `1` na outra | mesma filial; várias linhas somam |
| Competência fechada | fechamento ativo | `fechada: true` |
| Gestor | perfil sem `ler` | `403`, trilha `negado` |
| Outra empresa | lançamentos de outra empresa | invisíveis |
| CSV | `formato=csv` | texto com `=` ganha apóstrofo; número negativo não |
| Origens iguais ou ausentes | `origem_a = origem_b` | `400` |

</frozen-after-approval>

## Code Map

- `backend/dre/conferencia.go` -- `Conferir`: uma consulta agrupada por (filial normalizada, conta, origem), comparação em Go, resumo pelo Mapa, fechamento.
- `backend/handlers/conferencia.go` -- `ConferenciaHandler`, `ConferenciaOrigensHandler`, CSV seguro.
- `backend/main.go` -- as duas rotas com `CapLer`; `backend/rotas_capacidades_test.go` -- linhas da tabela de rotas.
- `backend/handlers/conferencia_test.go` -- testes com banco.
- `frontend/src/pages/aud/Conferencia.tsx`, `lib/navigation.ts`, `App.tsx` -- aba "Conferência" em Administrativo.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo de `ResetPassword.tsx`

## Implementation Notes

Decisões de detalhe:
- O par só existe "nos dois lados" se as duas origens têm linha para ele, mesmo que a soma seja zero.
- Limite e `offset` valem para cada uma das três listas; o CSV ignora a paginação e traz tudo.
- A consulta de origens não grava trilha (é só catálogo, sem valores).
- A descrição vem do dicionário de contas (padrão do Mapa) e, sem ela, da descrição do lançamento.
- Pontos frágeis: a comparação é por filial do ERP, não por filial gerencial; a conta é comparada como texto exato, sem correspondência entre planos de contas diferentes; a consulta lê todas as linhas das duas origens na competência (sem limite), adequado ao volume de um mês por empresa.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Número negativo no CSV não pode ganhar apóstrofo | medium | patch | só células de texto passam por `celulaSegura`; teste com diferença negativa |
| Comparação em ponto flutuante geraria falsas diferenças de centavo | medium | patch | somas arredondadas a centavos antes de comparar |
| Correspondência de conta entre planos e filial gerencial | low | defer | `deferred-work.md` |
