# Reconciliação PRD x Spine — FB_AUDITORIA A1

Insumo: `prds/prd-FB_AUDITORIA-2026-10-27/prd.md`. Derivado: `ARCHITECTURE-SPINE.md`.
Método: cada FR e cada "Consequence (testable)" confrontados com os ADs, as convenções, o mapa e o Deferred.

## Resumo

- Os 20 FRs estão ligados a algum AD (`binds` e Capability Map).
- Cobertos sem lacuna relevante: FR-2, FR-5, FR-6, FR-10, FR-18, FR-19 (6 FRs, ressalvas menores abaixo).
- Cobertos com pelo menos um requisito silencioso ou conflitante: FR-1, FR-3, FR-4, FR-7, FR-8, FR-9, FR-11, FR-12, FR-13, FR-14, FR-15, FR-16, FR-17, FR-20 (14 FRs).
- Os que mais pesam: FR-13 (conflito com AD-7), FR-4 (aviso por e-mail sem desenho), FR-20 (escrita fora do Ingestor, dicionário sem tabela), FR-15 (arredondamento da soma), FR-14 (granularidade do envio com fechamento por filial).

## A. Requisitos do PRD que NÃO aterrissaram (ou aterrissaram em conflito)

### A1. FR-13 x AD-7 — limite não reclassifica mês fechado (conflito, alta gravidade)
- PRD FR-13: "Alterar o limite não reclassifica retroativamente Competências com Fechamento ativo."
- Spine AD-7: "Pendente" ou "justificada" é derivado, nunca persistido. Limite é um único valor em `config_empresa` (Deferred, Convenções).
- Se o status é derivado do limite vigente, mudar o limite muda retroativamente o que é "desvio que exige justificativa" em mês fechado. Nenhum AD congela o limite por Competência ou Fechamento (por exemplo, copiar o limite para `dre_fechamentos` ou guardá-lo versionado). Também não há dono da regra "o que é desvio acima do limite" (valor e/ou percentual, base orçado e ano anterior).
- Afeta UJ-3 (o Paulo seleciona "contas com desvio acima do limite"), SM-3 e SM-4.

### A2. FR-4 x AD-13 — aviso por e-mail do Ajuste não tem mecanismo
- PRD FR-4 e UJ-1: cada Ajuste avisa por e-mail a Lista de aviso.
- AD-13 amarra `envios` a `UNIQUE (fechamento_id, user_id)`, preenchida na transação do Fechamento. Um Ajuste não tem `fechamento_id` (ocorre com a competência aberta) e a Lista de aviso é grupo de e-mail, não `user_id`.
- FR-4 consta em `binds` de AD-13, mas o esquema só serve ao FR-14. Faltam: outbox para aviso de ajuste, destinatário que não é usuário, e a regra de que falha de envio não pode reverter o Ajuste (nem de que a transação do Ajuste grava o item do outbox).
- O "NOTE FOR PM" (aviso a Gerentes afetados ou a lista fixa) foi para o Deferred, mas o desenho do outbox precisa suportar os dois destinatários.

### A3. FR-20 x AD-2 — dicionário de contas sem dono de escrita e sem tabela
- PRD FR-20: carga do dicionário (conta, descrição, grupo, Departamento); rejeita conta repetida e Departamento inexistente; nada gravado com erro bloqueante; reenvio atualiza sem apagar associação a Linha do DRE; grava na Trilha.
- AD-2 lista FR-20 em `binds`, mas a regra restringe o `dre.Ingestor` a `lancamentos` e `orcado_conta`. Quem escreve o dicionário não está definido.
- O ER e AD-10 só têm `departamentos`, `conta_departamento`, `usuario_departamentos`. Não há lugar para descrição e grupo da conta.
- Não há contrato de "atualiza sem apagar `dre_mapa`" (upsert que não toca o Mapa) nem de validação em lote com lista de erros.
- Pergunta aberta 2 (fonte do dicionário) não aparece no Deferred.

### A4. FR-15 x AD-14 — soma rateada igual ao valor, recusa de percentuais != 100%
- PRD FR-15: "a soma dos valores rateados é igual ao valor do Departamento centralizado; recusa percentuais que não somam 100%". SM-7 mede isso em 100% das competências fechadas.
- AD-14 não menciona nenhum desses dois. Com `NUMERIC(18,2)` o rateio proporcional gera resto de centavos; a spine não decide onde o resto cai (última filial, maior filial). Sem isso, a invariante do PRD falha em teste.
- Também ausente: quem valida os 100% (configuração ou aplicação) e o que ocorre com filial sem faturamento no critério proporcional.

### A5. FR-14 x AD-13 — granularidade do envio com fechamento por filial
- PRD FR-9 permite fechar por Filial; FR-14 diz que o envio é disparado pelo Fechamento, e cada Gerente recebe só o seu Departamento.
- AD-13: uma linha por `(fechamento_id, user_id)`. Com fechamentos por filial há vários `fechamento_id` por competência; o Gerente receberia um e-mail por filial, ou um e-mail parcial. Também não define o que ocorre numa reabertura seguida de novo fechamento (novo `fechamento_id`, novo envio: provavelmente certo, mas não dito).
- Gerente com mais de um Departamento (assunção do PRD): um e-mail com seções ou um por departamento? `user_id` único implica um, sem dizer.
- Não dito: o link para a tela de Justificativa (URL base na configuração), e que o envio grava na Trilha (destinatário, momento, competência). `envios` pode servir de trilha, mas FR-14 pede registro "na Trilha de auditoria" e a convenção geral não diz que o worker chama `writeAuditLogTx`.

### A6. FR-1 — mensagens e validação de entrada
- PRD: valor de mês fora do formato `MM/AAAA` é rejeitado com mensagem que identifica a linha. A spine não menciona parsing, rejeição nem identificação da linha (só `409` listando escopos).
- PRD: a importação grava na Trilha "quem iniciou, intervalo importado, quantidade de linhas". AD-15 prevê agendador sem usuário; a spine não define o autor da Trilha para execução agendada (ator "sistema"), nem o intervalo e a contagem. `carga_id` ajuda, mas não substitui o registro.
- PRD: Realizado da Filial = soma das UFs. A convenção diz "a UF nunca faz parte da chave", mas ninguém é designado a somar as UFs (adaptador Oracle, presumivelmente) nem a garantir que a soma ocorre antes do `INSERT`.

### A7. FR-3 — validação tudo ou nada com lista de linhas inválidas
- PRD: arquivo validado antes de gravar, linhas inválidas listadas, nada gravado com erro bloqueante. A spine fixa transação por escopo (AD-3), o que garante atomicidade por escopo, não por arquivo inteiro com várias competências e filiais. Se uma competência estiver fechada e outras abertas, a regra de AD-3 diz "devolve 409 listando os escopos recusados" mas não diz se as demais abertas são gravadas. Pelo PRD ("nada é gravado se houver erro bloqueante") deveria ser tudo ou nada; o recusar-por-fechamento pode ou não ser erro bloqueante. Indefinido.

### A8. FR-8 e FR-7 — regras de exibição não ancoradas
- Variação = Realizado menos Orçado, em valor e %; Orçado zero mostra "não calculável"; "sem orçado" para filial ou conta sem orçado.
- Percentual sobre Faturamento Líquido tendo essa linha como base.
- Para competência com Fechamento ativo, o DRE usa os valores congelados (e, em fechamento por filial, como misturar filial fechada e aberta numa visão consolidada).
- O detalhe mostra a origem de cada valor (banco, arquivo, Ajuste) e a tela de detalhe mostra carregado e Ajuste separados.
- A spine só diz que o orçado ausente é "sem orçado" (AD-5) e que o Realizado é `SUM(lancamentos)+SUM(ajustes)` (AD-6). "Ano anterior" não aparece uma vez sequer na spine; nenhum AD diz de onde sai (mesma consulta de Realizado um ano antes, respeitando fechamento?). As regras acima foram tratadas como "existe na Fase 1" (PRD 6.4), mas a spine não confirma contra o código nem cita onde ficam.
- Leitura de fechado: AD-9 torna `dre_fechamento_linhas` imutável, mas nenhum AD diz que a consulta de DRE lê do snapshot quando há fechamento ativo.

### A9. FR-17 e FR-16 — invariantes dos dashboards
- Ponte do resultado fecha (soma das contribuições = diferença total).
- Projeção marca quais meses são realizado e quais estimados, e o método (`realizado acumulado + orçado restante`).
- FR-16: status da Justificativa em cada Desvio e contagens por Departamento (pendentes e justificadas).
- A spine só mapeia FR-16/17 a `dre.Realizado` + handlers, sem decidir onde moram essas consultas nem as invariantes. Aceitável como adiado (FR-17 é "depois do núcleo"), mas não está no Deferred.

### A10. FR-11 — texto da recusa
- PRD: recusa com mensagem de que é preciso reabrir o Fechamento (UJ-1 repete). A spine fixa o `409` e a lista de escopos, não o conteúdo da mensagem ao usuário. Menor.

### A11. FR-19 e UJ — papel de administrador
- FR-19: usuários só são criados por administrador. A spine mantém `ALLOW_SELF_REGISTER=false` e a área `configuracoes` (usuários), mas "administrador" não está na matriz de capacidades de AD-11 nem entre os quatro Perfis do PRD. Quem é administrador (perfil existente do Farol, flag) não está dito.
- A Trilha na criação de usuário e mudança de perfil depende só da convenção geral; não há menção explícita.

### A12. Perguntas abertas sem destino
- Pergunta 10 (filiais sem Orçado): AD-5 trata como "sem orçado", mas a pergunta não consta no Deferred, nem a possibilidade de orçamento vir de outra fonte.
- Pergunta 4 (data de fechamento e envio automático): o Deferred cita "perguntas 4 e 5" só para o Rateio (herdando a confusão do PRD §6.3). O gatilho automático por data não tem lugar na spine; só o envio pós-Fechamento.
- Perguntas 1 e 2 (fonte do realizado por conta e do dicionário): 1 está; 2 não (ver A3).
- Perguntas 3, 6, 7, 8, 9, 11 estão no Deferred.

### A13. Itens menores
- SM-C2 (reaberturas devem ser visíveis): AD-9 guarda os dados, mas nenhuma leitura os expõe.
- Repositório público (PRD §0: nunca contrato, valores, CNPJ ou números reais): a spine não registra restrição para fixtures, testes e configuração. Tem eco no CLAUDE.md, não no desenho.
- Ajuste: o PRD o define por Conta gerencial e Competência, sem Filial; a Guarda (AD-8) e a soma por escopo exigem `filial`. A spine não diz se o Ajuste carrega filial (provável) nem quem escolhe; isso afeta o fechamento por filial.
- Fonte dupla: CSV e Oracle coexistem com `fonte` na chave de substituição (AD-3), mas nada impede a mesma conta vir das duas fontes e somar em dobro quando a fonte Oracle assumir.
- Folha (NFR única do PRD): o gancho "marcador de sensibilidade em `conta_departamento`" está no Deferred. Coberto como adiado.

## B. Onde a spine decide algo que contradiz ou ultrapassa o PRD

1. **AD-12 renomeia as áreas do menu** (`estrategico`→`administrativo`, `administrativo`→`configuracoes`, `operacional`→`operacao`, reservando `comercial`). O CLAUDE.md do projeto fixa "três áreas — Estratégico, Operacional, Administrativo" em `navigation.ts`. O PRD não pede nada disso. É decisão marcada `[ADOPTED]`, mas contradiz a restrição escrita do projeto e implica migração de `users.areas` no A1.
2. **Perfil "auditoria"** (AD-11: "Diretoria e auditoria nunca escrevem"). O PRD tem quatro perfis e põe auditoria externa em Non-Users. A spine introduz (ou herda do Farol) um perfil que o PRD exclui.
3. **AD-15 agendador no processo da API** com varredura automática. O PRD só diz que a importação "pode reler o histórico inteiro"; não pede agendamento. Cria o problema do ator "sistema" (A6) e adiciona um segundo caminho de escrita.
4. **AD-15 ping de 30 s, timeouts, advisory locks, Cloudflare Tunnel, uma réplica**: infraestrutura além do PRD. Coerente, mas é escopo novo (operação).
5. **`dre_orcado` vira legado, orçado por conta em `orcado_conta`** (AD-5). O PRD fala de Orçado por Conta gerencial e Filial (FR-2), mas a Fase 1 tinha orçado por linha (6.4 "Existe"). A spine cria uma segunda fonte do orçado por um período. O PRD não cobra a migração.
6. **`analista` entra no CHECK de `users.perfil`** (AD-11): o PRD supõe que o perfil existe ("Existe (perfil e áreas validados)") e a spine mostra que não existe no código. A tabela 6.4 do PRD ("a arquitetura confirma contra o código") fica corrigida pela spine: nesse ponto o PRD estava otimista.
7. **Fechamento consolidado como registro único com filial vazia** (AD-9) em vez de "fecha todas as Filiais" (FR-9). Equivalente na guarda, mas diferente em histórico: consolidado e fechamentos individuais por filial coexistem sem regra de precedência ou de conflito (fechar filial A, depois consolidar; reabrir só a filial A estando consolidado).
8. **Mapa por prefixo reaproveitado em `conta_departamento`** (AD-10): PRD FR-6 só pede a precedência "mais específico vence" para Linha do DRE; estender a mesma regra para Departamento é escolha da spine.
9. **Tabela `config_empresa` JSONB para limite, lista de aviso e quem ajusta**: o PRD só exige o limite configurável; guardar "quem lança Ajuste" como configuração enquanto AD-11 o fixa em matriz de capacidades por perfil em código é uma duplicidade (qual vence).

## C. Verificação por seção

- **FR-1 a FR-20**: ver A e B.
- **Non-Goals**: sem escrita no banco do cliente (AD-15, diagrama: coberto); sem detalhe até o documento (Deferred cita `tipo_doc`/`doc_ref`); sem exportação PDF/Excel e não replicar Excel (nada a decidir). Coberto.
- **NFRs**: único NFR do PRD é a folha (Deferred). Os demais (desempenho, disponibilidade) não existem no PRD; a spine inventou os seus (ping, timeout).
- **UJ-1**: borda "mês fechado recusa e informa reabrir" (AD-3, texto da mensagem em A10); aviso por e-mail (A2); acompanhar status de pendência (derivado, AD-7, sem leitura definida para o analista).
- **UJ-2**: recusa com conta sem mapeamento listando contas (existe; AD-9 não reafirma); envio disparado (AD-13, ver A5).
- **UJ-3**: negar outro departamento (AD-10, `403`); "desvio acima do limite" (A1).
- **UJ-4**: somente leitura (AD-11); filtros e justificativa registrada ou pendente (A9).
- **Perguntas abertas**: ver A12.
