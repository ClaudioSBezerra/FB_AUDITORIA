---
title: 'Story 2.19: Salvaguarda de período na carga de lançamentos'
type: 'feature'
created: '2026-10-29'
status: 'done'
baseline_commit: '2583fc7'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/epics.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Uma Entrada_Dados com as células `Mês` e `Loja` desatualizadas foi gravada no período e na filial errados, sem ninguém notar: o importador confiou nas células e a tela só dizia "N lançamentos importados".

**Approach:** Mostrar em destaque o que foi lido (competência, filial e contagem de cada par) e fazer a carga em duas etapas: simular (lê, valida, aplica a guarda de período, não grava) e confirmar. Avisos de plausibilidade, sem bloquear. A regra de leitura do arquivo não muda.

## Boundaries & Constraints

**Always:**
- `POST /api/aud/dre/lancamentos/importar?simular=1` (mesma rota e mesma capacidade `carregar`) lê e valida o arquivo, toma a guarda de período (mês fechado: 409 já na simulação) e devolve o mesmo corpo da gravação, com `simulacao: true` e sem `carga_id`. Não grava `cargas`, `lancamentos` nem `audit_log`; a transação só toma locks de período e termina em rollback.
- Resposta (simulação e gravação): `simulacao`, `linhas` (total), `origem`, `arquivo`, `pares` [{competencia AAAA-MM, filial, lancamentos}] ordenados, `avisos` [{codigo, mensagem}], `ajustes_ignorados`, `linhas_ignoradas`, `conferencia_total`. Gravação acrescenta `carga_id`.
- Avisos (nunca bloqueiam): `competencia_antiga` (mais de 12 meses antes do mês corrente), `competencia_futura` (depois do mês corrente), `nome_do_arquivo` (o nome traz AAAAMM, MM-AAAA, MM_AAAA ou MMAAAA, com espaços em volta do separador, e nenhum dos meses sugeridos é competência do arquivo), `filial_nova` (nem no Mapa de filiais, nem em lançamentos da empresa), `outra_origem` (o par já tem lançamentos de outra origem).
- Na gravação o resumo e os avisos são calculados na mesma transação, antes de substituir os escopos.
- A tela (`AdmCarga.tsx`) só oferece "Confirmar carga" depois de "Conferir arquivo" do mesmo arquivo; trocar o arquivo zera a conferência. O cartão da conferência e o da gravação ficam acima do resto.
- Toda consulta filtra por `empresa_id`.

**Never:** Não mudar a leitura das células `Mês`/`Loja`. Não bloquear por aviso. Nenhum dado real em teste, spec ou commit.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior |
|----------|--------------|---------------------------|
| Simulação | `?simular=1` | resumo e avisos, nada gravado |
| Confirmar | sem `simular` | grava e devolve o mesmo resumo com `carga_id` |
| Mês fechado | competência com fechamento | 409 na simulação e na gravação |
| Duas competências | arquivo com 2 meses | um par por (mês, filial) |
| Nome divergente | nome com 09-2026, dados 202602 | aviso `nome_do_arquivo` |
| Filial inédita | fora do Mapa e dos lançamentos | aviso `filial_nova` |
| Outra origem | par com lançamentos de outra origem | aviso `outra_origem` |
| Outra empresa | dados de outra empresa | não entram nos avisos |
| Sem `carregar` | diretoria, auditoria, gestor | 403 (portão da rota) |

</frozen-after-approval>

## Code Map

- `backend/dre/ingest.go` -- `Importar` separado em `ler`, `guardarPeriodos` e a gravação; `Simular`; `Ingestor.Agora` fixa o "hoje" nos testes.
- `backend/dre/ingest_resumo.go` -- `Resumo`, `ParResumo`, `Aviso`, o mês do nome do arquivo e `montarResumo`.
- `backend/handlers/dre_import.go` -- `?simular=1` e o corpo da resposta.
- `backend/handlers/dre_import_simular_test.go`, `backend/dre/ingest_resumo_test.go` -- testes com banco, dados fictícios.
- `frontend/src/pages/aud/AdmCarga.tsx` -- "Conferir arquivo", "Confirmar carga" e o cartão em destaque.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo de `ResetPassword.tsx`

## Implementation Notes

Decisões de detalhe:
- A simulação não vai à trilha: não é escrita, e o padrão do código só registra leituras que mostram valor (a resposta não traz valor).
- A tela garante as duas etapas; no backend a gravação sem `simular` continua possível, por compatibilidade com o CSV do sistema, o "Atualizar dados" (que passa pelo mesmo `Ingestor`) e integrações. Quem chamar a API direto não é obrigado a simular.
- "Anteriores" na filial nova inclui os lançamentos da própria empresa de qualquer origem e competência, e o Mapa de filiais por código do ERP (`filiais_gerenciais_erp`). Empresa sem Mapa e sem lançamentos avisa em toda filial da primeira carga.
- Janela de plausibilidade: do mês corrente menos 12 meses até o mês corrente, inclusive; vale o relógio do servidor.
- O mês do nome é tirado do nome sem pasta e sem extensão; só vale ano 20AA e mês 01 a 12 sem dígito colado. Se o nome sugere vários meses, basta um deles ser competência do arquivo. Formatos AAAA-MM e AAAA_MM não são lidos (não foram pedidos).
- Em arquivo com dois meses, o aviso de nome não dispara se o mês do nome é um dos dois.
- Pontos frágeis: o aviso de nome pode errar com nomes que carregam data completa (por exemplo, 05-12-2026 sugere 12/2026); o `outra_origem` não diz se as origens têm valores diferentes, só que coexistem (a conferência entre origens é a 2.12); a simulação lê o arquivo inteiro duas vezes (uma por etapa), custo aceito para arquivos grandes; a guarda na simulação toma o lock de período por instantes.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Obrigar a simulação também no backend | low | defer | `deferred-work.md` |
