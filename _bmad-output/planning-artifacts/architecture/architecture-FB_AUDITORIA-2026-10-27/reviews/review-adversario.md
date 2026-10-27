# Revisão adversária — ARCHITECTURE-SPINE (Release A1)

Método: para cada achado, duas unidades (stories/módulos/agentes) que seguem todas as ADs ao pé da letra e mesmo assim divergem. Os ataques usam o código real (`handlers/dre.go`, `dre_import.go`, `auditoria_auth.go`, `003_dre.sql`) e as FRs do PRD. Severidade: ALTA (dado errado ou vazamento em produção), MÉDIA (inconsistência provável, detectável), BAIXA.

Veredito: a spine fecha bem o "quem escreve" (Ingestor, guarda de período), mas deixa abertos a leitura (fechado = snapshot ou ao vivo), a identidade das chaves compartilhadas (conta, desvio, escopo) e o ciclo de vida do fechamento. Há ao menos três buracos ALTA que corrompem número ou escopo sem violar nenhuma AD.

---

## B-1 (ALTA) Fechar ainda lê fora do lock e fora de `dre.Realizado`
**ADs:** AD-6, AD-8, AD-9 (lacuna).
- **Unidade A (Fechamento, FR-9):** porta o handler atual para `dre.Fechamento`. Mantém o fluxo existente: `somaPorLinha` (lê `lancamentos` apenas, sem ajustes) e checagem de "contas sem mapa" **antes** de `db.Begin()`; depois, dentro da tx, toma o advisory lock (AD-8), grava `dre_fechamentos` e `dre_fechamento_linhas`.
- **Unidade B (Ajuste, FR-4 / Ingestor):** chama `Guardar` e grava um ajuste (ou uma reimportação) que confirma entre a leitura de A e o lock de A.
- **Colisão:** AD-8 só diz que Fechar toma "o mesmo lock"; não diz que o cálculo do snapshot acontece **depois** do lock e dentro da tx. O snapshot congelado não contém o ajuste/carga que entrou milissegundos antes do fechamento, e a competência fica fechada com número diferente do que o DRE exibia. Além disso AD-6 exige `dre.Realizado` como caminho único, mas nada obriga Fechar a usá-lo: o snapshot sai sem ajustes e o DRE vivo sai com (dois números para o mesmo mês). A checagem de "conta sem mapeamento" tem a mesma janela (conta nova chega entre a checagem e o lock).
- **Regra proposta (AD-8 reforçada):** `Fechar` = uma tx: (1) lock, (2) validar sem-mapa e sem-departamento, (3) calcular **via `dre.Realizado`** com o mesmo `tx`, (4) gravar. Teste de arquitetura: `dre_fechamento_linhas` só é inserida por função que recebeu o resultado de `dre.Realizado(tx, ...)`.

## B-2 (ALTA) Fechado: o DRE lê o snapshot ou o dado vivo? Mapa/estrutura/departamento não passam pela guarda
**ADs:** AD-8, AD-9, AD-4, AD-10 (lacuna).
- **Unidade A (Visões, FR-7):** consulta o DRE de qualquer mês sempre ao vivo (`lancamentos+ajustes` resolvidos pelo Mapa atual), pois AD-6 diz "Realizado exibido é SUM(...)".
- **Unidade B (Mapa, FR-6 / Estrutura FR-5):** edita `dre_mapa.sinal`, `linha_codigo`, `dre_linhas.soma_de` ou `conta_departamento`; a guarda cobre só `lancamentos`, `orcado_conta`, `ajustes`, `rateio_aplicado`.
- **Colisão:** os dados congelados não mudam, mas o **significado** muda. Um mês fechado em março passa a mostrar em outubro valores diferentes de `dre_fechamento_linhas`, e o e-mail/justificativa/dashboards divergem do snapshot. O requisito "o fechamento não será alterado depois" (PRD §Controller) é violado sem tocar nas tabelas guardadas. O mesmo vale para ano anterior (Realizado de um ano atrás é vivo ou snapshot?).
- **Regra proposta (AD nova, "Leitura de período fechado"):** (a) toda consulta de período com fechamento ativo lê `dre_fechamento_linhas` por linha; o detalhe por conta exige snapshot por conta (ver B-3); (b) o Mapa/estrutura/conta_departamento são **versionados por fechamento** (copiar o mapa efetivo para o fechamento, `dre_fechamento_mapa`) ou a edição que altere a resolução de conta com movimento em mês fechado é recusada pela mesma `Guardar`.

## B-3 (ALTA) Snapshot só por linha do DRE; escopo de departamento não se aplica nele
**ADs:** AD-9, AD-10, AD-13.
- **Unidade A (Fechamento):** conforme AD-9, `dre_fechamento_linhas` guarda `linha_codigo` e `valor`, sem conta e sem departamento.
- **Unidade B (Gerente/E-mail/Justificativa):** aplica `dre.FiltroEscopo` em "todo SQL que devolve valor por conta, ajuste ou justificativa", como manda AD-10.
- **Colisão:** (i) o snapshot não tem conta, então `FiltroEscopo` não tem onde se aplicar; para mês fechado B precisa ler dado vivo (B-2) ou expor a linha inteira (Faturamento Líquido, Margem, Resultado) ao Gerente. (ii) Subtotais por linha e o "percentual sobre Faturamento Líquido" (FR-7) vazam receita do grupo para o Gerente. (iii) O e-mail (AD-13) é montado "no worker", sem `AudContext` (que só existe em request), portanto exige uma segunda implementação de resolução de escopo.
- **Regra proposta:** `dre_fechamento_linhas` ganha granularidade `(linha, conta, filial)`; AD-10 passa a listar as **superfícies** cobertas (consulta, drill-down, snapshot, dashboards, rateio, envios, denominadores) e define o contrato do denominador (Faturamento Líquido é exposto ao Gerente só como percentual pré-calculado ou não é exposto). `FiltroEscopo` recebe `(usuário, tabela)`, não `AudContext`, para o worker usar o mesmo código.

## B-4 (ALTA) `Departamentos()` nil vs vazio: Gerente sem departamento enxerga tudo
**ADs:** AD-10.
- **Unidade A (middleware/perfis):** implementa `Departamentos()` com `var deps []string; for rows.Next() { deps = append(deps, d) }`. Gerente sem linhas em `usuario_departamentos` devolve `nil`.
- **Unidade B (qualquer handler):** segue a regra literal "nil = sem restrição", e aplica o filtro só `if deps != nil`.
- **Colisão:** o caso que a AD quer fail-closed ("Gerente sem departamento não vê nada") sai idêntico ao caso "sem restrição". Fail open por contrato ambíguo, e o ponto é natural em Go.
- **Regra proposta:** substituir o contrato por tipo explícito `Escopo{Irrestrito bool; Deps []string}` (nunca nil como sentinela); `FiltroEscopo` com `Irrestrito=false` e `Deps` vazio gera `FALSE`. Teste: Gerente sem departamento recebe 0 linhas em cada endpoint.

## B-5 (ALTA) Escopo cobre leitura, não escrita (justificativa e ajuste fora do departamento)
**ADs:** AD-7, AD-10, AD-11.
- **Unidade A (Justificativa, FR-12):** `POST /justificativas {alvo_tipo, alvo_chave, texto}` exige a capacidade "justificar" e grava append-only.
- **Unidade B (Visão do Gerente):** filtra a leitura com `FiltroEscopo`.
- **Colisão:** AD-10 diz "todo SQL que **devolve**"; a escrita não é coberta. Um Gerente chama a API com `alvo_chave` de outro departamento (ou inexistente) e grava uma justificativa que muda o status derivado ("justificada") de uma conta que não é dele. Mesma coisa para `alvo_tipo=ajuste` com id de ajuste de outro departamento.
- **Regra proposta:** `Justificar(tx, ctx, alvo)` resolve o alvo para `(conta, departamento)` no servidor, valida que o alvo existe e que o departamento está no escopo, antes do INSERT; `alvo_chave` nunca é aceito como texto livre do cliente (ver B-6).

## B-6 (ALTA) `alvo_chave` do desvio não tem formato; "vigente = maior id" depende dele
**ADs:** AD-7.
- **Unidade A (Justificativa por tela de Gerente):** `alvo_chave = "2026-03|3.1.05|DEP2"` (competência, conta, departamento).
- **Unidade B (Dashboard/Status e E-mail):** monta `"2026-03|3.1.05"` ou inclui filial e base (orçado vs ano anterior) e procura a vigente.
- **Colisão:** chaves diferentes para o mesmo desvio: a tela diz "justificada", o dashboard "pendente"; ou duas justificativas "vigentes" simultâneas. Desvio contra Orçado e contra Ano anterior são desvios distintos (PRD), mas a chave não distingue. Ainda: o valor do desvio muda (reingestão, ajuste, reabertura) e a justificativa antiga continua "vigente" para um número novo. E para `alvo_tipo=ajuste` não há FK (polimórfico), logo órfãos.
- **Regra proposta:** chave canônica única, gerada só por `dre.ChaveDesvio(competencia, conta, filial, base)` e armazenada em colunas (não string), com `valor_na_justificativa`; status "pendente" se o valor atual diferir do justificado além de tolerância. FK parcial para ajuste (`ajuste_id` nullable) em vez de `alvo_chave` para o tipo ajuste.

## B-7 (ALTA) Fechar sobre período já fechado: reabrir a última versão deixa a anterior ativa
**ADs:** AD-9.
- **Unidade A (Fechamento):** o handler atual não impede fechar de novo; `MAX(versao)+1` cria v2 com v1 ainda "sem `reaberto_em`".
- **Unidade B (Reabertura):** marca `reaberto_em` na última versão (v2), como a AD-9 sugere ("última versão sem reaberto_em").
- **Colisão:** v1 nunca foi reaberta, então "última versão sem reaberto_em" passa a ser v1; a competência **continua fechada** depois da reabertura (ou o oposto, conforme a implementação da consulta). Também: reabrir v1 quando v2 ativa.
- **Regra proposta:** Fechar é recusada se já existe fechamento ativo (409); índice único parcial `UNIQUE (empresa_id, competencia, filial) WHERE reaberto_em IS NULL` garante no máximo um ativo. A reabertura vale para o ativo e só ele.

## B-8 (MÉDIA) Consolidado x filial: ambiguidade de `filial = ''`, reabertura sem efeito, filial atrasada
**ADs:** AD-8, AD-9; PRD FR-9/FR-11.
- `''` significa "consolidado" em `dre_fechamentos` e também é o default de `lancamentos.filial` (linha sem filial), `dre_orcado` e do rateio. `Guardar(..., filial='')` é "o consolidado" ou "escrita sem filial"? Uma unidade bloqueia só se há consolidado; outra, bloqueia se há qualquer filial fechada.
- Reabrir a filial A enquanto o consolidado está ativo é um no-op silencioso (escrita continua bloqueada pelo consolidado). A UI mostra "reaberta"; o ajuste dá 409.
- O consolidado fecha **todas** as filiais, inclusive a que ainda não recebeu carga do ERP; a carga tardia dela fica recusada para sempre até reabrir tudo.
- **Regra proposta:** valor reservado separado (`filial IS NULL` para consolidado, ou coluna `escopo ∈ {filial, consolidado}`); tabela de verdade escrita na AD (escrita em filial F bloqueada sse fechamento ativo (F) ou consolidado ativo; escrita sem filial sempre exige consolidado aberto); reabrir filial sob consolidado ativo é recusado com mensagem; consolidado exige todas as filiais esperadas com carga (lista em `config_empresa`).

## B-9 (ALTA) Mesma conta vinda de duas fontes soma em dobro; fonte "dona" da conta
**ADs:** AD-2, AD-3, AD-4.
- **Unidade A (Fonte Oracle):** grava `ERP.VLFATBRUTO` etc. com `fonte='oracle'`.
- **Unidade B (CSV, FR-3):** o analista carrega um arquivo que também contém linhas de faturamento (a mesma conta ou uma conta gerencial mapeada para a mesma linha), `fonte='csv'`.
- **Colisão:** AD-3 isola a reimportação por `(empresa, fonte, competência, filial)`, o que justamente permite coexistir; `SUM(lancamentos)` soma as duas. A mesma ideia vale para a futura fonte real (Pergunta 1 do PRD): as contas `ERP.*` e as contas gerenciais do dicionário mapeadas para a mesma linha. Nenhuma AD declara quem é dono da conta.
- **Regra proposta (AD nova):** cada fonte declara um espaço de contas (`Fonte.Contas() []Prefixo`); o Ingestor recusa lançamento fora do espaço da fonte e recusa registro de duas fontes com prefixos sobrepostos. Mapa em que uma linha agrega contas de fontes diferentes exige confirmação explícita (alerta no fechamento).

## B-10 (MÉDIA/ALTA) Reimportação por escopo: escopo que some e fonte que devolve vazio
**ADs:** AD-3, AD-15.
- **Unidade A (Oracle):** `Ler` devolve 0 linhas por manutenção do ERP / view vazia / filtro errado; Ingestor, "substitui por escopo", apaga o escopo e grava nada.
- **Unidade B (CSV corrigido):** o novo arquivo não tem mais a filial X; as linhas antigas dela permanecem (o escopo é derivado do arquivo).
- **Colisão:** ambos seguem AD-3 e produzem, respectivamente, perda de dado e dado velho. A varredura que "relê o histórico inteiro" é a mais perigosa: um dia ruim apaga o histórico de todas as competências abertas.
- **Regra proposta:** o escopo é enumerado pelo **chamador** (Escopo explícito), não derivado das linhas; resultado vazio para escopo que tinha dados exige confirmação (carga manual) ou limite de queda (`> X%` aborta e alerta); guardar `carga_id` anterior para restauração.

## B-11 (MÉDIA) Reabertura x varredura agendada: ingestão sobrescreve o que o Controller reabriu para ajustar
**ADs:** AD-3, AD-8, AD-15.
- Controller reabre março às 10h00 para lançar um ajuste calculado sobre o valor visto; o agendador das 10h01 reingere março (período agora aberto) e muda o Realizado base. O ajuste de valor absoluto/delta agora corrige outro número. Ao fechar de novo (v2), nada marca que a base mudou desde a v1.
- **Regra proposta:** (a) reabertura pode "suspender ingestão" do escopo (flag em `dre_fechamentos`) até o novo fechamento ou até o Controller liberar; (b) `ajustes` guardam `carga_id` de referência e a visão mostra o ajuste "stale" se a carga mudou; (c) v2 grava diff contra v1.

## B-12 (MÉDIA) Locks: `pg_try_advisory_lock` de sessão em pool, chaves e ordem
**ADs:** AD-8, AD-15.
- `database/sql` usa pool; `pg_try_advisory_lock` (nível sessão, AD-15) adquirido numa conexão e liberado em outra nunca solta (fica preso até a conexão cair) e o agendador deixa de rodar sem erro. A regra deve exigir `db.Conn(ctx)` dedicada ou `pg_try_advisory_xact_lock` dentro da tx.
- A AD-8 diz "o mesmo lock de Fechar e Reabrir", mas não obriga uma única função `dre.lockPeriodo`. Unidade A usa `pg_advisory_xact_lock(hashtext(empresa||comp))` (int4), B usa `(int4,int4)` ou `bigint`: espaços de chave diferentes, sem exclusão mútua. E as chaves de AD-15 e AD-8 podem colidir entre si.
- Uma carga de orçado de 12 competências numa só tx adquire 12 locks em ordem de mapa Go (aleatória); duas cargas ou Fechar+carga em ordem oposta geram deadlock.
- **Regra proposta:** `dre.lockPeriodo` é a única função autorizada a chamar advisory lock (teste de build); namespace por constante; adquirir locks em ordem crescente de competência; sempre `_xact_`, ou conexão dedicada com defer de unlock.

## B-13 (MÉDIA) Orçado: duas verdades (`dre_orcado` legado x `orcado_conta`) e orçado anual vs. guarda
**ADs:** AD-2, AD-5, AD-8.
- `DREOrcadoHandler` atual continua gravando em `dre_orcado` (o teste de AD-2 só proíbe INSERT/DELETE em `lancamentos` e `orcado_conta`); a leitura passa a ser em `orcado_conta`; o orçado digitado pela tela some do DRE ("sem orçado").
- Orçado por linha (legado) não converte em orçado por conta, então a migração prometida não é possível; no período de coexistência, uma unidade lê um e outra o outro.
- A view de orçado traz 12 meses; meses fechados são recusados (AD-8) e os abertos substituídos, produzindo um orçado misto (versões diferentes no mesmo ano) sem rastro.
- **Regra proposta:** `DREOrcadoHandler` removido/desabilitado no A1 (405) e o teste de AD-2 estendido a `dre_orcado`; declarar que orçado é versionado por `carga_id` e que mês fechado mantém o orçado do fechamento (snapshot de orçado em B-2); limite à conversão linha→conta.

## B-14 (MÉDIA) Sinal e domínio do valor do ajuste
**ADs:** AD-4, AD-6.
- Realizado = `SUM(lancamentos)+SUM(ajustes)` na convenção da fonte, depois `* sinal` do Mapa. Unidade A (UI de ajuste) assume que o analista digita na convenção do DRE (positivo = mais receita) e grava assim; unidade B (`dre.Realizado`) aplica o sinal do Mapa de novo. Receita ajustada na direção contrária, em silêncio.
- Ajuste em conta que não tem nenhum lançamento no escopo: uma consulta orientada por `lancamentos` o perde (precisa de UNION/FULL JOIN).
- **Regra proposta:** `ajustes.valor` documentado na convenção da fonte (a UI mostra o efeito no DRE, convertendo no backend); `dre.Realizado` faz a união de contas das duas tabelas; teste com conta de sinal -1 e ajuste.

## B-15 (MÉDIA) Mapa de departamento é uma segunda verdade ao lado do Mapa de linhas
**ADs:** AD-4, AD-10.
- O DRE tem as linhas "Comercial", "Logística", "Administrativo" (departamentos), mapeadas por `dre_mapa`. AD-10 cria `conta_departamento` à parte. Uma conta pode estar na linha "Despesas Logística" e em `conta_departamento` = Comercial; o total do departamento no DRE não bate com o do Gerente. O PRD (FR-6) trata departamento como parte do mesmo Mapa.
- A resolução "mesma por prefixo" ficará implementada duas vezes: `resolverConta` em Go (carrega o mapa em memória) e `FiltroEscopo` em SQL. Em SQL `LIKE` trata `_` como curinga (`ERP.CMV_DEV` casa com `ERP.CMVXDEV`), `%` literal precisa de escape, e o desempate (maior prefixo) é fácil de errar.
- Conta sem departamento: o Gerente fail-closed não a vê, mas ela soma no total; o somatório dos departamentos não fecha com o total.
- **Regra proposta:** departamento é coluna/relação do Mapa (`dre_mapa.departamento_id`) ou `conta_departamento` resolvida por **uma** função (`dre.ResolverConta` devolvendo linha, sinal e departamento) usada pelo Go e materializada em tabela de resolução (`conta_resolvida`) para o SQL; fechamento recusa conta sem departamento (ou bucket "Sem departamento" explícito, visível só ao Controller).

## B-16 (MÉDIA) Caminhos de escrita sem dono: ajuste, justificativa, rateio, aprovação
**ADs:** AD-1, AD-2, AD-6, AD-7, AD-14.
- O leiaute de `dre/` lista Ingestor, Periodo, Fechamento, Escopo, Envio, Rateio; **não há `Ajuste` nem `Justificativa`**. O mapa FR-4/FR-12 diz "dre + handlers". Duas stories fazem o INSERT em lugares diferentes (uma no handler, outra em `dre`); a que fica no handler pode esquecer o `Guardar`. O teste de AD-2 só cobre `lancamentos`/`orcado_conta`.
- O PRD diz "Controller aprova": aprovar/rejeitar justificativa é transição de estado; AD-7 proíbe persistir status, então um dev cria `justificativas.aprovada BOOLEAN` com UPDATE (quebra o append-only) ou nova tabela sem trigger.
- `Guardar` é chamada pelo escritor; nada impede que um novo caminho escreva sem chamá-la.
- **Regra proposta:** `dre.Ajustar`, `dre.Justificar`, `dre.AplicarRateio` são os únicos escritores; teste de build estendido a `ajustes`, `justificativas`, `rateio_aplicado`, `envios`; a escrita só é possível com um tipo `dre.TxPeriodo` obtido de `Periodo.Abrir(...)` (impossível escrever sem passar na guarda); aprovações como linhas append-only (`justificativa_decisoes`).

## B-17 (MÉDIA) Rateio: ordem, invalidação e escopo
**ADs:** AD-8, AD-10, AD-14.
- `rateio_aplicado` é derivado de `lancamentos` (critério "proporcional ao faturamento" lê o faturamento da filial). Se a ingestão muda faturamento depois do rateio e antes do fechamento, o rateio fica velho (a soma deixa de bater, SM-7). Nenhuma AD diz que a reingestão invalida ou recalcula rateio; nem que o rateio **precisa** estar aplicado antes de Fechar; nem se entra no snapshot.
- Sob consolidado fechado o rateio não pode ser aplicado; sob fechamento por filial, rateio de 10 filiais exige que cada filial esteja aberta (rateio parcial).
- `rateio_aplicado` não é "valor por conta, ajuste ou justificativa" no texto de AD-10: o Gerente de um departamento centralizado vê (ou não) o rateio? Duas stories decidem diferente.
- **Regra proposta:** rateio é marcado `stale` por qualquer escrita guardada na competência (mesma tx da guarda); Fechar recusa se há regra de rateio sem aplicação atual, e inclui `rateio_aplicado` no snapshot; AD-10 cobre explicitamente `rateio_aplicado`.

## B-18 (MÉDIA) E-mail (outbox): conteúdo no envio, não no fechamento; múltiplos fechamentos; aviso de ajuste
**ADs:** AD-13, AD-10.
- A linha da outbox é criada no Fechamento, o corpo é montado pelo worker, minutos ou dias depois, com escopo e dados vivos: se `usuario_departamentos` ou o Mapa mudou, ou a competência foi reaberta, o e-mail mostra números que não são os do fechamento enviado (B-2) ou o departamento errado.
- Fechamento por filial gera um fechamento por filial: `UNIQUE (fechamento_id, user_id)` produz N e-mails por Gerente por mês; consolidado gera outro. Reabrir e fechar de novo gera outro conjunto, sem distinguir "retificação".
- FR-4 exige aviso por e-mail ao **ajuste** (para a lista de aviso): não há `fechamento_id`; um dev usa `services.SendEmail` direto (fora da outbox, sem retentativa, sem trilha) e outro cria linha em `envios` com `fechamento_id` nulo (viola o UNIQUE/FK).
- Destinatário removido, sem departamento (fail-closed) ou sem e-mail: envia vazio? pula? Falha silenciosa.
- **Regra proposta:** `envios` guarda `tipo ∈ {fechamento, ajuste, retificacao}`, `chave_evento`, `payload` congelado (corpo montado na tx do evento, via `FiltroEscopo`) ou referência ao snapshot; unicidade por `(tipo, chave_evento, user_id)`; 1 e-mail por Gerente por competência (agrega filiais fechadas); toda mensagem do sistema passa por `envios`.

## B-19 (MÉDIA) Limite de desvio retroativo
**ADs:** AD-7, convenções (`config_empresa`).
- FR-13: alterar o limite não reclassifica competências com fechamento ativo. Com `config_empresa(chave, valor)` único e status "pendente/justificada" derivado ao vivo (AD-7), qualquer mudança reclassifica tudo. Unidade A (dashboard) deriva ao vivo; unidade B (fechamento) quer o congelado.
- **Regra proposta:** o limite efetivo entra no fechamento (coluna ou snapshot de config); `config_empresa` é versionada (`vigente_desde`) e a derivação usa a versão da data de fechamento. O mesmo para a lista de aviso.

## B-20 (MÉDIA) Dois mecanismos de autorização coexistem
**ADs:** AD-11.
- O middleware atual recebe `perfis ...string` por rota (`AuditoriaAuthMiddleware(db, next, area, perfis...)`) e `c.Tem("controladoria")` dentro do handler (como em `DREFechamentoHandler`). AD-11 cria capacidades, mas não manda remover os dois antigos nem testar. Rota nova com lista de perfil literal passa; as matrizes divergem. Os nomes divergem entre o código (`gestor`, `controladoria`, `auditoria`) e a spine/PRD (`gerente`, `Controller`, `analista`): renomeação de `users.perfil` e `CHECK` vira migração destrutiva sem estar na AD-12.
- **Regra proposta:** AD-11 estendida: middleware aceita só capacidade; grep de build proíbe `c.Tem("` e literais de perfil fora de `perfis.go`; tabela de equivalência dos perfis antigos e migração de `users.perfil` junto com `users.areas` na mesma migração.

## B-21 (BAIXA/MÉDIA) Triggers append-only x FK `ON DELETE SET NULL/CASCADE`
**ADs:** AD-6, AD-7, AD-9.
- `dre_fechamentos.fechado_por` e `cargas.user_id` são `ON DELETE SET NULL`; `lancamentos`/novas tabelas usam `ON DELETE CASCADE` de `companies`. Um trigger BEFORE UPDATE/DELETE que bloqueia tudo faz o `DELETE FROM users` falhar (ou o desenvolvedor desativa o trigger, perdendo a imutabilidade). Também a "mudança única" do fechamento por trigger precisa lidar com isso.
- **Regra proposta:** FKs de autoria em tabelas imutáveis usam `ON DELETE RESTRICT` (usuário é desativado, nunca apagado), e o trigger permite apenas a transição única de `reaberto_*`.

## B-22 (BAIXA) Carga CSV x orquestração
**ADs:** AD-3, AD-15.
- Carga CSV usa transação própria (handler/CSV `Fonte`) e não é coberta por `pg_try_advisory_lock` do agendador; CSV e Oracle correm em paralelo (aceitável pela fonte distinta, mas gera B-9). Não há regra para dois uploads CSV simultâneos do mesmo escopo (o lock de período serializa, mas o segundo apaga a carga do primeiro sem aviso).
- **Regra proposta:** carga registra `carga_id` e o segundo upload exibe a carga substituída; opcional `If-Match` com a carga vigente.

---

## Ordem sugerida de fechamento
1. B-4, B-5, B-7 (correções pequenas, contrato claro, risco de segurança/integridade imediato).
2. B-1, B-2, B-3, B-6 (modelo de leitura de período fechado e chaves: exige decisão antes de qualquer story de Visões/Justificativa/E-mail).
3. B-9, B-10, B-12, B-15, B-16 (ingestão e dono único de escrita).
4. B-8, B-11, B-13, B-14, B-17 a B-20 (regras de borda e de configuração).
5. B-21, B-22.

## Pontos que a spine acerta (para não regredir)
Ingestor como único escritor com teste de build; imutabilidade por trigger; idempotência por escopo; `Fonte` como porta; ids de área em migração única; Cloudflare como único caminho público.
