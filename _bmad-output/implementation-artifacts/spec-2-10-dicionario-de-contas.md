---
title: 'Story 2.10: Dicionário de contas por arquivo'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '5457cf8'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O sistema não sabe de qual departamento é cada conta. Regra do cliente: o departamento de uma conta é o do **grupo da conta**.

**Approach:** Carga do dicionário (conta, descrição, grupo, departamento opcional) por CSV ou XLSX; o vínculo conta-departamento deriva do grupo (departamento cujo código é o grupo) e pode ser corrigido conta a conta. Resolução de conta com `%` igual à do Mapa. Tela em Configurações. Não altera `dre_mapa`.

## Boundaries & Constraints

**Always:** Migration `010_dicionario_contas.sql`: `conta_dicionario (empresa_id, conta, descricao, grupo, PK (empresa_id, conta))` e `conta_departamento (empresa_id, conta_padrao, departamento_id, origem CHECK IN ('grupo','manual'), PK (empresa_id, conta_padrao), FK composta (departamento_id, empresa_id) para departamentos)`; recriar `limpar_dados_teste` com as duas tabelas (filhas antes da mãe) e as contagens. Colunas aceitas, com cabeçalho normalizado (sem acento, caixa, pontos): `conta`, `grupo` (obrigatórias), `descricao`/`descrição`, `departamento` (código). Leitura CSV/XLSX pela mesma rotina de `dre/fontes/csv` (`lerTabela`, extraída de `Ler`), sem duplicar. Rotas na área `configuracoes`: `POST /api/aud/dicionario/carga` (multipart `arquivo`, 64 MB, só controladoria), `GET /api/aud/dicionario` (qualquer perfil da área; `busca`, `pagina`, `limite`; devolve as contas com departamento e origem, e as sem departamento à parte), `PUT /api/aud/dicionario/conta {conta, departamento_id}` (só controladoria; define origem `manual`; id vazio volta a derivar do grupo). Conta repetida no arquivo ou departamento informado inexistente: erros listados (até 20 e o total, `ErroLinhas`) e nada é gravado. Sem departamento informado: deriva pelo grupo; sem departamento de código igual ao grupo a conta fica pendente, sem bloquear. Vínculo `manual` não é sobrescrito por reenvio que deriva do grupo; só por departamento informado no arquivo. Reenvio atualiza descrição, grupo e vínculo sem tocar em `dre_mapa`; contas fora do arquivo não são apagadas. `dre.DepartamentoDe(ctx, q, empresa, conta)` usa a mesma resolução do Mapa (exato vence, senão o prefixo `%` mais longo), via `dre.MelhorPadrao` compartilhada com `ResolverConta`. Toda escrita na trilha (entidade `dicionario`, ações `carregar` com contagens e `corrigir_departamento`) na mesma transação. Excluir departamento com conta ligada: `409` (fecha o critério da 2.9).

**Never:** Nenhum dado real do cliente (contas, grupos, departamentos, valores) em arquivo versionado; os testes geram CSV/XLSX em código com dados fictícios. Não escrever em `dre_mapa` nem em `lancamentos`. Não editar migration aplicada.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Deriva do grupo | conta com grupo igual ao código de um departamento | vínculo origem `grupo` | N/A |
| Grupo sem departamento | nenhum departamento com esse código | conta sem vínculo, listada em pendentes | não bloqueia |
| Departamento informado | coluna `departamento` com código existente | vínculo origem `manual` | N/A |
| Departamento inexistente / conta repetida | no arquivo | nada gravado, linhas listadas | `400` |
| Reenvio deriva sobre `manual` | vínculo `manual` existente | mantido | N/A |
| Reenvio informa departamento | vínculo `manual` existente | substituído | N/A |
| Reenvio com grupo sem departamento | vínculo `grupo` antigo | removido; `manual` fica | N/A |
| Prefixo `%` | `402%` e `402.001` no dicionário | exato vence, senão o prefixo mais longo | N/A |
| Corrigir uma conta | PUT com departamento da empresa | origem `manual`, trilha | `404` conta/departamento inexistente |
| Excluir departamento ligado | 1 ou mais contas | nada muda | `409` |
| Perfil sem escrita | diretoria faz POST/PUT | nada gravado | `403` |
| Outra empresa | mesmos códigos | isolados | N/A |

</frozen-after-approval>

## Code Map

- `backend/migrations/010_dicionario_contas.sql` -- tabelas e nova `limpar_dados_teste` (cópia da 009 mais as duas).
- `backend/dre/fontes/csv/csv.go` -- `lerTabela` extraída de `Ler` (formato por assinatura, separador, cabeçalho nas 20 primeiras linhas); `csv/dicionario.go` -- `LerDicionario` (só leitura, sem banco).
- `backend/dre/realizado.go` -- `MelhorPadrao` extraída; `ResolverConta` a usa.
- `backend/dre/dicionario_contas.go` -- `Carregar`, `ListarContas`, `CorrigirDepartamento`, `DepartamentoDe`; `dicionario.go` -- 409 de exclusão.
- `backend/handlers/dicionario.go`, `departamentos.go` (`erroDicionario`), `backend/main.go` -- rotas.
- `frontend/src/pages/aud/AdmDicionario.tsx`, `App.tsx`, `lib/navigation.ts` -- aba "Dicionário".
- Testes: `dre/dicionario_contas_test.go`, `dre/fontes/csv/dicionario_test.go`, `handlers/dicionario_test.go`; `TestAmbiente_LimpezaCobreTodaTabelaDeDominio` cobre as tabelas novas.

## Tasks & Acceptance

- [x] Migration, extração da leitura, serviço, handlers, rotas, tela, testes.

**Acceptance Criteria:**
- Given linhas com conta repetida ou departamento inexistente, when carrega, then são listadas e nada é gravado.
- Given reenvio, when carrega, then atualiza descrição, grupo e departamento sem apagar a associação do Mapa nem vínculo manual.
- Given conta com `%`, when resolvida, then vale a resolução do Mapa.
- Given `go build`, `go vet`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`), when rodam, then passam.

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo; `npm run lint` não tem configuração do ESLint (já em `deferred-work.md`)

## Implementation Notes

Decisões: (1) departamento informado no arquivo grava origem `manual` (decisão explícita, não derivada), portanto resiste a reenvios que só derivam; (2) reenvio não apaga contas ausentes do arquivo; (3) a listagem resolve o departamento de cada conta pela regra do `%`, então uma conta coberta por um padrão com departamento não aparece como pendente (campo `padrao` informa qual); (4) `GET` carrega o dicionário inteiro e pagina em memória (volume esperado de milhares de contas); (5) carga e correção usam `pg_advisory_xact_lock` por empresa para serializar escritas concorrentes; (6) `dre.DepartamentoDe` carrega todos os vínculos por chamada (sem versão em lote); o DRE por departamento (épico 3) deve resolver em lote com `MelhorPadrao`.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| `ResolverConta` e o dicionário poderiam divergir | medium | patch | regra única em `MelhorPadrao`; teste compara os dois |
| `GET` pagina em memória e `DepartamentoDe` é N consultas se usado em laço | low | defer | volume pequeno; versão em lote na história do DRE por departamento |
| Reenvio parcial não remove contas antigas | low | rejeitado | segue "atualiza sem apagar"; exclusão de conta fica fora do escopo |
| Alterar o código do departamento depois da carga não muda o vínculo (guarda o id) | low | rejeitado | intencional; a derivação por grupo só roda em nova carga |
| `GET` aberto a qualquer perfil da área | low | rejeitado | segue o padrão de filiais gerenciais |
