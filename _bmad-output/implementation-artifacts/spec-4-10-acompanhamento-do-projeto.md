---
title: 'Story 4.10: Acompanhamento do projeto'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '9ff2146'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/spec-4-4-ids-de-area-novos-e-menu.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** A Controller e os gestores do cliente não têm conta na ferramenta onde o dono do produto acompanha os cards, e precisam ver o andamento das histórias.

**Approach:** Área nova `acompanhamento` e a página `/acompanhamento`: Kanban somente leitura com as histórias do projeto. Os dados saem de `epics.md` e `sprint-status.yaml` por um gerador, ficam em `backend/acompanhamento/dados.json` (versionado) e são embutidos no binário com `go:embed`, porque o container de produção não tem `_bmad-output`.

## Boundaries & Constraints

**Always:** Gerador em Go (`backend/cmd/gerar-acompanhamento`, lógica em `backend/acompanhamento/gerador.go`), sem dependência nova. Por história: `id`, `titulo`, `epico {numero,nome}`, `release` (A1), `status` (Backlog | Pronto | Em andamento | Em revisão | Feito, vindo de backlog | ready-for-dev | in-progress | review | done), `bloqueada_cliente` + `motivo_bloqueio`, `resumo` e `criterios` em texto simples; metadados `gerado_em` e `versao` (git short SHA). Histórias descartadas (marcadas "**descartada**" no `epics.md`: 5.5 e 5.7) não entram. Só `epics.md` e `sprint-status.yaml` são lidos; as "Notas" das histórias nunca vão ao JSON. Rota `GET /api/aud/acompanhamento` com `withAud` e a área `acompanhamento`; só GET (`405` nos demais); `Cache-Control: private, max-age=60`. `AreaAcompanhamento` entra em `areasValidas` (PUT de usuários) e na lista do admin de plataforma em `/api/aud/me`. Teste que lê as rotas `/api/aud/*` de `main.go` e prova `403` para quem só tem a área nesta lista inteira, e `403` em `/api/aud/acompanhamento` para quem não a tem. Frontend: item no rail, página com cinco colunas e contadores, filtro por épico, busca, selo "Aguardando o cliente", painel com resumo e critérios, "Atualizado em" e versão, progresso geral e por épico (feitas ou em revisão sobre o total); colunas rolam na horizontal no celular; nenhum botão de escrita. Quem só tem a área cai em `/acompanhamento` na landing; o seletor de áreas de Usuários lista "Acompanhamento".

**Never:** Não adicionar `acompanhamento` ao DEFAULT de `users.areas` nem conceder a usuários existentes (sem migration). Não ler nem copiar `respostas-cliente-*.md`, `deferred-work.md` nem dado do cliente. Não mexer no quadro do GitHub.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Só `acompanhamento` | GET `/api/aud/acompanhamento` | `200`, JSON, cache de 60 s | N/A |
| Só `acompanhamento` | qualquer outra rota `/api/aud/*` com área | `403` | N/A |
| Sem a área | GET `/api/aud/acompanhamento` | `403` | N/A |
| POST/PUT/PATCH/DELETE | rota do quadro | `405` | N/A |
| `epics.md` mudou, JSON não | `go test ./acompanhamento` | falha dizendo como regenerar | N/A |
| `_bmad-output` ausente | teste de atualização | pulado | N/A |
| História no yaml e fora do `epics.md` | gerador | erro | recusa gerar |
| História no `epics.md` e fora do yaml | gerador | coluna Backlog | N/A |

</frozen-after-approval>

## Code Map

- `backend/acompanhamento/` -- `acompanhamento.go` (tipos, `go:embed`), `gerador.go` (leitura e geração), `dados.json`, `gerador_test.go`.
- `backend/cmd/gerar-acompanhamento/main.go` -- `go run ./cmd/gerar-acompanhamento` a partir de `backend/`.
- `backend/handlers/acompanhamento.go`, `auditoria_auth.go` (`AreaAcompanhamento`, lista do admin), `aud_usuarios.go` (`areasValidas`), `backend/main.go` (rota).
- Testes: `backend/acompanhamento_rotas_test.go` (rotas reais de `main.go` pelo `AuditoriaAuthMiddleware`), `handlers/acompanhamento_test.go`, `handlers/acompanhamento_ambiente_test.go`, `handlers/areas_test.go` (PUT com o id novo).
- `frontend/src/pages/aud/Acompanhamento.tsx`, `App.tsx` (rota e landing), `lib/navigation.ts`, `components/AppRail.tsx`, `pages/aud/AdmUsuarios.tsx`.

## Tasks & Acceptance

- [x] Gerador, `dados.json`, rota, área, testes, página, rail, landing, seletor de áreas.

**Acceptance Criteria:**
- Given um usuário só com a área `acompanhamento`, when entra, then cai no Kanban de cinco colunas e não consegue abrir nenhuma outra rota `/api/aud/*`.
- Given uma mudança em `epics.md` sem regenerar o JSON, when `go test ./...` roda, then falha com o comando para regenerar.
- Given `go build`, `go vet`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`) e `npm run build`, when rodam, then passam.

## Verification

- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo; build passa
- `cd backend && go run ./cmd/gerar-acompanhamento` -- "já estava em dia"

## Implementation Notes

- Decisões de detalhe: resumo e critérios saem em português (Como/quero/para que; Dado/Quando/Então/E); o sufixo de referências `(FR-x, AD-y)` é tirado do título; sem entrada no yaml a história vai ao Backlog.
- `bloqueada_cliente` vem de três fontes do `epics.md`: nota ou revisão da própria história com frase de dependência ("depende da resposta", "ainda depende d...", "só é executável quando...", "Até lá, não executar"); nota de épico que cita a história ("a história 8.2 ... depende de uma resposta do cliente"); nota de épico que diz que o épico só começa depois da resposta (épico 7 inteiro). O motivo é a frase da nota, cortada em 220 caracteres; frase com cara de dado real (CNPJ, número longo, R$ ou citação de `respostas-cliente`) vira "Depende de resposta do cliente.".
- Metadados: `gerado_em` é a data da geração e `versao` o `HEAD` no momento. Regenerar sem mudança de conteúdo não altera o arquivo; o teste compara só as histórias. Como o JSON entra no commit seguinte ao `HEAD` lido, a versão mostrada é a do commit anterior à mudança.
- Rotas sem área: `/api/aud/me`, `ambiente/limpar` e `ambiente/virar` não têm área: `me` só devolve o menu e as duas outras exigem admin de plataforma no handler (testado).
- Para liberar o acesso: criar o usuário, abrir Configurações > Usuários e marcar só "Acompanhamento".

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Flag de bloqueio é heurística de texto: nota nova com outra redação não marca | medium | aceito | frases cobertas listadas acima; o dono ajusta a redação ou o regex |
| Épico 7 inteiro marcado "Aguardando o cliente" pela nota do épico, mesmo com revisão parcial de 28/10 | medium | aceito | segue o `epics.md`; remover a frase da nota do épico desmarca |
| `versao` fica um commit atrás | low | aceito | só se vê no cabeçalho; o conteúdo é o que importa |
| Rota nova de `/api/aud/` fora do padrão de `main.go` quebra o teste de rotas | low | aceito | a falha pede para ensinar o regex; é a intenção (fail closed) |
| Sem testes de frontend | low | defer | o repositório não tem harness de frontend |
| `npm run lint` sem configuração do ESLint | low | defer | já registrado em `deferred-work.md` |
