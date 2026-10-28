# Epic 1 Context: Fechar o mês com segurança

<!-- Compiled from planning artifacts. Edit freely. Regenerate with compile-epic-context if planning docs change. -->

## Goal

O Controller fecha o mês do grupo inteiro e o reabre com motivo, sabendo que ninguém altera números fechados. A equipe valida o sistema num ambiente de Teste que pode ser apagado e que vira Produção de forma irreversível. O fechamento e a importação CSV de hoje (que gravam direto nos handlers, sem teste algum) passam a viver num pacote próprio, com guarda de período e testes, antes de qualquer fonte nova (Oracle) entrar na release.

## Stories

- Story 1.1: Testes que registram o fechamento e a importação de hoje
- Story 1.2: Marcador Teste/Produção e apagar os dados de teste
- Story 1.3: Virar para Produção e faixa de ambiente
- Story 1.4: Importação CSV passa pelo Ingestor
- Story 1.5: Fechamento passa pelo serviço `dre`
- Story 1.6: Guarda de período e bloqueio de escrita em mês fechado
- Story 1.7: Um fechamento ativo por mês e reabertura com motivo
- Story 1.8: Mês fechado lido do que foi congelado
- Story 1.9: Nenhum endereço do banco do cliente no repositório

## Requirements & Constraints

- O fechamento é sempre do grupo inteiro, só pelo Controller (perfil `controladoria`; o admin de plataforma passa em tudo), com versão e registro na trilha. É recusado se houver conta com lançamento sem linha do DRE.
- Reabrir exige motivo, cria nova versão no próximo fechamento e mantém as anteriores legíveis. Enquanto reaberta, a competência volta a aceitar carga.
- Carga, importação e ajuste são recusados em competência com fechamento ativo, com a mensagem de que é preciso reabrir.
- Teste/Produção por empresa: nasce em Teste; a virada é de mão única; apagar e virar exigem admin de plataforma e a senha digitada de novo, e gravam na trilha. Em Produção nada de domínio é apagado, nem por chamada direta à API. Apagar mantém a estrutura do DRE, o Mapa e os usuários; a trilha nunca é apagada.
- Toda tela mostra a faixa "AMBIENTE DE TESTE" enquanto o ambiente for Teste.
- Valores monetários são exatos (`NUMERIC(18,2)`); todo subtotal é a soma de seus componentes.
- Nenhum endereço do banco do cliente nem dado real do cliente no repositório; segredos só em variáveis de ambiente.

## Technical Decisions

- Camadas: handlers só decodificam, autorizam e respondem; a regra mora em `backend/dre`. `dre` define a porta `Fonte` e os tipos; `dre/fontes/csv` a implementa e não acessa Postgres; `dre` não importa `handlers`. Só `dre.Ingestor` escreve em `lancamentos`, e um teste falha o build se outro pacote o fizer.
- Reimportar substitui por `(empresa, origem, competência, filial)` numa transação; leitura vazia nunca apaga; `filial` vazia é rejeitada; escopo em período fechado é recusado com `409` listando os escopos.
- `dre.Periodo.Guardar(tx, empresa, competência)` roda antes de toda escrita de lançamento, com `pg_advisory_xact_lock(dre.ChaveLock(...))` (a única função de chave, usada também por Fechar e Reabrir) e isolamento `READ COMMITTED` explícito. "Fechado" é fechamento ativo da competência.
- Fechar calcula o snapshot dentro da transação e dentro do lock, por `dre.Realizado`, o único caminho que calcula o realizado exibido. Um índice único parcial em `(empresa, competência) WHERE reaberto_em IS NULL` garante um só ativo; `reaberto_*` mudam uma só vez, por gatilho. Versão é `MAX+1` por `(empresa, competência)`.
- Mês fechado é lido só do snapshot (`dre_fechamento_linhas` e `dre_fechamento_contas`, imutáveis por gatilho); mudar Mapa ou estrutura depois não altera o mês fechado.
- `companies.ambiente` (`teste` ou `producao`, padrão `teste`, gatilho que recusa voltar). `auditoria.limpar_dados_teste(empresa)` recusa fora de `teste`. Os gatilhos de imutabilidade chamam `auditoria.imutavel_salvo_em_teste(empresa_id)`, que só libera `DELETE` em `teste` e com a sessão aberta pela função de limpeza. Toda tabela de domínio nova entra em `limpar_dados_teste`.
- A senha do admin é conferida com o mesmo bcrypt do login. O admin de plataforma é `users.role = 'admin'`. `/api/aud/me` informa o ambiente.
- Convenções: migrations `NNN_descricao.sql`, aditivas e idempotentes, que nunca alteram fechamentos; escrita relevante em transação com `writeAuditLogTx`; erros HTTP em texto simples (`409` conflito de período, `403` sem permissão).
- O runner de migrations roda no boot da API e trata como "já aplicada" qualquer erro cujo texto contenha `already exists` ou `duplicate`; uma migration que precisa abortar não pode usar essas palavras na mensagem.

## UX & Interaction Patterns

- Faixa fixa de ambiente em todo o layout enquanto for Teste; na área de plataforma, o ambiente aparece por extenso e, em Produção, não há botão de apagar.
- Virar para Produção pede a senha e a frase digitada "VIRAR PARA PRODUCAO"; a resposta avisa (sem bloquear) as contas ainda vindas de arquivo.
- A tela de fechamento deixa de oferecer escolha de filial; fechamentos por filial já gravados continuam legíveis.

## Cross-Story Dependencies

- 1.1 precede todas as mudanças de código: os testes que ela cria são o contrato que 1.4, 1.5, 1.6, 1.7 e 1.8 mantêm; o teste "aceita em mês fechado" é invertido na 1.6.
- 1.2 cria `imutavel_salvo_em_teste` e `limpar_dados_teste`; 1.7 e 1.8 os usam nos gatilhos e na limpeza. 1.3 usa o marcador e a senha da 1.2.
- 1.4 cria o pacote `dre`, a porta `Fonte` e o escritor único; 1.5 e 1.6 reaproveitam o pacote; 1.6 cria `ChaveLock` e `Guardar`, usados por Fechar (1.5 o prepara) e Reabrir (1.7).
- 1.7 define o que é fechamento ativo; 1.8 lê do snapshot só o ativo.
- 1.9 é independente e pode sair a qualquer momento.
- Fora do épico: o Épico 2 acrescenta `orcado_conta` e fontes; 4.x trocará a checagem de perfil por capacidades; as histórias futuras com tabela de domínio entram em `limpar_dados_teste`.
- Produção só recebe mudança por push, que dispara deploy; migrations rodam no boot da API em produção. O push é decisão humana.
