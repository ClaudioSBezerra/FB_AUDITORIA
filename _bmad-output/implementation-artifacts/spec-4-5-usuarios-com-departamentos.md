---
title: 'Story 4.5: Usuários com perfil e departamentos (FR-18, AD-10)'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: 'f2ad047'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/spec-4-3-rotas-por-capacidade.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O perfil `gestor` existe, mas não há como dizer a que departamentos o Gerente pertence; sem isso a 4.7 não tem escopo para calcular.

**Approach:** Tabela `auditoria.usuario_departamentos` e, em `PUT /api/aud/usuarios`, a lista de departamentos do Gerente. Só o perfil `gestor` tem departamentos. Toda alteração de perfil ou de departamento vai à trilha com antes e depois, na mesma transação.

## Boundaries & Constraints

**Always:** Só a capacidade `gerir_usuarios` acessa (middleware e handler). Migration `015_usuario_departamentos.sql` com `empresa_id`, FK composta ao departamento da mesma empresa, mensagens sem `already exists`/`duplicate`. `departamentos` no PUT: ausente mantém os vínculos; presente (mesmo `[]`) substitui. Ligar departamento a perfil que não é `gestor` é `400` e nada é gravado. Mudar o perfil de `gestor` para outro apaga os vínculos (em todas as empresas do usuário) e a trilha mostra antes e depois. Departamento de outra empresa ou id inválido: `400`. Trilha: entidade `usuario`, ação `alterar_perfil`, payload `{antes:{perfil,areas,departamentos[codigo]}, depois:{...}}`.

**Never:** Não calcular escopo nem mudar rota de leitura (4.7). Não guardar dado real de cliente.

## Decisões

- **`limpar_dados_teste` não muda.** A tabela é configuração; o teste `TestAmbiente_LimpezaCobreTodaTabelaDeDominio` passou a listá-la em `mantidas`. O vínculo sai por `ON DELETE CASCADE` quando a limpeza apaga `departamentos` (que já apagava), então a função não precisou ser recriada e o usuário continua existindo. Teste: `TestUsuarios_LimpezaDeTesteContinuaFuncionando`.
- `ON DELETE CASCADE` também no departamento excluído pela tela (o Gerente perde aquele departamento, falha para o lado seguro) e no usuário.
- Perfil trocado para não-gestor apaga vínculos de todas as empresas (o perfil é global em `users`), para não ressuscitarem se o usuário voltar a ser Gerente.
- GET devolve `departamentos` (`id, codigo, nome`) de cada usuário da empresa ativa; sempre lista, nunca `null`.
- Frontend: para `gestor` a linha mostra os departamentos (de `/api/aud/departamentos`) como caixas de seleção e avisa quando não há nenhum.

## Tasks & Acceptance

- [x] `backend/migrations/015_usuario_departamentos.sql`
- [x] `backend/handlers/aud_usuarios.go` -- GET com departamentos; PUT com validação, vínculos e trilha antes/depois na transação
- [x] `backend/handlers/aud_usuarios_test.go` -- dois departamentos, perfil não-gestor recusa, departamento alheio/ inválido, troca de perfil limpa, só `gerir_usuarios`, limpeza de teste, FK de mesma empresa
- [x] `frontend/src/pages/aud/AdmUsuarios.tsx`

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...`
- `cd frontend && npm run typecheck && npm run build`

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Excluir um departamento tira o vínculo dos Gerentes sem aviso na tela | low | defer | a exclusão de departamento já vai à trilha; mostrar quantos Gerentes serão afetados fica para depois |
| Troca de perfil global afeta o usuário em todas as empresas | low | rejeitado | `users.perfil` já é global desde a 4.2; registrado como decisão |
