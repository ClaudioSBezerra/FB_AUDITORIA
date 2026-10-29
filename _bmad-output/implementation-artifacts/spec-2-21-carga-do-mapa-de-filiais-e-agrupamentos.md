---
title: 'Story 2.21: Carga do Mapa de filiais e dos agrupamentos por arquivo'
type: 'feature'
created: '2026-10-29'
status: 'done'
baseline_commit: '61da1ab'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/epics.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O Mapa de filiais (filial gerencial e as filiais do ERP que a compõem) e os agrupamentos só se cadastram um a um, pela tela. A estrutura de filiais da empresa tem várias filiais gerenciais e agrupamentos, e entrar tudo à mão é lento e sujeito a erro.

**Approach:** Duas cargas por arquivo (CSV ou XLSX), no padrão da carga de departamentos (2.20): aditivas, tudo ou nada, com trilha. Uma para o mapa, outra para os agrupamentos.

## Boundaries & Constraints

**Always:**
- `POST /api/aud/filiais-gerenciais/carga` e `POST /api/aud/agrupamentos-filiais/carga` (área configuracoes, capacidade `editar_estrutura`, a mesma do cadastro manual), multipart `arquivo`, `limiteCargaBytes()` e `erroUploadCarga`. Leitura por `leTabelaCarga`, no padrão de `dre/fontes/csv/departamentos.go`; o cabeçalho pode vir depois de linhas de título.
- Mapa: colunas `filial_gerencial` e `filial_erp` (aliases sem acento, maiúsculas e espaço no lugar de `_`), `nome` opcional (padrão "Filial <código>"). Agrupamentos: colunas `agrupamento` e `filial` (alias `codigo`), `nome` opcional (padrão: o código).
- Código de filial (gerencial, do ERP e membro) perde zeros à esquerda se for só dígitos ("01" = "1"), como na ingestão; `11.0` do XLSX vira `11`. Código de agrupamento é gravado como veio.
- Aditivas: criam o que falta e atualizam o nome se o arquivo trouxer um nome diferente. Nunca apagam filial gerencial, ligação, agrupamento ou membro que o arquivo não cite.
- Mapa: uma filial do ERP que já pertence a outra filial gerencial é movida para a do arquivo, e a resposta lista cada movimentação (`movidas`: filial, de, para).
- Agrupamentos: os membros são filiais gerenciais que já existem; se alguma não existe, uma só mensagem lista as que faltam (até 20 e "e mais K") e pede o Mapa de filiais antes. Uma filial que vale só por si precisa de uma linha no mapa ligada a ela mesma (`12;12`), senão não entra num agrupamento (e o DRE por agrupamento soma só as filiais do ERP ligadas).
- Erros de linha (até 20 e o total, como as demais cargas): vazio, filial do ERP em duas filiais gerenciais diferentes no arquivo, nomes diferentes para o mesmo código, palavra reservada GRUPO, tamanhos acima dos do cadastro manual, código de filial gerencial igual a filial do ERP de outra, código de agrupamento igual ao de filial gerencial (e o inverso). Par repetido igual é ignorado.
- Tudo ou nada numa transação com advisory lock por empresa (`filiais:<empresa>`, o mesmo para as duas cargas); trilha `filial_gerencial`/`carga` e `agrupamento_filiais`/`carga` na mesma transação, com contagens e códigos.
- Respostas: mapa — `linhas`, `criadas`, `atualizadas`, `ligacoes_novas`, `ligacoes_movidas`, `inalteradas`, `movidas`; agrupamentos — `linhas`, `criados`, `atualizados`, `membros_adicionados`, `inalteradas`.
- Telas: bloco de carga (`BlocoCargaArquivo.tsx`, com resultado, erros e "Baixar modelo CSV" com linhas fictícias) em `AdmFiliais.tsx` e `AdmAgrupamentos.tsx`, visível a quem edita.
- Toda consulta filtra por `empresa_id`.

**Never:** Não apagar nada fora do arquivo. Nenhum dado real em teste, spec ou modelo. Os arquivos de carga com os dados do cliente ficam fora do repositório.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior |
|----------|--------------|---------------------------|
| Criação do mapa | filiais gerenciais novas | `criadas`, `ligacoes_novas` |
| Reenvio | mesmo arquivo | tudo `inalteradas` |
| Filial já em outra gerencial | linha leva a filial a outra | movida, listada em `movidas` |
| Ausentes | ligações e filiais fora do arquivo | continuam |
| Duas gerenciais para a mesma filial | linhas 3 e 5 | 400, "a filial X já está na filial gerencial Y (linha N)" |
| Código igual a filial de outra gerencial | `61;62` e `60;61` | 400 |
| Agrupamento com filial inexistente | 99, 98 | 400, "filiais gerenciais inexistentes: 99, 98 (2 linha(s))" |
| Agrupamento reenviado com membro novo | acrescenta | `membros_adicionados`; os antigos ficam |
| Sem `editar_estrutura` | diretoria, auditoria, gestor | 403, nada gravado |
| Outra empresa | outra empresa | não vê nem é alterada |

</frozen-after-approval>

## Code Map

- `backend/dre/filiais_carga.go` -- `CarregarMapaFiliais`, `CarregarAgrupamentos` e os tipos.
- `backend/dre/fontes/csv/filiais.go` -- `LerMapaFiliais`, `LerAgrupamentos` (sem SQL).
- `backend/handlers/filiais_gerenciais.go`, `backend/handlers/agrupamentos_filiais.go`, `backend/main.go`, `backend/rotas_capacidades_test.go` -- handlers, rotas e tabelas de teste (a varredura de escopo não as alcança: as rotas não têm `ler_no_escopo`).
- `backend/handlers/filiais_carga_test.go`, `backend/dre/fontes/csv/filiais_test.go` -- testes com banco e de leitura.
- `frontend/src/pages/aud/BlocoCargaArquivo.tsx`, `AdmFiliais.tsx`, `AdmAgrupamentos.tsx` -- o bloco e as telas.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck` -- só o erro antigo de `ResetPassword.tsx`

## Implementation Notes

Decisões de detalhe:
- Um arquivo de agrupamentos carregado antes do mapa é recusado com a mensagem das filiais que faltam. A ordem é: mapa, depois agrupamentos.
- Para as filiais que valem só por si (por exemplo, balcões), o arquivo do mapa leva uma linha ligada a ela mesma. Sem isso a filial gerencial não existe e o agrupamento não a aceita.
- Filial gerencial já gravada é achada pelo código normalizado ("01" casa com "1"); a gravada mantém o código que já tinha.
- Ao mover uma filial do ERP, a filial gerencial de origem pode ficar sem filiais; isso é permitido pelo cadastro manual e não bloqueia a carga.
- As duas cargas usam o mesmo advisory lock, então rodam em série por empresa.
- O componente `BlocoCargaArquivo` é novo e genérico; o bloco de departamentos (2.20) segue com o seu próprio código, sem refatoração.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Modo "substituir" (apagar o que não vem no arquivo) | low | defer | `deferred-work.md` |
