- source_spec: `_bmad-output/implementation-artifacts/spec-4-1-fechar-cadastro-publico.md`
  summary: O `LoginHandler` auto-provisiona ambiente "Ambiente de Testes", grupo e empresa para quem entra com credencial válida e não tem empresa; um usuário criado pelo administrador sem vínculo a ambiente cairia num ambiente compartilhado. A história 4.9 deve vincular o usuário ao ambiente e desligar esse caminho, e o critério ainda não consta nela.
  evidence: `backend/handlers/auth.go`, `LoginHandler` (bloco de auto-provisionamento, `INSERT INTO companies`); o teste `backend/public_routes_test.go` o aceita como exceção documentada porque exige conta existente e senha correta.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-1-fechar-cadastro-publico.md`
  summary: O `AGENTS.md` manda rodar `npm run typecheck` e `npm run lint` antes de commitar, mas o frontend não tem configuração do ESLint (o `npm run lint` falha) e o `typecheck` já acusa um erro em `ResetPassword.tsx` (variável `navigate` sem uso); também diz que não há testes Go, e agora há.
  evidence: na base limpa, `npm run lint` responde "ESLint couldn't find a configuration file" e `npm run typecheck` devolve `ResetPassword.tsx(13,9): error TS6133`; a correção edita o `AGENTS.md`, por isso fica adiada.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-1-fechar-cadastro-publico.md`
  summary: `frontend/src/pages/Register.tsx` ficou sem uso depois de a rota `/register` passar a redirecionar; pode ser apagado quando o cadastro público for descartado de vez.
  evidence: `App.tsx` não importa mais a página e nada mais referencia `/register`; foi mantido de propósito por ser reversível.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-1-testes-fechamento-importacao.md`
  summary: Nenhum teste de handler passa pelo `AuditoriaAuthMiddleware` nem verifica isolamento entre empresas (perfil e área negados, dado da empresa A invisível à B).
  evidence: os testes da 1.1 injetam `AudContext` direto na requisição; a política do projeto exige validação de perfil e áreas no backend.
