---
title: 'Story 4.8: "Meu departamento" para o Gerente (FR-18, FR-8)'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: 'b904978'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/spec-4-7-escopo-por-departamento.md'
  - '{project-root}/_bmad-output/implementation-artifacts/spec-3-5-dre-por-departamento.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O Gerente (`gestor`) já tem escopo por departamento (4.7), mas só a visão genérica "DRE por departamento", sem a base do grupo, sem Orçado, Ano anterior nem variação.

**Approach:** Rota própria `GET /api/aud/meu-departamento` e tela "Meu departamento", landing do Gerente: contas dos seus departamentos com Realizado, Orçado, Ano anterior, variação (valor e %), % sobre o Faturamento Líquido calculado no servidor sem devolver a base, totais por departamento e total do escopo, mensal e acumulado, com o recorte de filial/agrupamento/Grupo.

## Boundaries & Constraints

**Always:** Escopo só do servidor (`escopoDe`); `?departamento=` só restringe aos próprios (alheio: `403` registrado como `negado`). Mês fechado lê `dre_fechamento_contas` (valor, ano anterior e departamento congelados); mês aberto, ao vivo. A base (Faturamento Líquido) é lida no servidor para o percentual e nunca vai no corpo; o corpo traz só `pct_status`.

**Never:** Zero falso ou rateio inventado para Orçado ou Ano anterior ausente. Mostrar "Sem departamento" ao Gerente.

## Decisões

- **Quem usa a rota:** só `gestor` (e não o admin de plataforma). A rota passa pelo portão `ler_no_escopo` e o handler responde `403` (registrado) a controladoria, analista, diretoria, auditoria e admin de plataforma: a visão é pessoal (escopo = departamentos do próprio usuário), esses perfis usam o "DRE por departamento" e não há parâmetro para ver o escopo de outro usuário. É a opção mais segura.
- **Ano anterior por conta existe:** `dre_fechamento_contas.ano_anterior` guarda o ano anterior por conta no fechamento; em mês aberto, o mesmo período do ano anterior é lido dos lançamentos pelo mesmo mapa. Conta sem movimento no ano anterior (ou só com soma zero) devolve `null` ("sem ano anterior"); o total soma só as contas que têm. Mês de fechamento legado (sem contas): ano anterior `null` para tudo e aviso.
- **Orçado por conta não existe:** `dre_orcado` é por linha e filial (a leitura por conta é a história 2.7). Orçado e variação sobre o orçado ficam `null` em todo o corpo, a tela mostra "sem orçado" e o corpo traz `orcado_por_conta: false`. Nada é rateado da linha para as contas. Pendência registrada em `deferred-work.md`.
- **Variação** = Realizado − base; percentual = variação / |base|; base nula → ambos `null`; base zero → valor sim, percentual `null` ("não calculável").
- Percentual sobre o Faturamento Líquido: `baseFaturamentoLiquido` (extraída de `DREDepartamentosHandler`, que a reaproveita) lê o recorte (snapshot nos meses fechados); sem papel, `pct_status: sem_papel` + aviso; base zero, `zero`.
- **Menu:** aba "Meu departamento" (`gerenteOnly`) só para `gestor` não admin; é a landing do Gerente; "DRE por departamento" sai do menu e da rota do Gerente (a API `/dre/departamentos` continua com escopo da 4.7). `qLegado` e o diálogo de lançamentos antigos saíram de `DRE.tsx`.

## Tasks & Acceptance

- [x] `backend/dre/meu_departamento.go` (`ContasComparativo`, `VariacaoSobre`, `MontarMeuDepartamento`) + teste
- [x] `backend/handlers/meu_departamento.go`, `escopo.go` (`baseFaturamentoLiquido`), `dre.go`, `main.go`
- [x] `backend/handlers/meu_departamento_test.go` -- A e B, variação, null sem orçado/ano anterior, mês fechado vs aberto (departamento e valores congelados), legado, sem departamento, sem papel, departamento alheio 403, perfis que não são gestor 403, outra empresa, base fora do corpo
- [x] `rotas_capacidades_test.go`, `escopo_rotas_test.go` (varredura passa a conhecer `ContasComparativo` e `MeuDepartamentoHandler`)
- [x] `frontend`: `MeuDepartamento.tsx`, `App.tsx`, `navigation.ts`, limpeza de `DRE.tsx`

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...`
- `cd frontend && npm run typecheck && npm run build`

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| O percentual (conta/total) com o valor da conta permite ao Gerente deduzir a base (valor / pct) | med | defer | inerente ao critério "percentual no servidor sem devolver a base"; se o cliente considerar sigilo, arredondar ou ocultar o percentual; `deferred-work.md` |
| Orçado por conta não existe; tela mostra "sem orçado" em tudo | high | defer | depende da 2.7 (`orcado_conta`); `deferred-work.md` |
| Ano anterior "zero" não se distingue de "sem movimento" (o snapshot guarda `NUMERIC NOT NULL`) | low | rejeitado | ambos exibem "sem ano anterior"; para fluxo contábil é a mesma informação |
| `/api/aud/dre/departamentos` ainda responde ao Gerente (restrito, sem percentuais) | low | rejeitado | segue com escopo da 4.7 e coberta por testes; só saiu do menu |
| Conta que muda de departamento depois do fechamento aparece em dois departamentos em períodos mistos | low | rejeitado | cada parte vem do departamento congelado ou vivo do seu mês; mesmo comportamento da 3.5 |
