# Revisão independente da spine — FB_AUDITORIA A1 (checklist de boa spine)

Spine: ARCHITECTURE-SPINE.md. Conferida contra o PRD (FR-1 a FR-20), `backend/` (migrations 001-003, handlers/dre.go, dre_import.go, auditoria_auth.go, audit_log.go, main.go, docker-compose.yml, Dockerfiles, go.mod) e `frontend/src/lib/navigation.ts`.

## Veredito
Spine bem estruturada e quase toda verificável, com stack correta. Porém ratifica como "existente" coisas que não existem, contradiz uma restrição do CLAUDE.md (três áreas), deixa o mapeamento dos perfis do PRD para os perfis do código em silêncio e tem um AD (AD-13) cujo esquema não suporta o requisito ao qual está ligado. Precisa de uma rodada de correção antes de virar substrato de build.

## Achados

### A1 (alta) — AD-12: renomeação de áreas contradiz CLAUDE.md e navigation.ts, sem motivo no PRD
- CLAUDE.md (Constraints) fixa "três áreas — Estratégico, Operacional, Administrativo"; `navigation.ts`, `AreaEstrategico/Operacional/Administrativo` em auditoria_auth.go e `users.areas DEFAULT ARRAY['estrategico','operacional','administrativo']` (001) confirmam. O AD-12 troca os ids (`estrategico`→`administrativo`, `administrativo`→`configuracoes`, `operacional`→`operacao`) e reserva `comercial`.
- Nenhum FR do PRD pede isso (FR-18/19 tratam de perfil e cadastro). É decisão de produto/UX infiltrada na arquitetura, e não está marcada [ADOPTED] (corretamente), mas também não está marcada como mudança de restrição nem como pergunta aberta.
- Reutilizar o id `administrativo` com significado novo é o pior caso de migração: o `UPDATE ... CASE` é seguro, mas qualquer cache, token ou rota antiga que ainda carregue `administrativo` passa a conceder a área errada. A Rule não cobre a janela de deploy (front e back "no mesmo release" não é atômico: cache de SPA, sessões abertas).
- Ação: ou remover o AD (não está amarrado a FR), ou registrá-lo como mudança deliberada do CLAUDE.md com id novo (sem reaproveitar `administrativo`) e plano de transição.

### A2 (alta) — AD-11: perfis do PRD não são mapeados aos perfis do código
- PRD: analista, Controller, Gerente, diretoria. Código (CHECK em 001): `auditoria | controladoria | diretoria | gestor`; main.go usa `"controladoria"` para escrita e `"controladoria","auditoria"` para trilha.
- A spine só diz que `analista` entra no CHECK. Não diz que Controller = `controladoria`, Gerente = `gestor`, nem o que `auditoria` (perfil que existe no código e não existe no PRD; o PRD lista auditoria externa como non-user) pode fazer. Hoje `controladoria` faz tudo (carga, mapa, usuários); com `analista` e Controller separados, a divisão (quem carrega, quem fecha, quem gere usuários) tem que ser decidida, e a spine não a decide.
- `perfis.go` e as constantes `PodeFechar/PodeAjustar/PodeEditarMapa` não existem; a Rule não traz a matriz capacidade x perfil, então não é verificável nem impede duas unidades de divergirem (cada story escolheria seu conjunto). Não há dono para "quem lança ajuste" (pergunta 6 do PRD, adiada) — adiá-la é ok só se a matriz for configurável; a spine não diz isso.
- Ação: incluir a tabela capacidade x perfil (mesmo que provisória) e o mapeamento nomes PRD <-> CHECK; decidir o destino de `auditoria` e `gestor`.

### A3 (alta) — AD-13: o esquema de e-mail não cobre FR-4
- AD-13 amarra FR-4 (aviso da Lista de aviso a cada Ajuste), mas `envios` tem `UNIQUE (fechamento_id, user_id)` e é preenchida "na transação do Fechamento". Aviso de ajuste não tem fechamento nem user (a Lista de aviso é grupo de e-mail, não usuário). O fluxo de FR-4 fica sem decisão: duas unidades vão modelar de formas diferentes (a tabela de outbox é o ponto real de divergência).
- Também não define destino do "link" (APP_URL existe no compose; autenticação do link para o Gerente), nem retentativa/limite de tentativas ("falhas legíveis" não diz como).
- Ação: outbox genérica (tipo, destinatário e-mail, referência) ou AD separado para o aviso de Ajuste.

### A4 (média-alta) — Ratificação falsa do que "existe hoje"
Conferido contra o código:
- `resolverConta` (AD-5, "como o realizado") não existe; o código tem `carregarMapa`, `somaPorLinha`, `aplicarSubtotais` em handlers/dre.go.
- AD-1 está [ADOPTED], mas hoje handlers/dre.go e dre_import.go contêm SQL, regra de mapa, cálculo de fechamento e INSERTs (`lancamentos`, `cargas`, `dre_orcado`, `dre_fechamentos`). Não existe pacote `backend/dre/`. O AD-1 é alvo, não ratificação, e a spine não traz plano de transição (ordem de extração, quem migra os handlers existentes, o que a regra "teste falha o build se outro pacote tiver INSERT" faz enquanto o legado existe). Sem isso, o teste do AD-2 quebra o build desde o primeiro dia ou é adiado informalmente.
- `ALLOW_SELF_REGISTER` (convenções) não existe no código; `/api/auth/register` está ligado sem flag em main.go. Plausível como mudança de FR-19, mas é apresentado como convenção vigente.
- `auditoria.config_empresa` não existe (nenhuma migration).
- AD-3 diz que `lancamentos` "têm coluna `fonte`"; a tabela já tem `origem` (csv|winthor, 003) e `carga_id`. Criar `fonte` duplica `origem`: o ponto de divergência real é qual das duas é a chave de escopo. Decidir: renomear/reusar `origem` ou migrar.
- Tabelas do ER têm nomes diferentes das reais (`FECHAMENTO` x `dre_fechamentos`, `CARGA` x `cargas`); convenção de nomes não diz se o prefixo `dre_` permanece.
- Itens que batem: stack e versões (go 1.26.1, lib/pq 1.11.2, go-ora v2.9.0 `// indirect`, jwt v5.3.1, x/crypto 0.49.0, postgres:15-alpine, golang:1.26.10-alpine, node:18-alpine, nginx:stable-alpine, React/Vite/Tailwind/Recharts/tanstack conforme package.json); `writeAuditLogTx`; trigger de imutabilidade do audit_log; slow log (`AUDITORIA_SLOW_REQ_MS`); `GetEmailConfig` em services/email.go; compose com cloudflared, `expose` sem `ports`, volume `auditoria_pg_data`; JC_EMPRESA_ID.

### A5 (média) — Contradição interna: configuração "nunca em env" x implementação
- Convenções: "configuração por empresa em config_empresa, nunca em env nem no código". Mas AD-13 usa `services.GetEmailConfig` (lê SMTP de env) e AD-15 usa `JC_EMPRESA_ID` e `JC_ORACLE_*` em env. Falta a fronteira: segredos e infraestrutura em env; regra de negócio por empresa em config_empresa. Sem isso, duas unidades escolhem lugares diferentes para o limite (FR-13), Lista de aviso, views Oracle (`JC_ORACLE_DRE_VIEWS` já é env).

### A6 (média) — Deferred que pode fazer unidades divergirem
- Sensibilidade da folha (pergunta 7): afeta AD-10 (o que o filtro devolve para o Gerente); a spine adia, mas o filtro `FiltroEscopo` é um ponto único, então o adiamento só é seguro se o marcador existir desde a migração. O texto diz "o gancho é um marcador", sem dizer que a coluna entra já na `conta_departamento` — se não entrar, a mudança depois exigirá reescrever consultas. Fixar a coluna agora.
- Fonte real do Realizado por conta: o prefixo `ERP.` (AD-4) assume que as colunas da view viram contas; se a fonte por conta vier numa view com contas numéricas (glossário: "código numérico"), as duas famílias de contas coexistem no Mapa. A regra de coexistência (prefixos x contas numéricas, sinal) não é decidida.
- FR-13 ("alterar o limite não reclassifica retroativamente competências fechadas"): exige que o limite vigente seja gravado no fechamento (ou versionado). Nem o esquema de `dre_fechamento_linhas` nem `config_empresa` (sem histórico) o preveem. Decisão ausente, não adiada: dois implementadores vão divergir.
- "Pendente" derivado (AD-7) depende de saber qual desvio exige justificativa; onde e quando o desvio é calculado e qual limite usa (em fechada x aberta) não está decidido.

### A7 (média) — Fechamento: AD-9 diverge do PRD e deixa lacunas
- FR-10: "reabrir cria nova versão". AD-9 marca `reaberto_em` no fechamento existente e mantém `MAX+1` (a nova versão só nasce no próximo fechamento). Comportamento diferente do que o PRD descreve; a spine não declara o desvio. Escolher e dizer.
- `dre_fechamento_linhas` tem `ON DELETE CASCADE` (003) e é "imutável por trigger": o trigger deve cobrir DELETE vindo de cascata de `empresa` e de `dre_fechamentos`; isso não foi dito.
- O handler atual calcula o fechamento com `somaPorLinha(db, ...)` fora da transação e só então abre `tx` e calcula `MAX(versao)` sem lock (visto em dre.go ~660-690). A AD-8 cobre o lock, mas o Fechar precisa mover o cálculo para dentro do lock; a Rule de AD-8 só diz que o lock é "o mesmo de Fechar e Reabrir", sem exigir a leitura dentro da transação. Risco real de snapshot que não bate com o que a guarda protegeu.
- Chave dos locks: `pg_advisory_xact_lock` por (empresa, competência) e `pg_try_advisory_lock` global (AD-15) sem convenção de espaço de chaves; colisão entre os dois é possível. Falta o namespace.

### A8 (média) — Escopo do Gerente e agregados
- AD-10 filtra "todo SQL que devolve valor por conta, ajuste ou justificativa". Subtotais da árvore do DRE e o percentual sobre Faturamento Líquido (FR-7) usam linhas de outros departamentos; o Gerente, ao ver subtotais/ponte/projeção (FR-17), pode inferir valores fora do escopo. A Rule não define o que o Gerente vê em linhas agregadas. Decidir: Gerente só vê linhas do seu departamento, sem subtotais globais, ou valores de agregados são permitidos.
- A guarda do e-mail (corpo só com o escopo do destinatário) está coberta por AD-13/AD-10; ok.

### A9 (média) — Dois caminhos de Orçado coexistindo
- Hoje existem `dre_orcado` (por linha_codigo) e o endpoint `/api/aud/dre/orcado` que o grava. AD-5 cria `orcado_conta` e declara o legado "somente leitura", mas não diz de qual tabela o DRE lê durante a transição, nem se o endpoint antigo continua escrevendo. É exatamente a divergência "orçado por um caminho, realizado por outro" que AD-5 diz impedir. Também FR-2 (nulos = sem orçado) vs `valor NUMERIC NOT NULL` no legado.

### A10 (alta, dimensão operacional) — Envelope operacional em silêncio ou só no Deferred
Existe um bloco "Implantação" no Structural Seed (bom), mas faltam decisões ou perguntas abertas para:
- **Ambientes**: só produção. Não há dev/staging para validar migrations destrutivas de dados (AD-12 faz UPDATE em `users.areas`), nem estratégia de rollback de migration. As migrations rodam no startup da API (main.go ~163-200, tabela `schema_migrations`); múltiplas réplicas ou falha parcial não foram discutidas (a spine adia réplicas, mas não o rollback).
- **Backup/restauração** adiado. Para um produto cuja promessa é trilha imutável e fechamento congelado, perder o volume `auditoria_pg_data` anula o PRD (SM-2, SM-4). Deveria ser decisão (RPO/RTO mínimos) ou pergunta aberta, não "Deferred" genérico. Não há política de retenção do `audit_log`.
- **Rede com o Oracle do cliente**: o compose traz o host real como default (`JC_ORACLE_HOST:-201.48.119.197`) em repositório que o PRD declara público. Isso contradiz a regra "repo público nunca expõe dados do cliente" e deve virar achado de segurança; a spine não cita isso nem o allowlist de IP de saída da VPS, nem o que acontece quando o Oracle está fora (a ingestão falha e o fechamento depende dela?).
- **Observabilidade**: só slow log; sem alerta para falha de ingestão agendada ou de envio (a spine diz "falhas legíveis ao Controller" mas não fixa o canal) e sem health da fila de e-mail.
- **Fuso horário/calendário** do agendador (AD-15) e da competência: não decidido.
- **Imagens sem tag fixa** (`cloudflared:latest`, `alpine:latest`) listadas como dívida, ok, mas `node:18` fora de suporte no build de produção é decisão, não só dívida.
- **Sessão/autenticação** (JWT em cookie, refresh) não aparece no Paradigma; é código existente que a spine não ratifica nem usa como referência, embora FR-18 dependa dele (perfil relido do banco a cada request em `AuditoriaAuthMiddleware` — boa propriedade que merece AD [ADOPTED]).

### A11 (baixa) — Cobertura e consistência menores
- Cobertura de FR: todos aparecem no `binds`, mas a Capability Map não menciona FR-13 além de "convenções", nem FR-5 (Trilha da estrutura), nem FR-16/FR-17 (dashboards: onde fica o cálculo de ponte e projeção, `dre.Realizado`? serviço próprio?). FR-17 é "depois do núcleo" e a spine não decide onde mora.
- `dre/envio` e `dre/rateio` na Capability Map x árvore de arquivos com `dre/` e `Envio`, `Rateio` como itens (pacote único ou subpacotes?). Também o diagrama diz `D --> E[services/email.go]` mas `dre` importar `services` não é listado como seta permitida de forma consistente com "setas são as únicas dependências".
- AD-2: o teste "falha o build se outro pacote contiver esses comandos" é verificável, mas não diz como (grep? análise estática?), nem cobre SQL montado por concatenação ou funções SQL. Aceitável como Rule, mas precisa do critério.
- AD-6: a tabela `ajustes` não tem a chave dimensional declarada (filial? centro de custo?). O PRD define Ajuste por conta e competência; sem filial, o fechamento por filial (AD-8) não sabe a que filial o ajuste pertence.
- AD-3: reimportação por `(empresa, fonte, competência, filial)` ignora `centro_custo`/`canal` existentes em `lancamentos`; dizer se somem ou continuam dimensões.
- Numeração e nomes: AD-1 e AD-12 são os únicos [ADOPTED]; outros ADs com partes já existentes (AD-9 base de `dre_fechamentos`, AD-11 parte de `AudContext.Tem`) não distinguem o que é ratificação do que é novo.

## Checklist resumido
| Critério | Resultado |
| --- | --- |
| Fixa os pontos de divergência sem perder nenhum | Parcial: faltam matriz de perfis (A2), outbox de ajuste (A3), limite no fechamento (A6), Orçado transitório (A9) |
| Rule verificável e que impede o Prevents | Maioria sim (AD-2, 3, 6, 7, 8); AD-11 e AD-13 não; AD-1 não é verdadeiro hoje (A4) |
| Deferred não faz unidades divergirem | Parcial (A6, A10 backup) |
| Tecnologias verificadas | Sim, batem com go.mod, package.json, Dockerfiles, compose |
| Ratifica o código existente | Parcial: ADOPTED falso em AD-1; `resolverConta`, `config_empresa`, `perfis.go`, `ALLOW_SELF_REGISTER`, `fonte` vs `origem`; AD-12 contradiz CLAUDE.md |
| Cobre FR-1 a FR-20 | Sim no `binds`; lacunas reais em FR-4 (aviso), FR-13, FR-17 |
| Todas as dimensões decididas/adiadas/abertas | Não: ambientes, rollback de migration, backup/retenção, rede com Oracle e segredo no repo, alertas, fuso |
