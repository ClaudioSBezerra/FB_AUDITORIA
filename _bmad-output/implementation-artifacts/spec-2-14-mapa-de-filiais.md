---
title: 'Story 2.14: Mapa de filiais (filial do ERP para filial gerencial)'
type: 'feature'
created: '2026-10-28'
status: 'draft'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O cliente definiu que algumas filiais do ERP formam uma filial gerencial (a matriz agrupa várias; duas outras agrupam duas cada) e que as demais valem por si. Não existe cadastro disso: a ingestão e o DRE só conhecem a filial do ERP.

**Approach:** Cadastro (CRUD) de filiais gerenciais, cada uma com as filiais do ERP que a compõem, em tabelas por empresa, com tela em Configurações e um resolvedor `filial do ERP → filial gerencial` no pacote `dre` para as histórias seguintes. Esta história não muda a leitura do DRE, o snapshot nem a ingestão.

## Boundaries & Constraints

**Always:** Migration `008_filiais_gerenciais.sql`: `auditoria.filiais_gerenciais (id UUID PK DEFAULT gen_random_uuid(), empresa_id UUID NOT NULL REFERENCES companies(id), codigo TEXT NOT NULL, nome TEXT NOT NULL, UNIQUE (empresa_id, codigo))` e `auditoria.filiais_gerenciais_erp (empresa_id UUID NOT NULL REFERENCES companies(id), filial_gerencial_id UUID NOT NULL REFERENCES auditoria.filiais_gerenciais(id) ON DELETE CASCADE, filial_erp TEXT NOT NULL, PRIMARY KEY (empresa_id, filial_erp))`, de modo que uma filial do ERP pertence a uma só filial gerencial. Recriar `auditoria.limpar_dados_teste` (mesma função da 007, mais os `DELETE` das duas tabelas novas, filha antes da mãe, e as contagens no JSON). Mensagens de erro sem `already exists` nem `duplicate`. Serviço `dre.Filiais` (`Listar`, `Salvar`, `Excluir`) e resolvedor `dre.FilialGerencialDe(ctx, q, empresa, filialERP)`: devolve o código da filial gerencial que contém a filial do ERP, ou a própria filial do ERP se não houver ligação (filial solta). Rota `/api/aud/filiais-gerenciais` na área `configuracoes`: `GET` lista cada filial gerencial com as filiais do ERP (qualquer perfil da área); `POST`/`PUT` salvam (código, nome e lista de filiais do ERP, substituindo a lista de forma atômica) e `DELETE ?id=` exclui; escrita só para `controladoria` (`403` para os demais). Toda escrita grava em `writeAuditLogTx` na mesma transação (entidade `filial_gerencial`). Filial do ERP já ligada a outra filial gerencial: `409` dizendo a qual (pelo código). Código e nome obrigatórios (`400`); filiais do ERP aparadas, sem vazio e sem repetição na lista. Excluir uma filial gerencial é recusado com `409` se qualquer uma das filiais do ERP dela aparecer em `lancamentos`, `dre_orcado` ou `dre_fechamentos`/`dre_fechamento_linhas`/`dre_fechamento_contas` da empresa. Toda leitura e escrita filtra por `empresa_id`. Frontend: aba "Filiais" em Configurações (`/configuracoes/filiais`, perfis `controladoria` e `auditoria`) com lista, criação, edição e exclusão; os códigos reais entram pela tela.

**Never:** Nenhum código de filial, nome ou composição real em arquivo versionado, seed ou teste (use códigos fictícios como `G1`, `100`, `101`). Não alterar `DREHandler`, `Leitor`, `Fechar`, o snapshot da 1.8 nem a ingestão (a troca da chave do snapshot para a filial gerencial é história à parte, registrada em `deferred-work.md`). Não criar migration que dependa de dado de produção. Não editar migration já aplicada. Produção só recebe por push; decisão humana.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Criar grupo | controladoria, `G1`, `100`,`101` | grava, lista mostra as duas filiais, trilha registra | N/A |
| Filial já ligada | `101` já está em `G1`; tentar ligar em `G2` | nada muda | `409` citando `G1` |
| Substituir lista | editar `G1` para `100`,`102` | `101` fica livre, `102` ligada, atômico | N/A |
| Filial solta | `FilialGerencialDe('999')` sem ligação | `'999'` | N/A |
| Filial em grupo | `FilialGerencialDe('101')` | `'G1'` | N/A |
| Excluir usado | `100` tem lançamento | nada muda | `409` |
| Excluir livre | nenhum uso | exclui o grupo e as ligações | N/A |
| Perfil sem escrita | diretoria faz `POST` | `403`, nada gravado | N/A |
| Outra empresa | mesma filial `100` em outra empresa | não conflita nem aparece | N/A |
| Código ou nome vazio | `""` | `400` | mensagem |
| Código repetido | segundo `G1` na mesma empresa | `409` | mensagem |

</frozen-after-approval>

## Code Map

- `backend/migrations/008_filiais_gerenciais.sql` -- novo; base é a `limpar_dados_teste` de `007_fechamento_snapshot.sql:80-114` (copiar inteira e acrescentar as duas tabelas).
- `backend/dre/filiais.go` -- novo; serviço e resolvedor; padrão de transação e trilha de `backend/dre/fechamento.go` (`trilha.Gravar`).
- `backend/handlers/filiais_gerenciais.go` -- novo; handler fino no padrão de `DREMapaHandler` (`backend/handlers/dre.go:363`): `GetAudContext`, `c.Tem("controladoria")`, `http.MaxBytesReader`, `log.Printf` nos 500.
- `backend/main.go:~425` -- registrar a rota com `withAud(..., handlers.AreaConfiguracoes)` ao lado de `/api/aud/usuarios`.
- `frontend/src/lib/navigation.ts`, `frontend/src/App.tsx` (rotas de `/configuracoes/*`) -- aba e rota; página nova `frontend/src/pages/aud/AdmFiliais.tsx` no estilo de `AdmUsuarios.tsx` (`api` de `./util`, `useQuery`, `sonner`).
- `backend/handlers/ambiente_test.go` (`TestAmbiente_LimpezaCobreTodaTabelaDeDominio`) -- falha se a limpeza não apagar as tabelas novas; reusar `testdb`, `NovaEmpresa` e os helpers `chamar` e `reqComCtx` de `dre_testhelpers_test.go`.

## Tasks & Acceptance

**Execution:**
- [ ] `backend/migrations/008_filiais_gerenciais.sql` -- tabelas e nova `limpar_dados_teste`
- [ ] `backend/dre/filiais.go` -- `Listar`, `Salvar`, `Excluir`, `FilialGerencialDe`, erros tipados (conflito de filial com o código dono, em uso, entrada inválida)
- [ ] `backend/handlers/filiais_gerenciais.go` e `backend/main.go` -- rota e mapeamento de erros (400, 403, 409, 500 com log)
- [ ] `backend/dre/filiais_test.go` e `backend/handlers/filiais_gerenciais_test.go` -- cobrir toda a matriz, inclusive a trilha, o isolamento por empresa e a limpeza de teste
- [ ] `frontend/src/lib/navigation.ts`, `App.tsx`, `pages/aud/AdmFiliais.tsx` -- aba "Filiais" e tela

**Acceptance Criteria:**
- Given um grupo com duas filiais do ERP, when o resolvedor recebe uma delas, then devolve o código do grupo; e uma filial sem ligação devolve a si mesma.
- Given uma filial do ERP já ligada, when se tenta ligá-la a outro grupo, then `409` com o código do dono e nada muda.
- Given `go build ./...`, `go vet ./...`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`), when rodam, then passam.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- expected: passam
- `cd frontend && npm run typecheck` -- expected: só o erro anterior em `ResetPassword.tsx(13,9)`
- `git grep -n -E "[0-9]{2,3}.*(10,11|11,13)" -- backend frontend` -- expected: nenhuma composição real de filiais

## Implementation Notes

Tamanho acima de 1600 tokens: cruza banco, backend e frontend; sem divisão proposta. A migration é irreversível em produção. A chave do snapshot (1.8) segue sendo a filial do ERP até uma história própria usar `FilialGerencialDe`.
