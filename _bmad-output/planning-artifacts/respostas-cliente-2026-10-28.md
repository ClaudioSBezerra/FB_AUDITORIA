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
