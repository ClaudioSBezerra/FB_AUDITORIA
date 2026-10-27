---
title: 'Story 4.1: Fechar o cadastro público (FR-19, AR-14)'
type: 'feature'
created: '2026-10-27'
status: 'done'
route: 'oneshot'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-4-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** `POST /api/auth/register` é público e cria usuário, ambiente e empresa de teste para qualquer pessoa na internet. O site já está em produção atrás do Cloudflare Tunnel.

**Approach:** Fechar o cadastro por padrão com a variável `ALLOW_SELF_REGISTER` (só o valor `true` o reabre, para desenvolvimento). Fechado, a rota responde `404` antes de ler o corpo ou tocar o banco. No frontend, `/register` redireciona para o login. Um teste de código-fonte lista as rotas públicas do `main.go` e garante que nenhuma outra cria usuário ou empresa. O padrão `false` consta em `backend/.env.example` e em `docker-compose.yml`. A história sai sozinha, antes de qualquer outra, e só com commit local (sem push, que dispararia deploy).

</frozen-after-approval>

## Implementation Notes

Plano e decisões do agente (antes de implementar):

- `backend/handlers/auth.go`: função `SelfRegisterEnabled()` (lê `ALLOW_SELF_REGISTER`, só `true`, sem diferenciar maiúsculas, reabre) e guarda no início de `RegisterHandler`, com `http.NotFound`. Isso permite testar com `RegisterHandler(nil)`, sem banco.
- `backend/handlers/auth_register_test.go`: fechado por padrão (variável ausente), fechado com `false`, e aberto com `true` (corpo inválido devolve `400`, o que prova que a guarda não bloqueou e que o banco não foi tocado).
- `backend/public_routes_test.go` (pacote `main`): lê `main.go`, compara o conjunto de rotas públicas com uma lista permitida e, para cada handler público, confere que o corpo não tem `INSERT INTO users`. `RegisterHandler` precisa chamar `SelfRegisterEnabled`.
- Decisão: o `LoginHandler` cria ambiente, grupo e empresa para quem já fez login com credencial válida e não tem empresa (auto-provisionamento herdado do Farol). Não é cadastro público (exige conta existente e senha correta), então fica como exceção explícita e documentada no teste. **Risco registrado para a história 4.9:** um usuário criado pelo administrador sem vínculo a ambiente cairia nesse auto-provisionamento e entraria num ambiente "Ambiente de Testes" compartilhado; a 4.9 deve vincular o usuário ao ambiente e desligar esse caminho.
- `frontend/src/App.tsx`: trocar a rota `/register` por `<Navigate to="/login" replace />` e remover o `import` preguiçoso de `Register`. O arquivo `Register.tsx` fica (sem uso), por ser reversível.
- `backend/.env.example` e `docker-compose.yml`: `ALLOW_SELF_REGISTER` com padrão `false`.
- Verificação: `go build ./...`, `go vet ./...`, `go test ./...` no `backend/` e `npm run typecheck`, `npm run lint` no `frontend/`.

Resultado da implementação:

- Arquivos: `backend/handlers/auth.go` (`SelfRegisterEnabled` e a guarda), `backend/main.go` (linha de log com o estado do cadastro na inicialização), `backend/handlers/auth_register_test.go`, `backend/public_routes_test.go`, `backend/.env.example`, `docker-compose.yml`, `frontend/src/App.tsx`.
- Verificado: `go build`, `go vet` e `go test` passam; `vite build` passa; o compose renderiza `ALLOW_SELF_REGISTER: "false"` sem a variável definida. Checagem de mutação: com a guarda desligada, `TestRegisterFechadoPorPadrao` e `TestRotasPublicasNaoCriamUsuario` falham; com um registro `http.HandleFunc(` invisível, a contagem falha.
- Problemas anteriores à mudança, confirmados na base limpa e fora do escopo: o `typecheck` acusa `ResetPassword.tsx(13,9)` (variável `navigate` sem uso) e o frontend não tem configuração do ESLint, então o `npm run lint` não roda. Ambos vão para `deferred-work.md`.
- Nenhum push foi feito, porque o push dispara o deploy.

## Review Triage Log

- Register.tsx sem uso: `low`. Foi mantido de propósito por ser reversível; rejeitado e registrado em `deferred-work.md`.
- Frontend sempre redireciona, backend reabrível: `low`. Corrigido o texto do `.env.example` ("reabre a rota da API; a tela sempre redireciona").
- Testes sem caminho de sucesso, outros métodos ou vazamento no corpo: `low`. O caminho de sucesso exige banco (história 1.1) e o handler responde 404 a qualquer método; rejeitado.
- `RegisterHandler(nil)` entra em pânico se a guarda mudar de lugar: `false`. Falhar alto é o objetivo, e a checagem de mutação mostrou o teste falhando.
- Teste frágil por regex: `medium` para o registro em várias linhas (corrigido com a contagem de `http.HandleFunc(` contra as linhas lidas, e testado por injeção) e para o SQL em outra caixa ou com schema (corrigido com `(?is)insert\s+into\s+(?:\w+\.)?users`). `low` para usuário criado por função auxiliar: só um parse de AST pegaria, e isso custa mais do que vale; rejeitado.
- Comentário desatualizado em `public_routes_test.go`: `low`. Corrigido.
- Rotas multilinha nunca conferidas contra criação de usuário: `medium`. Corrigido: `main.go` inteiro é conferido por `criaUsuario`.
- 404 sem log, variável lida a cada requisição, sem aviso na inicialização: `low` para o log da tentativa (varreduras gerariam ruído e uma tentativa recusada não é escrita) e para o aviso de validação; rejeitados. A linha de log do estado na inicialização foi adicionada.
- "Não há caminho de administrador para criar usuários": `false`. Existem `POST /api/admin/users/create` e a tela `AdminUsers.tsx`. A exceção do auto-provisionamento do login só estava num comentário de teste: `medium`, adiada para `deferred-work.md` (a história 4.9 deve tratá-la).

