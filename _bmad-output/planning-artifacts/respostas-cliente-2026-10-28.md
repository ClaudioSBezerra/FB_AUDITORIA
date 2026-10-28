# Respostas da área de negócio do cliente (28/10/2026) — resumo

Só estrutura e regras. Os e-mails dos gerentes, a lista de aviso e os percentuais de rateio são dado do cliente: entram pelo sistema (tela de configuração ou carga), nunca em arquivo versionado.

## Decisões

1. **Fechamento mensal:** todo dia 1 ou primeiro dia útil do mês. O disparo do fechamento é iniciado manualmente pela controladoria, depois da revisão geral e da revisão dos e-mails (trocas por férias etc.). Não é automático por data.
2. **Disparo parcial semanal:** toda sexta-feira, com o acumulado do mês até aquele dia (semana 1 de 01 a 07, semana 2 de 01 a 14 etc.), em nível de lançamento. É requisito novo.
3. **Rateio:** nova aba, com duas visões: a normal (padrão do sistema) e a rateada por faturamento. Configuração por caixas: incluir ou tirar departamentos do rateio e editar à mão o percentual por filial. Há uma matriz departamento × filial que diz em quais filiais cada departamento entra, e o percentual padrão por departamento e filial.
4. **Destinatários:** lista de e-mails por gerente, organizada por bloco (matriz consolidada e cada filial com gerente). Cada gerente tem a sua lista, e alguns e-mails se repetem em várias listas (controladoria, TI).
5. **Desvios:** todos ficam disponíveis para comentário e justificativa.
6. **Folha:** só o lançamento consolidado do departamento; a abertura salarial não existe no sistema.
7. **Ajuste manual:** sem ajuste manual nas contas do DRE, "apenas direto no sistema". Controladoria e analista configuram disparos, percentuais de rateio etc. Quatro pessoas são a lista de aviso de ajustes.

## Impacto no planejamento (a decidir)

- **Confirmado pelo cliente (28/10/2026): o ajuste vem do ERP e chega ao sistema pelo banco, junto da ingestão. O sistema não tem lançamento manual de ajuste.** Consequência: a FR-4 como escrita (lançar ajuste no sistema) cai; as histórias 5.4, 5.5 e 5.7 e o AD-6 precisam ser reescritos. Mudança ainda não aplicada no `epics.md`, no PRD, na arquitetura nem no quadro.
- **Épico 5 (antes da confirmação):** o "ajuste manual imutável" (FR-4) provavelmente perde o sentido: precisa ser reinterpretado ("direto no sistema" = lançamento dentro do sistema?). Perguntar ao cliente. Disparo semanal e disparo de fechamento manual entram no outbox de e-mail (AD de e-mail).
- **Épico 7 (rateio):** deixa de depender de resposta do cliente. Já há regra: visão por faturamento, matriz de inclusão, percentual editável por filial.
- **Épico 2/8:** destinatários e percentuais precisam de tela ou carga, e não de seed no repositório.
- **Folha:** consolidada por departamento, sem dado individual (também é questão de LGPD).

## Relatório da rotina 124 (balancete por lançamento) — o que ele mostra

O arquivo de exemplo (um grupo de despesas, quinzena de janeiro) não vai para o repositório; só a estrutura.

- **Agrupamento:** grupo (código de 3 dígitos mais nome, que é o departamento do rateio) > conta (7 dígitos, os 3 primeiros iguais ao código do grupo) > lançamentos, com total por conta, por grupo e geral.
- **Campos por lançamento:** data de pagamento, número do lançamento, histórico, número do documento, código e nome do fornecedor, valor pago (despesa negativa, receita e estorno positivos), banco/cheque, transação, filial, usuário que lançou.
- **Parâmetros do relatório:** lista de filiais (mais filiais do que as 8 do rateio), período, "desconsiderar ST" e "desconsiderar beneficiamento".
- **Conclusão:** este é o realizado por conta gerencial, ao nível de lançamento, que faltava (a lacuna das views). O sinal vem do próprio valor. Contas de acerto de estoque e ICMS sobre ajuste de inventário aparecem como lançamentos comuns: é o "ajuste que vem do ERP".
- **Folha:** os lançamentos de salário são pagamentos agregados por filial; mesmo assim o histórico de outras contas traz nome de pessoas (rescisões, vales). O sistema não deve guardar nem exibir o histórico dessas contas sem decisão de LGPD.

### Perguntas novas para o dono dos dados (Keslley)

1. De qual tabela ou view do Oracle sai esse relatório (rotina 124), e dá para ter uma view só de leitura por lançamento, como as do consolidado e do orçado?
2. O DRE é por **data de pagamento** (o relatório) ou por **competência**? O histórico traz meses de referência diferentes do mês do pagamento.
3. Os parâmetros "desconsiderar ST" e "desconsiderar beneficiamento" valem NÃO no DRE?
4. As filiais do relatório (cerca de 20) são todas do mesmo CNPJ/empresa, e só 8 entram no rateio?

## Respostas às perguntas 2 a 4 do dono dos dados (Marcos, 28/10/2026)

- **Regime:** o DRE é por **data de pagamento**, como o relatório da rotina 124.
- **Parâmetros "desconsiderar ST" e "desconsiderar beneficiamento":** nenhum dos dois é usado; valem NÃO.
- **Filial gerencial:** três filiais gerenciais são compostas por várias filiais do ERP (a matriz agrupa a maior parte delas; duas outras agrupam duas filiais cada); as demais são filiais soltas, que valem por si. A composição é configuração por empresa, nunca código nem arquivo versionado (os códigos entram pelo sistema).

### Impacto no planejamento

- Entra um conceito novo: **filial gerencial = conjunto de filiais do ERP**. O realizado, o orçado e o rateio devem ser lidos e gravados por filial gerencial; a ingestão (Épico 2) precisa do mapa filial do ERP para filial gerencial.
- Isso provavelmente resolve a lacuna das filiais sem orçado (elas entram no consolidado da filial gerencial); confirmar com o cliente.
- O snapshot por filial da 1.8 e a matriz do rateio (Épico 7) passam a usar a filial gerencial como chave.
- Competência pelo **pagamento**: a coluna de data da ingestão é a data de pagamento; a pergunta de competência fica encerrada.

## Respostas do dono dos dados (Keslley) às perguntas do PRD, 28/10/2026

Só estrutura e regras; nomes de views, host e credenciais ficam fora do repositório.

**Realizado por conta gerencial**
- Existe view (ou tabela) com o realizado por conta gerencial, por filial e mês. A forma de apresentar está sendo alinhada com o Marcos. Ainda não temos o nome nem as colunas.
- CMV e "Outros Faturamentos": as colunas correspondentes às rotinas 8103 e 124 foram adicionadas ao consolidado. "Outros Faturamentos" **não deve aparecer** no DRE (Marcos). Como o CMV entra continua em aberto com o Marcos.
- O orçamento inclui venda bruta e devoluções; o CMV orçado depende do Marcos.
- A folha é controlada em outro sistema (RM); o ERP guarda só o consolidado das contas salariais por departamento e conta (combina com a regra da folha só consolidada).

**Dicionário e filiais**
- O cadastro de contas tem descrição, grupo e tipo, mas **não existe vínculo entre conta e departamento, nem entre conta e filial**. O departamento vem do grupo da conta. O vínculo conta-departamento que o PRD supunha precisa ser decidido (provavelmente pelo grupo da conta, a validar).
- Foram criadas duas views novas: uma de contas, grupos e tipo de conta, e outra com as filiais cadastradas. O SELECT para o usuário de integração é concedido sempre que o dono dos dados cria uma view do projeto. O IP do servidor de produção já está liberado; a máquina de desenvolvimento não.
- A UF do consolidado é a **UF de destino da venda**, não a da filial. Ela é uma dimensão à parte; a chave do DRE por filial continua sendo a filial gerencial.
- As filiais 13, 18 e 29 não têm orçado só porque não estavam na planilha usada para subir o orçado; a fonte do orçamento é a mesma. O orçado delas será carregado: resolve a lacuna.

**Sinais e atualização**
- Sinal das colunas: o entendimento do dono dos dados é "negativo" para devoluções e custos, mas precisa da confirmação do Marcos antes de fixar em `dre_mapa.sinal`.
- Hoje as views atualizam no início de cada mês, consolidando o mês anterior. Isso **conflita** com o disparo semanal de sexta (acumulado do mês) pedido pelo cliente: é preciso alinhar a frequência e o formato da atualização com o dono dos dados. Pergunta aberta: lançamentos retroativos em mês fechado.

**Ainda em aberto com o Marcos:** significado de AC e GRUPO nas bases; CMV e sinais; se a view por lançamento (nota, título) pode ser exposta, já que o dono dos dados não sabe se é dado sensível para esta finalidade.

### Impacto no planejamento
- A ingestão do Oracle (Épico 2) ainda não pode ser construída por completo: faltam o nome e as colunas da view de realizado por conta e a definição de CMV e sinais. Dá para construir o adaptador do que já existe (consolidado e orçado) e as duas views novas (contas e filiais), e provar a conexão a partir do servidor de produção, não daqui.
- O vínculo conta-departamento precisa de decisão (a regra por grupo da conta é uma hipótese a validar com o Marcos).

## Respostas do Marcos às perguntas de negócio (28/10/2026)

Só estrutura e regras; composição de filiais e dados reais ficam no sistema.

- **CMV:** usa o da rotina 124. O CMV orçado fica gravado na conta correspondente do orçamento.
- **Sinais:** na 124 todos os valores vêm **positivos**; o sistema recalcula o sinal por conta (só o CMV é negativo). Vai em `dre_mapa.sinal`.
- **AC e GRUPO da planilha:** `AC` = acumulado do ano. `Grupo` = soma de todas as filiais. `AC + filial` = acumulado da filial; `filial` = só a filial filtrada; `AC + Grupo` = acumulado do grupo. Existem visões nomeadas que somam algumas filiais gerenciais (Balcões e Distribuição). Nova história **2.15**.
- **Departamento:** é o do grupo da conta (confirmado; por exemplo, o grupo 402 é Centro de Distribuição).
- **Apresentação:** o DRE mostra por **grupo**; clicar no grupo abre as contas; clicar na conta abre os **lançamentos** (data, fornecedor, nota, valor, filial). Isso conflita com a 1.8, que parou o detalhe de mês fechado na conta. Nova história **3.9**.
- **Histórico com nome de pessoa:** pode aparecer na tela e no e-mail (já é visível hoje nos acompanhamentos, autorizado por eles).
- **Atualização:** semanal. Não existe lançamento com data retroativa; é raro estornar pagamento do mês anterior (entra no mês atual). Pode haver correção no primeiro dia útil do mês, e depois é preciso buscar os dados de novo. Só a gestão pede a atualização. Nova história **2.16** (botão "Atualizar dados"); corrigir mês fechado = reabrir (1.7), atualizar, fechar de novo.
- **Limite de desvio:** não há limite. Todo desvio tem a linha de justificativa, **não obrigatória**. As histórias 5.1 e 5.2 foram ajustadas.
- **Ajuste:** só a controladoria faz ajuste e alteração de conta. O gerente justifica **só o desvio**; se houver lançamento em conta incorreta, ele pede à controladoria que realoque. As histórias 5.5 e 5.7 foram **descartadas**.
- **Rateio:** gravado **mês a mês**, tanto por faturamento quanto por percentual manual (7.1 ajustada).
- **Quem reabre:** só a gestão (a controladoria, que já é o perfil que reabre na 1.7).

### Perguntas que continuam abertas
- Nome e colunas da view de realizado por conta (Keslley); as colunas data de pagamento, fornecedor e documento precisam estar nela (3.9).
- Formato exato da planilha para a história 2.2 (cabeçalho e se é CSV ou .xlsx).
- Frequência e formato da atualização das views, alinhados entre o Keslley e o Marcos (semanal).

## Pedido do Marcos: visão por trimestre, e o bloco de faturamento da planilha (28/10/2026)

- **Trimestres:** a planilha enviada não tem a visão por trimestre, e o Marcos quer. Nova história **3.10** (T1 a T4 e o total do ano, com realizado, orçado, ano anterior e percentual).
- **Estrutura do bloco que a planilha mostra** (valores reais não registrados aqui): colunas Código, Conta, Vl. Realizado, Ajuste e Saldo. Linhas: Faturamento Bruto (código 1), Devoluções de Cliente (2), Devoluções avulsas (2), Outros Faturamentos (código de transferência), Faturamento Líquido, CMV Devolução (3), CMV devolução avulsa (3), CMV (3) e Saldo Bruto.
- **Convenção de sinal confirmada pelo exemplo:** o `Vl. Realizado` vem positivo e a coluna `Saldo` traz o valor com o sinal do DRE (devoluções e CMV negativos). O Faturamento Líquido é bruto mais outros faturamentos menos devoluções; o Saldo Bruto é o líquido menos o CMV, mais o CMV de devolução. O sinal fica em `dre_mapa.sinal`, como já decidido.
- **Pergunta nova ao Marcos:** o bloco de exemplo inclui **Outros Faturamentos** no Faturamento Líquido, mas a resposta anterior dizia que "Outros Faturamentos" não deve aparecer no DRE. Qual vale: incluir no Faturamento Líquido, mostrar à parte ou ocultar?
