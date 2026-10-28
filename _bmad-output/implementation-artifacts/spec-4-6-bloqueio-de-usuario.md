---
title: 'Story 4.6: Bloqueio de usuário no lugar da expiração (FR-22, AD-19)'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: 'a87fdf4'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/spec-4-5-usuarios-com-departamentos.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O acesso termina por data (`trial_ends_at`). No login, `COALESCE(trial_ends_at, NOW())` transforma a coluna vazia em "vencido agora": um usuário comum criado sem a data não consegue entrar. O Controller não tem como cortar um acesso por decisão.

**Approach:** Bloqueio com motivo (`users.bloqueado_em/bloqueado_por/motivo_bloqueio`), checado no login, na renovação de sessão e a cada requisição autenticada. A verificação por `trial_ends_at` sai do login e do middleware.

## Boundaries & Constraints

**Always:** Bloqueado: login `403` com mensagem e motivo (só depois da senha certa, para não revelar a conta a quem não a tem); renovação de sessão `403` (e o refresh token é descartado); sessão aberta `403` na requisição seguinte, em **toda** rota autenticada. O bloqueio vale também para o admin de plataforma. `POST /api/aud/usuarios/bloquear` e `/desbloquear` `{user_id, motivo}`: capacidade `gerir_usuarios`; motivo obrigatório (até 500 caracteres); trilha na mesma transação (entidade `usuario`, ações `bloquear`/`desbloquear`, motivo e antes/depois); o próprio usuário não se bloqueia (`400`); o último administrador de plataforma ativo não é bloqueado (`409`); outra empresa `404`; bloquear quem já está bloqueado, ou desbloquear quem não está, `409`. Migration `016_bloqueio_usuario.sql` sem as palavras `already exists`/`duplicate`.

**Never:** Não apagar a coluna `trial_ends_at` (dados existentes). Nenhum código a lê ou grava mais. Não bloquear por data.

## Decisões

- **Onde o JWT é validado e como o bloqueio vale em tudo:** `AuthMiddleware(db, next, role)` agora recebe o banco e consulta o bloqueio depois de validar o JWT; a validação pura virou `authJWT` (privada). `autenticaAud` usa `authJWT` e checa o bloqueio na própria consulta do perfil (uma consulta só). `TestAuthJWTSoEUsadaPorQuemChecaBloqueio` falha se outro arquivo chamar `authJWT` direto. Cobre as rotas `/api/auth/me`, `change-password`, `/api/user/*`, `/api/admin/*`, `/api/config/*`, logo e `/api/aud/*`.
- Usuário que não existe mais (token antigo) passa a receber `401` em vez de seguir adiante.
- Só o administrador de plataforma bloqueia outro administrador de plataforma (Controller comum recebe `403`); a regra do último ativo trava as linhas de admin (`FOR UPDATE`) para duas requisições não bloquearem os dois.
- `bloqueado_por` é `ON DELETE SET NULL`: a FK é de `users` para `users`; o trigger de imutabilidade é só do `audit_log` e não é tocado (teste `TestBloqueio_BloqueadoPorSomeSemQuebrarABloqueado`).
- Bloquear apaga os refresh tokens do usuário (memória do processo). O access token em uso (30 minutos) morre na requisição seguinte pela consulta de bloqueio.
- Remoção do trial: `GenerateToken`/login/`/auth/me` sem `trial_ends_at`; `CreateUserHandler` e o registro público (fechado pela 4.1, mas ainda presente) não gravam mais a data; `PromoteUserHandler` ignora `extend_days`/`is_official` (só altera o papel). `User.trial_ends_at` saiu do JSON.
- Frontend: tela de usuários da empresa (`AdmUsuarios`) ganha a coluna Bloqueado (com motivo) e Bloquear/Desbloquear com campo de motivo; a tela de plataforma (`AdminUsers`) troca "Trial" por "Bloqueado" (só leitura) e perde "Estender (dias)" e "Cliente Oficial"; o menu do usuário não mostra mais "Licença vence".

## Tasks & Acceptance

- [x] `backend/migrations/016_bloqueio_usuario.sql`
- [x] `backend/handlers/auth.go`, `auditoria_auth.go`, `admin.go`, `main.go` -- bloqueio em login, refresh, middleware; trial removido
- [x] `backend/handlers/aud_usuarios_bloqueio.go` e rotas em `main.go`; GET de usuários traz o bloqueio
- [x] `backend/handlers/bloqueio_test.go` -- login com trial vazio e com trial vencido entra; bloqueado é recusado; sessão aberta 403; refresh recusado; motivo obrigatório; trilha; auto-bloqueio; último admin; outra empresa; capacidade
- [x] `backend/arquitetura_test.go`, `rotas_capacidades_test.go`
- [x] `frontend` -- AdmUsuarios, AdminUsers, AppRail, AuthContext

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...`
- `cd frontend && npm run typecheck && npm run build`

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Cada requisição autenticada fora de `/api/aud` ganha uma consulta (chave primária) ao banco | low | rejeitado | custo desprezível; é o preço de o bloqueio valer na próxima requisição |
| Refresh tokens e blacklist ficam em memória: reinício derruba as sessões, mas o bloqueio não depende disso (vem do banco) | low | rejeitado | `estadoDeBloqueio` lê `users` sempre |
| Coluna `trial_ends_at` segue no banco | low | defer | `deferred-work.md`: remover numa limpeza futura de schema |
| Bloqueio de administrador de plataforma só pelo próprio admin; não há tela de bloqueio na área de plataforma | low | defer | `AdminUsers` mostra o estado; bloquear/desbloquear pela API da empresa ativa |
