---
title: 'Story 2.20: Carga de departamentos por arquivo'
type: 'feature'
created: '2026-10-29'
status: 'done'
baseline_commit: '5ef2cf3'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/epics.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Os departamentos só se cadastram um a um, pela tela. Para a lista inteira da empresa isso é lento e sujeito a erro; e o arquivo do dicionário, quando cita departamento que não existe, só diz "N linhas com problema".

**Approach:** Carga por arquivo (CSV ou XLSX) no mesmo padrão das cargas do dicionário, da estrutura e do Mapa: upsert por código, tudo ou nada. E a mensagem do dicionário passa a listar os códigos de departamento inexistentes, agregados.

## Boundaries & Constraints

**Always:**
- `POST /api/aud/departamentos/carga` (área configuracoes, capacidade `editar_estrutura`, a mesma do cadastro manual), multipart `arquivo`, `limiteCargaBytes()` e `erroUploadCarga`. Colunas `codigo`, `nome`, `grupo` (aliases `cod`, `descricao`; cabeçalho pode vir depois de linhas de título); lê com `lerTabela`, no padrão de `dre/fontes/csv/estrutura.go`.
- Grupo: Comercial, Logística ou Administrativo, aceito sem acento e em qualquer caixa (`logistica`); grava o nome canônico.
- Upsert por `codigo` da empresa: cria os novos, atualiza nome e grupo dos existentes, conta os inalterados. NÃO apaga quem não veio. Tudo ou nada numa transação com advisory lock por empresa (`departamentos:<empresa>`); trilha `departamento`/`carga` na mesma transação, só contagens e códigos (nenhum nome).
- Erros de linha (até 20 e o total, como as demais cargas): código vazio ou repetido no arquivo, grupo inválido, nome vazio, código acima de 40 e nome acima de 120 caracteres (os do cadastro manual).
- Resposta: `linhas`, `criados`, `atualizados`, `inalterados`.
- Tela: bloco "Carregar departamentos" em `AdmDepartamentos.tsx` (visível a quem edita), com resultado, erros e "Baixar modelo CSV" gerado no navegador com linhas fictícias.
- Dicionário (2.10): departamento inexistente vira uma mensagem só, "departamentos inexistentes: A, B (N linhas)" (singular quando for um só), com até 20 códigos distintos e "e mais K"; nada é gravado.
- Toda consulta filtra por `empresa_id`.

**Never:** Não apagar departamento fora do arquivo. Não gravar nome na trilha. Nenhum dado real em teste, spec ou modelo.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior |
|----------|--------------|---------------------------|
| Criação | códigos novos | `criados` = N |
| Atualização | código existente com nome ou grupo diferente | `atualizados`; igual: `inalterados` |
| Ausentes | existentes que o arquivo não traz | continuam |
| Repetido | mesmo código duas vezes | 400, "codigo repetido X (já na linha N)", nada gravado |
| Grupo inválido, nome vazio, código vazio ou grande | linha inválida | 400 com a linha; nada gravado |
| Sem `editar_estrutura` | diretoria, auditoria, gestor | 403, nada gravado |
| Outra empresa | outra empresa | não vê nem é alterada |
| Dicionário com departamento inexistente | A, B em N linhas | 400 "departamentos inexistentes: A, B (N linhas)" |

</frozen-after-approval>

## Code Map

- `backend/dre/departamentos_carga.go` -- `CarregarDepartamentos`, tipos e a mensagem agregada do dicionário.
- `backend/dre/fontes/csv/departamentos.go` -- `LerDepartamentos` (sem SQL).
- `backend/dre/dicionario_contas.go` -- o dicionário usa a mensagem agregada.
- `backend/handlers/departamentos.go`, `backend/main.go`, `backend/rotas_capacidades_test.go` -- `DepartamentosCargaHandler`, rota e tabelas de teste (a varredura de escopo não a alcança: a rota não tem `ler_no_escopo`).
- `backend/handlers/departamentos_carga_test.go`, `backend/handlers/dicionario_test.go`, `backend/dre/fontes/csv/departamentos_test.go` -- testes com banco e de leitura.
- `frontend/src/pages/aud/AdmDepartamentos.tsx` -- o bloco da carga.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo de `ResetPassword.tsx`

## Implementation Notes

Decisões de detalhe:
- Código é comparado exatamente (maiúsculas e minúsculas contam), como a chave única do cadastro. `10.0` do XLSX vira `10`.
- A mensagem agregada do dicionário leva o número da primeira linha em que um código inexistente aparece; as linhas de conta repetida entram na contagem de linhas.
- A trilha usa `entidade_id = "carga"` (como a do dicionário) e guarda os códigos criados e atualizados, sem nomes.
- A rota não aceita GET: 405 pelo handler (o portão da rota deixa passar quem tem a capacidade).
- Pontos frágeis: a carga nunca renomeia o código (um código diferente é um departamento novo); sem opção de "substituir tudo", de propósito; departamentos de uma carga muito grande são lidos todos em memória (lista pequena por natureza).

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Opção de apagar os ausentes do arquivo | low | defer | `deferred-work.md` |
