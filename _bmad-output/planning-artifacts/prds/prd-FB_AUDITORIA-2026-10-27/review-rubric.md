# PRD Quality Review — FB_AUDITORIA — Release A1: DRE gerencial

## Overall verdict
O PRD tem tese clara (confiança: número rastreável, ajuste justificado, mês fechado não muda), glossário disciplinado, 19 FRs com consequências testáveis, quatro jornadas com protagonista e contra-métricas. Os riscos estão em três pontos: (1) é um PRD brownfield que não diz quais FRs já existem em código e quais são novos; (2) vários FRs dependem de perguntas abertas que bloqueiam a construção (Rateio, Orçado de 3 filiais, quem recebe o aviso de Ajuste) e o escopo não é priorizado; (3) há lacunas de rastreabilidade (FRs sem jornada/métrica, gatilho do envio de e-mail sem FR, carga do dicionário de contas sem FR) e alguns números do cliente em repositório público.

## Decision-readiness — adequate
As decisões centrais estão declaradas como decisões: Realizado por conta entra por CSV até a fonte ser confirmada (FR-3, §6.2), limite de desvio global (FR-13), e-mail só após o Fechamento (FR-14). Os `[NOTE FOR PM]` estão em tensões reais (quem lança Ajuste, folha sensível, Rateio). O que falta é o custo das escolhas: não se diz o que se perde ao adiar o Realizado por conta (SM-1, paridade com a planilha, depende da carga CSV manual, que é exatamente o que o produto quer eliminar).

O escopo do MVP (§6.1) inclui Rateio e dashboards avançados (FR-15, FR-17) enquanto as perguntas que os destravam seguem abertas (OQ-5, OQ-8, OQ-4). Não há priorização nem sequência dentro da release; um decisor não sabe o que cortar se o prazo apertar.

### Findings
- **high** Escopo sem priorização e com FRs bloqueados por perguntas abertas (§4.7, §6.1, §8) — FR-15 (Rateio) depende de OQ-5 (quais departamentos são centralizados, critério fixo ou mensal); FR-13 depende de OQ-8; FR-14 de OQ-4; FR-4 de OQ-6. Tudo está "In Scope" no mesmo nível. *Fix:* marcar FRs bloqueados como "dependem de OQ-x", ordenar em núcleo (FR-1 a 12, 18) e segunda onda (FR-15, 16, 17), ou mover Rateio para fora até OQ-5 fechar.
- **medium** Custo da dependência do CSV não é dito (§4.1, §7 SM-1) — a paridade depende de dados que o CSV traz (verbas, CMV, folha), mas o PRD não diz que sem o CSV a paridade não é atingível. *Fix:* declarar na SM-1 que a medição usa o Realizado por CSV, e o que acontece se a fonte nunca for confirmada.
- **medium** Questões abertas sem dono (§8, OQ-6, 7, 8) — só OQ-1 a 5, 9, 10, 11 têm dono; as que mais travam FRs (6, 7, 8) não têm. *Fix:* atribuir dono e data.

## Substance over theater — strong
Sem persona theater (quatro personas, cada uma ligada a perfil real de FR-18). Sem innovation theater. A Visão é específica (planilha Excel, macro, aba de justificativas). NFRs: quase não há seção de NFR; isso evita boilerplate, mas deixa o vazio oposto (ver Done-ness). A referência "mesma ideia do Farol" (§1) é jargão interno que um leitor externo do repositório público não resolve.

### Findings
- **low** Referência ao Farol sem explicação (§1) — leitor externo não sabe o que é. *Fix:* uma frase explicando ("o sistema vai atrás do gestor: o gestor recebe o que precisa em vez de consultar") ou remover.

## Strategic coherence — adequate
Há tese e as funcionalidades a seguem (ingestão, estrutura, fechamento, justificativa, envio, dashboards). SMs validam a tese de confiança (SM-1, SM-2, SM-4) e há duas contra-métricas bem escolhidas (SM-C1, SM-C2). Fraquezas: o tipo de MVP não é declarado (é "problem-solving: substituir Excel", mas o texto não o diz); SM-2 e SM-3 não têm limiar numérico; várias funcionalidades não são validadas por nenhuma métrica.

### Findings
- **medium** FRs sem métrica (§7) — FR-5, 6, 15, 16, 17, 18, 19 não aparecem em nenhum "Validates". Rateio e dashboards (FR-15 a 17) são grandes e sem critério de sucesso. *Fix:* acrescentar SM para uso do dashboard/Rateio ou admitir que são secundários.
- **medium** SMs sem limiar (§7 SM-1, SM-2) — SM-1 "bate em todas as linhas de subtotal" sem tolerância (centavos? arredondamento?) e sem dizer quantos meses; SM-2 "concluído dentro do sistema" sem prazo-alvo (ex.: dias úteis para fechar). SM-3 tem prazo "a definir". *Fix:* tolerância e número de meses em SM-1; meta de dias em SM-2; ou marcar como `[ASSUMPTION]` já indexada.
- **low** Tipo de MVP não nomeado (§6) — declarar "problem-solving" e a lógica de corte.

## Done-ness clarity — adequate
Todos os FRs têm "Consequences (testable)", o que é bom. Mas várias consequências são vagas ou deixam comportamento indefinido.

### Findings
- **high** Comportamento indefinido para Filiais sem Orçado (§8 OQ-11, FR-2, FR-8) — a view de realizado tem mais filiais que a de orçamento; FR-8 só trata Orçado zero. O que o DRE mostra para essas filiais (variação não calculável? zero?) não está especificado; FR-2 "nulo vira zero" mascararia a falta como Orçado zero. *Fix:* FR com consequência explícita para filial sem Orçado, distinguindo ausente de zero.
- **high** Fechamento "por Filial ou consolidada" sem semântica (FR-9, FR-1, FR-2, FR-11) — as consequências falam em "Competência com Fechamento ativo", mas se uma Filial está fechada e outra não, a carga para a Competência é permitida? Reimportar histórico "preservando Competências com Fechamento ativo" (FR-1) fica ambíguo. *Fix:* definir a granularidade do bloqueio (por Filial+Competência) e o que "consolidada" significa.
- **medium** Ajuste sem Filial (FR-4, Glossário) — o Ajuste é "em uma Conta gerencial de uma Competência"; mas Lançamento tem Filial e o DRE é por Filial. Onde o Ajuste cai? *Fix:* incluir Filial no Ajuste (glossário e FR-4).
- **medium** FR-4 não testa o aviso por e-mail (FR-4) — a descrição promete aviso e exigência de Justificativa, mas nenhuma consequência verifica o e-mail (destinatários, falha, registro). A nota de PM contradiz o texto ("lista de gestores definida" vs. incerteza sobre quem recebe). *Fix:* consequência de e-mail e registro de envio, condicionada a OQ-6.
- **medium** Desvio contra Ano anterior indefinido (Glossário "Desvio", FR-8, FR-13) — o glossário diz "e também contra o Ano anterior", FR-8 define variação só como Realizado menos Orçado, e FR-13 não diz contra qual base o limite se aplica. *Fix:* dizer que a base do limite é o Orçado (ou ambos) e como FR-16 ordena.
- **medium** Termos vagos sem limiar: "usuário autorizado" (FR-7, 8, 16, 17), "ranking dos maiores Desvios" (FR-16, sem critério nem quantidade, valor absoluto ou percentual?), "se está subindo" (FR-17, sem regra de tendência), "falhas de envio visíveis ao Controller" (FR-14, onde?), "critério definido" (Rateio, §3), "volume pequeno (centenas de linhas)" (FR-1, sem limite de desempenho). *Fix:* regra explícita ou `[ASSUMPTION]` indexada.
- **medium** Sem requisitos não funcionais (todo o PRD) — não há bounds de desempenho (tempo de carga/abertura do DRE), retenção de dados, entrega de e-mail, nem tratamento de dado sensível (folha; só uma `[NOTE FOR PM]` solta no fim de §4.9). *Fix:* seção curta de NFRs com valores, ou declarar explicitamente que ficam para arquitetura.
- **medium** Comportamento do link do e-mail (FR-14, UJ-3) — o link exige login? Expira? Passa pela validação de Departamento de FR-18? *Fix:* consequência explícita (link nunca concede acesso sem autenticação).
- **low** FR-3 não definido por referência (FR-3 Out of Scope) — "o formato do CSV é o já aceito pelo importador existente, ampliado conforme necessário" não é verificável sem o formato. *Fix:* citar o contrato do CSV (colunas) em anexo.
- **low** FR-11 duplica o que FR-1, 2, 3, 4 e 15 já dizem, e mistura justificativa de design ("de modo que o drill-down não mostre dados posteriores"). *Fix:* manter FR-11 como regra única e referenciar nos demais, ou retirar a duplicação.
- **low** FR citando implementação: "view" e "texto MM/AAAA" (FR-1), "rota", "API" (FR-18, FR-19), "associação por prefixo" (FR-6; "prefixo" não está no glossário nem explicado). Parte é justificável por ser brownfield, mas capacidade deveria vir primeiro. *Fix:* reescrever em termos de capacidade; formato do campo para o anexo de dados.

## Scope honesty — adequate
Non-Goals e Out of Scope existem e fazem trabalho real (sem escrita no banco do cliente, sem detalhe até documento, sem exportação). `[ASSUMPTION]` e `[NOTE FOR PM]` estão em quantidade (5 assumptions, 7 notas, 11 questões) alta para uma release que se declara pronta para fluxos seguintes, mas é coerente com status draft.

### Findings
- **high** Contradição entre "fora de escopo: verbas" e escopo (§0, §4.1, §5 vs. Glossário/§6.1) — §0 diz "Fica de fora tudo que for verbas", §5 "Não cobre verbas", mas §4.1 inclui "verbas" no Realizado por Conta via CSV e o DRE tem a linha "CMV, Deduções e Verbas". O que está fora é a *gestão* de verbas de indústria (Fase 1 do roadmap), não as contas de verba no DRE. *Fix:* reescrever §0 e §5 para "módulo de verbas (apuração, contratos, ressarcimento)".
- **medium** Brownfield sem distinção novo/existente (§0, todo §4) — §0 afirma que a Fase 1 já existe (DRE, fechamento versionado, mapa, trilha, CSV). FR-5, 6, 7, 9, 10, 19 e parte de FR-3 provavelmente descrevem o que já existe; o PRD não marca quais, nem se são requisitos de regressão ou trabalho novo. Isso inflaria o backlog ou duplicaria stories. *Fix:* coluna/tag "existente | ampliar | novo" por FR (não verificado contra o código por esta revisão).
- **medium** Suposições implícitas fora do índice (§9) — não marcadas: "volume pequeno, pode reler o histórico inteiro" (FR-1); "nulo = zero" (FR-2); "formato `MM/AAAA` confirmado" (FR-1); "Ano anterior sempre existe/mesma base" (Glossário; e o que acontece no primeiro ano); precedência "mais específico prevalece" (FR-6); "uma só Empresa" (Glossário); "e-mail institucional disponível para cada Gerente" (FR-14); limite "em percentual e/ou valor" (FR-13). *Fix:* marcar e indexar.
- **low** O `[ASSUMPTION]` de FR-14 está dentro de uma `[NOTE FOR PM]` e sem texto próprio (§4.6); o índice o redige, mas inline é só a etiqueta. *Fix:* padronizar `[ASSUMPTION: ...]`.
- **low** `[NON-GOAL for MVP]` usado só para exportação (§5); "limite por departamento" aparece só em §6.2 e FR-13. Sem erro, mas conviria consistência.

## Downstream usability — adequate
IDs de FR (1 a 19), UJ (1 a 4) e SM (1 a 4, C1, C2) são contíguos e únicos; "Validates"/"Realiza" resolvem. Glossário é forte, mas há deriva de termos e furos de rastreabilidade.

### Findings
- **medium** Jornada sem FR correspondente: carga do dicionário/arquivo de contas (UJ-1: "carrega o arquivo de contas gerenciais"; FR-6 Notes: "cadastrado por arquivo") e gatilho do envio ao fechar (UJ-2: "dispara o envio do DRE aos gerentes" vs. FR-9 e FR-14 que não definem o disparo) e a acompanhamento do status de Justificativa pelo analista (UJ-1 Resolution; FR-12 só cobre painel por Departamento). *Fix:* FR para importar o dicionário de contas; consequência em FR-9/FR-14 sobre o disparo; escopo do painel em FR-12.
- **medium** FRs sem jornada (FR-5, 13, 15, 17, 19; FR-13/15/17 também sem "Realiza") — configuração do limite, Rateio, tendência/ponte/projeção e cadastro de usuários não têm protagonista. Também não há UJ para o administrador. *Fix:* estender UJ-2 (Helena define limite e aplica Rateio) e UJ-4 (Sônia vê tendência/ponte), ou aceitar capacidade de suporte sem UJ.
- **medium** Deriva de glossário: "gestores" (FR-4, UJ-1) vs. **Gerente**; "gerente" minúsculo (UJ-3, §2.1); "painel" (FR-12) vs. "dashboard" (§4.8, UJ-4); "venda líquida" (FR-7, §4.3) vs. **Faturamento Líquido** (§4.2); "mês"/"mês fechado" vs. **Competência**/**Fechamento**; "conta(s)" solto em FR-12 e UJ-3 vs. **Conta gerencial**. Perfis (**analista, Controller, diretoria, administrador, controladoria**) são usados centralmente em FRs mas não estão no glossário; "controladoria" e "analista" parecem sinônimos parciais. "Departamento centralizado" (FR-15) e "critério de Rateio" também não estão definidos. *Fix:* acrescentar entradas de Perfil ao glossário e normalizar os termos.
- **low** Numeração de Open Questions fora de ordem (§8): 1…9, **11**, **10**; FR-17/§4.3 referencia "pergunta aberta 10", que aparece por último. *Fix:* reordenar 10 antes de 11.
- **low** Título provisório ("*Título provisório — confirmar*", H1) e `status: draft` — esperado, mas deve ser resolvido antes de alimentar UX/arquitetura.
- **low** §4.9 posiciona a "Feature-specific NFRs" (folha de pagamento) depois de FR-19, desconectada de FR-18 e de OQ-7. *Fix:* mover para FR-18 ou FR-12.

## Shape fit — adequate
B2B multi-stakeholder com perfis distintos: UJs com protagonistas nomeados são carga útil, e a densidade (4 UJs para 4 perfis) é adequada. O formato de uma "release" derivada de um produto maior, brownfield, pede distinção entre existente e novo (ver Scope honesty). Para um PRD que alimenta UX → arquitetura → épicos, a rastreabilidade mínima (FR↔UJ↔SM) deveria estar mais completa.

### Findings
- **low** Nomes de personas (Rafael, Helena, Paulo, Sônia) — confirmar que são fictícios e não pessoas reais do cliente; em repositório público, nomes reais de funcionários seriam dado do cliente.

## Mechanical notes
- **IDs:** FR-1 a FR-19 contíguos e sem repetição; UJ-1 a UJ-4; SM-1 a SM-4 + SM-C1/C2. Única irregularidade: Open Questions 9, 11, 10.
- **Referências cruzadas:** todas as "Realiza UJ-x" e "Validates FR-x" resolvem. "pergunta aberta 10" (§4.3) resolve, mas está fora de ordem. FR-9 ("dispara o envio", UJ-2) e FR-14 não se ligam explicitamente.
- **Roundtrip de suposições:** as 5 entradas do índice (FR-1, FR-13, FR-17, FR-14, SM-3) existem inline. Em sentido contrário, há pelo menos 7 suposições implícitas sem tag (ver Scope honesty). A tag de FR-14 está sem texto inline.
- **Protagonistas das UJs:** todas nomeadas, com contexto inline.
- **Deriva de glossário:** gestores/Gerente, painel/dashboard, venda líquida/Faturamento Líquido, mês/Competência; perfis não definidos no glossário.
- **Seções exigidas:** presentes (propósito, visão, usuário, glossário, funcionalidades, non-goals, escopo, métricas, questões abertas, índice). Faltam NFRs e a marcação novo/existente por FR.

### Dados do cliente em repositório público (atenção)
Não há nome da empresa, CNPJ, valores monetários nem contrato; §0 declara a regra, e ela é majoritariamente cumprida. Itens que merecem decisão do dono do produto:
- **medium** Contagens específicas do cliente em §8: "a view de realizado tem 8 filiais e a de orçamento só 5" (OQ-11) e "uma filial em 7 estados" (OQ-9). Combinadas com "grupo de distribuição" (§0), "protótipo de painel que o cliente já usa" (FR-17) e o desenho completo do DRE do cliente (§4.2: linhas e ordem), tornam o cliente potencialmente identificável. *Fix:* generalizar ("algumas filiais da view de realizado não têm Orçado"; "o mesmo código de Filial aparece em várias UFs").
- **low** Nomes de campos e valores do ERP/banco do cliente: `CODFILIAL` (Glossário), formato `MM/AAAA` da view, `AC` e `GRUPO` (OQ-3), "centenas de linhas" (FR-1), lista de componentes do consolidado ("CMV de devolução, ST, desconto de custo"). Baixo risco, mas revelam o esquema de dados do cliente. *Fix:* mover para um anexo privado (fora do repositório) ou generalizar.
- **low** A estrutura do DRE do cliente (§4.2) é informação de negócio; avaliar se pode ser genérica.
- **Operacional:** `git status` mostra `_bmad-output/` como não rastreado; antes de qualquer commit, decidir se esta pasta (que inclui `.memlog.md`, não revisado por mim) entra em `.gitignore` ou se o conteúdo precisa de sanitização.
