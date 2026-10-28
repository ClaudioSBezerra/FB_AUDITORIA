---
title: 'Story 2.9: Cadastro de departamentos'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: 'bd46465'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Contas e gerentes precisam se ligar a departamentos, mas não existe cadastro deles.

**Approach:** CRUD de departamentos (código, nome, grupo) por empresa, no serviço `dre.Dicionario`, com tela em Configurações. A ligação conta-departamento e a recusa de exclusão de departamento ligado a contas vêm na 2.10.

## Boundaries & Constraints

**Always:** Migration `009_departamentos.sql`: `auditoria.departamentos (id UUID PK DEFAULT gen_random_uuid(), empresa_id UUID NOT NULL REFERENCES companies(id), codigo TEXT NOT NULL, nome TEXT NOT NULL, grupo TEXT NOT NULL CHECK IN ('Comercial','Logística','Administrativo'), UNIQUE (empresa_id, codigo))`, mais `UNIQUE (id, empresa_id)` para a FK composta da 2.10. Recriar `auditoria.limpar_dados_teste` (versão da 008 inteira, mais o `DELETE` da tabela nova e a contagem no JSON). Mensagens de erro sem `already exists` nem `duplicate`. Serviço `dre.Dicionario` (`ListarDepartamentos`, `SalvarDepartamento`, `ExcluirDepartamento`) com trilha (`trilha.Gravar`, entidade `departamento`) na mesma transação. Rota `/api/aud/departamentos` na área `configuracoes`: `GET` para qualquer perfil da área; `POST`/`PUT`/`DELETE ?id=` só `controladoria` (`403` para os demais). Código e nome obrigatórios, grupo da lista (`400`); código repetido na empresa `409`; id inexistente `404`. Tela: aba "Departamentos" (`/configuracoes/departamentos`, perfis `controladoria` e `auditoria`).

**Never:** Nenhum departamento, código ou nome real em arquivo versionado (testes usam `D1`, `D2`). Não editar migration já aplicada. O `409` de exclusão por conta ligada não é implementado aqui: a tabela `conta_departamento` não existe; fica o ponto de extensão `verificarUsoDepartamento` (retorna nil), preenchido e testado na 2.10.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Criar | controladoria, `D1`, `Comercial` | grava, lista mostra, trilha registra | N/A |
| Código repetido | segundo `D1` na empresa (ou renomear para `D1`) | nada muda | `409` |
| Mesmo código, outra empresa | `D1` em outra empresa | grava | N/A |
| Grupo fora da lista / código ou nome vazio | `Nada` / `""` | nada grava | `400` |
| Editar / excluir id inexistente, inválido ou de outra empresa | | nada muda | `404` |
| Perfil sem escrita | diretoria faz `POST`/`DELETE` | nada gravado | `403` |
| Limpeza de teste | `limpar_dados_teste` | apaga os departamentos da empresa | N/A |

</frozen-after-approval>

## Code Map

- `backend/migrations/009_departamentos.sql` -- tabela e nova `limpar_dados_teste` (cópia da 008 mais a tabela).
- `backend/dre/dicionario.go` -- serviço, erros tipados, ponto de extensão do 409.
- `backend/handlers/departamentos.go` -- handler fino e `erroDicionario` (reaproveitado na 2.10); rota em `backend/main.go`.
- `frontend/src/pages/aud/AdmDepartamentos.tsx`, `App.tsx`, `lib/navigation.ts` -- aba e tela.
- Testes: `backend/dre/dicionario_test.go`, `backend/handlers/departamentos_test.go`; `TestAmbiente_LimpezaCobreTodaTabelaDeDominio` cobre a tabela nova (confirmado).

## Tasks & Acceptance

- [x] Migration, serviço, handler, rota, tela, testes.

**Acceptance Criteria:**
- Given o cadastro, when o analista cria, edita ou lista, then funciona e grava na trilha.
- Given `go build`, `go vet`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`), when rodam, then passam.

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo; `npm run lint` falha por falta de configuração do ESLint (já registrado em `deferred-work.md`)

## Implementation Notes

Decisão: o 409 de exclusão com contas ligadas é entregue na 2.10 (a tabela ainda não existe), conforme combinado; o critério da 2.9 fica completo só com a 2.10.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Critério "409 ao excluir ligado" da 2.9 sem implementação nesta história | medium | defer | ponto de extensão `verificarUsoDepartamento`; implementado e testado na 2.10 |
| `GET` aberto a qualquer perfil da área | low | rejeitado | segue o padrão das filiais gerenciais |
| Corrida entre editar código e carga do dicionário | low | rejeitado | o vínculo guarda o id, não o código |
