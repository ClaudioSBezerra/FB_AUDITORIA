---
title: 'Story 4.9: Criar usuário da empresa (FR-19)'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: 'd67fcc2'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/spec-4-5-usuarios-com-departamentos.md'
  - '{project-root}/_bmad-output/implementation-artifacts/spec-4-6-bloqueio-de-usuario.md'
  - '{project-root}/_bmad-output/implementation-artifacts/spec-4-1-fechar-cadastro-publico.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Com o cadastro público fechado (4.1), não há como o Gerente entrar. O `CreateUserHandler` pede senha ao administrador e não conhece perfil, áreas nem departamentos.

**Approach:** `POST /api/admin/users/empresa` (administrador de plataforma): e-mail, nome, perfil, áreas, departamentos (só `gestor`, mesma validação da 4.5) e a empresa. O usuário é vinculado à empresa e recebe um link para definir a própria senha, pelo mecanismo de redefinição existente. Sem SMTP, a tela mostra o link.

## Boundaries & Constraints

**Always:** Só `users.role = 'admin'` (token e cadastro conferidos); os demais, `403`. E-mail normalizado em minúsculas e único sem diferenciar maiúscula (`409`, inclusive se existir em outra empresa). Perfil fora da matriz, e-mail ou nome inválido, área inválida/vazia, departamento fora da empresa, ligado a perfil que não é `gestor`: `400`; empresa inexistente: `404`. Nada é gravado em erro. Criação, vínculo ao ambiente, departamentos, token e trilha na MESMA transação. Trilha: entidade `usuario`, ação `criar`, `empresa_id` da empresa alvo, autor = administrador; payload com e-mail, nome, perfil, áreas, códigos dos departamentos e validade; NUNCA senha, hash nem token. Sem `trial_ends_at`.

**Never:** Não aceitar senha no pedido. Não criar o perfil de TI. Não devolver o link quando o e-mail foi enviado.

## Decisões

- **Tela:** a de plataforma, `AdminUsers` (`/config/usuarios`), a que o administrador de plataforma já usa; o formulário "Novo usuário da empresa" (`NovoUsuarioEmpresa.tsx`) escolhe ambiente → grupo → empresa e, para o Gerente, os departamentos dessa empresa (`GET /api/admin/users/empresa?company_id=`, porque a listagem `/api/aud/departamentos` só serve a empresa ativa do administrador). O botão antigo virou "Novo usuário com senha" (cria administradores e novos ambientes); aposentá-lo fica para depois.
- **Vínculo à empresa:** o mesmo critério de `AudUsuariosHandler` ("usuários da empresa" = `user_environments` do ambiente do grupo da empresa), com `preferred_company_id` = a empresa, de modo que o login cai nela e o auto-provisionamento do `LoginHandler` nunca é alcançado por esse usuário. O auto-provisionamento em si NÃO foi desligado (o admin de plataforma pode depender dele); fica registrado em `deferred-work.md`.
- **Senha:** o usuário nasce com o hash bcrypt de 32 bytes aleatórios descartados (ninguém conhece o valor); só loga depois de definir a senha pelo link. `is_verified = true`, `role = 'user'`.
- **Token:** reaproveita `verification_tokens` (tabela, não memória), `type = 'password_reset'`, consumido por `ResetPasswordHandler` (`/api/auth/reset-password`, tela `/reset-senha?token=`); `expires_at` = 72 horas (o mecanismo já honra `expires_at`), uso único. O token fica em claro na tabela, como o do esqueci-a-senha.
- **Envio:** com SMTP (`SMTP_USER` e `SMTP_PASSWORD`) e `APP_URL`, envia o e-mail (`services.SendInviteEmail`) e a resposta não traz o link (`email_enviado: true`). Sem SMTP, sem `APP_URL` ou com falha de envio, a resposta traz `link` (com `APP_URL`) e `caminho` (sem ele, a tela prefixa a origem do navegador) e `motivo_sem_envio`. O resultado do envio vai à trilha como `convite` (`enviado`, `motivo`), sem o link.

## Tasks & Acceptance

- [x] `backend/handlers/admin_usuario_empresa.go`; rota em `main.go`; `departamentosPedidos`/`departamentosSaoDaEmpresa` extraídas de `aud_usuarios.go` (4.5) e reaproveitadas
- [x] `backend/services/email.go` -- `SMTPConfigurado`, `LinkDefinirSenha`, `CaminhoDefinirSenha`, `SendInviteEmail`
- [x] `backend/handlers/admin_usuario_empresa_test.go` -- cria vinculado (empresa, perfil, áreas, departamentos), link define a senha e permite logar, sem login antes, uso único, e-mail repetido 409 sem diferenciar maiúscula, validações 400/404 sem gravar, não-admin 403 (inclusive token antigo), sem prazo, trilha sem segredo, outra empresa não vê, envio com/sem SMTP
- [x] `frontend`: `NovoUsuarioEmpresa.tsx`, `AdminUsers.tsx`

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...`
- `cd frontend && npm run typecheck && npm run build`

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Token em claro na tabela `verification_tokens` | low | defer | padrão existente do esqueci-a-senha; guardar só o hash exige mexer nos dois fluxos |
| O auto-provisionamento do `LoginHandler` continua ligado | med | defer | o usuário criado pela 4.9 sempre tem empresa; desligar pode quebrar o admin de plataforma sem vínculo; `deferred-work.md` |
| O link só serve a quem o recebeu; reenviar o convite exige criar de novo ou usar "esqueci a senha" | low | defer | sem tela de reenvio nesta história |
| Quem reutiliza um e-mail de usuário já existente em outra empresa recebe 409 | low | rejeitado | `users.email` é único na plataforma; incluir o usuário em outra empresa é outra história |
| `CreateUserHandler` (senha informada) continua existindo | low | defer | cria administradores e ambientes; aposentar quando o fluxo novo cobrir tudo |
| Dependência circular de import entre `AdminUsers` e `NovoUsuarioEmpresa` (o seletor de hierarquia é exportado de `AdminUsers`) | low | rejeitado | só usa funções em tempo de render; build e tela funcionam |
