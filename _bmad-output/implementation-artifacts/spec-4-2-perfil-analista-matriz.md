---
title: 'Story 4.2: Perfil analista e matriz de capacidades (AD-11)'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '6ba4ae0'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-4-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** As rotas decidem por lista de perfis soltas no código; não há perfil para quem carrega dados e edita Mapa/estrutura sem poder fechar o mês.

**Approach:** Perfil `analista` na coluna, capacidades tipadas e uma matriz perfil→capacidades num só arquivo (`handlers/perfis.go`). O middleware passa a aceitar uma capacidade (`AuditoriaAuthCap`) e ainda aceita a lista de perfis até a 4.3.

## Boundaries & Constraints

**Always:** Migration `014_perfil_analista.sql` idempotente (troca a CHECK de `users.perfil` por `users_perfil_check` com os cinco perfis; sem as palavras `already exists`/`duplicate` nas mensagens). Admin de plataforma (`users.role='admin'`) tem todas as capacidades. Validação sempre no backend. Rótulos: analista = Analista, controladoria = Controller, gestor = Gerente, diretoria = Diretoria, auditoria = Auditoria (valor gravado não muda).

**Never:** Não migrar rotas (4.3). Não implementar escopo por departamento (4.7). Não alterar o comportamento de nenhuma rota existente.

## Matriz final

| Capacidade | analista | controladoria | gestor | diretoria | auditoria |
|---|---|---|---|---|---|
| ler | x | x | | x | x |
| ler_no_escopo | x | x | x | x | x |
| ler_trilha | | x | | | x |
| carregar | x | x | | | |
| ajustar | | x | | | |
| editar_mapa | x | x | | | |
| editar_estrutura | x | x | | | |
| editar_dicionario | x | x | | | |
| configurar_rateio, aplicar_rateio, definir_limite | | x | | | |
| justificar | | x | x | | |
| fechar, reabrir, gerir_usuarios | | x | | | |

## Decisões

- `editar_mapa_estrutura_dicionario` dividida em `editar_mapa`, `editar_estrutura` (linhas/papel do DRE, departamentos, filiais gerenciais, agrupamentos) e `editar_dicionario`: a matriz fica mais honesta e uma separação futura não exige migração de rota.
- `ler` é a leitura sem restrição de escopo (detalhe por conta/lançamento, DRE por departamento, trimestres); `ler_no_escopo` é a leitura que todo perfil tem hoje nas telas gerais do DRE, e o `gestor` só tem esta (fail-closed até a 4.7). Quem tem `ler` tem também `ler_no_escopo`.
- `ler_trilha` (não estava na lista da história) existe porque hoje `/api/aud/audit-log` é de `controladoria` e `auditoria`; sem ela a auditoria perderia a trilha ou ganharia uma escrita que não tem. O analista não a recebe (hoje não lê a trilha; ele não existia).
- `ajustar`, `configurar_rateio`, `aplicar_rateio`, `definir_limite` e `justificar` ainda não protegem rota nenhuma (épicos 5+); ficam definidas e com dono na matriz (Controller; `justificar` também do Gerente).
- Forma do middleware: `AuditoriaAuthCap(db, next, area, leitura, escrita...)`; `leitura` vale para GET/HEAD/OPTIONS e `escrita` (opcional) para os demais métodos. `AudContext.Pode(cap)` para checagem dentro do handler. `/api/aud/me` devolve também `capacidades`.
- A lista de perfis da validação de `PUT /api/aud/usuarios` passa a vir da matriz (`analista` aceito).

## Tasks & Acceptance

- [x] `backend/migrations/014_perfil_analista.sql` -- CHECK com `analista`
- [x] `backend/handlers/perfis.go`, `auditoria_auth.go`, `aud_usuarios.go` -- capacidades, matriz, `Pode`, `AuditoriaAuthCap`
- [x] `backend/handlers/perfis_test.go` -- percorre a matriz; migração aceita analista; middleware por capacidade e por método, e lista legada
- [x] `frontend/src/lib/perfis.ts`, `AdmUsuarios.tsx` -- seletor com Analista e rótulos Controller/Gerente

**Acceptance Criteria:**
- Given `users.perfil='analista'`, when a migration roda, then a coluna aceita; perfil inventado segue recusado.
- Given a matriz, when o teste a percorre, then analista carrega e edita Mapa/estrutura sem fechar nem reabrir; diretoria e auditoria sem escrita; controladoria e admin com todas.

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...`
- `cd frontend && npm run typecheck && npm run build`

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Rotas ainda por lista de perfis | esperado | defer | 4.3 migra e remove a assinatura legada |
| Frontend ainda decide botões por `perfil === 'controladoria'` | low | defer | 4.3 ajusta os pontos em que o analista ganha acesso |
| Capacidades sem rota (ajustar, rateio, limite) | low | rejeitado | previstas na história; protegem rotas dos épicos seguintes |
