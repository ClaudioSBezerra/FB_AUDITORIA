- source_spec: `_bmad-output/implementation-artifacts/spec-4-1-fechar-cadastro-publico.md`
  summary: O `LoginHandler` auto-provisiona ambiente "Ambiente de Testes", grupo e empresa para quem entra com credencial válida e não tem empresa; um usuário criado pelo administrador sem vínculo a ambiente cairia num ambiente compartilhado. A história 4.9 deve vincular o usuário ao ambiente e desligar esse caminho, e o critério ainda não consta nela.
  evidence: `backend/handlers/auth.go`, `LoginHandler` (bloco de auto-provisionamento, `INSERT INTO companies`); o teste `backend/public_routes_test.go` o aceita como exceção documentada porque exige conta existente e senha correta.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-1-fechar-cadastro-publico.md`
  summary: O `AGENTS.md` manda rodar `npm run typecheck` e `npm run lint` antes de commitar, mas o frontend não tem configuração do ESLint (o `npm run lint` falha) e o `typecheck` já acusa um erro em `ResetPassword.tsx` (variável `navigate` sem uso); também diz que não há testes Go, e agora há.
  evidence: na base limpa, `npm run lint` responde "ESLint couldn't find a configuration file" e `npm run typecheck` devolve `ResetPassword.tsx(13,9): error TS6133`; a correção edita o `AGENTS.md`, por isso fica adiada.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-1-fechar-cadastro-publico.md`
  summary: `frontend/src/pages/Register.tsx` ficou sem uso depois de a rota `/register` passar a redirecionar; pode ser apagado quando o cadastro público for descartado de vez.
  evidence: `App.tsx` não importa mais a página e nada mais referencia `/register`; foi mantido de propósito por ser reversível.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-1-testes-fechamento-importacao.md`
  summary: Nenhum teste de handler passa pelo `AuditoriaAuthMiddleware` nem verifica isolamento entre empresas (perfil e área negados, dado da empresa A invisível à B).
  evidence: os testes da 1.1 injetam `AudContext` direto na requisição; a política do projeto exige validação de perfil e áreas no backend.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-2-ambiente-teste-producao-apagar.md`
  summary: Toda empresa existente fica em `teste` após a migration 004; antes de o cliente lançar dado real é preciso virar para Produção (1.3), senão uma senha de admin apaga o dado.
  evidence: `004_ambiente.sql` define o padrão `teste` e a rota `/api/aud/ambiente/limpar` apaga lançamentos, orçado, fechamentos e cargas em `teste`.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-2-ambiente-teste-producao-apagar.md`
  summary: O marcador de sessão `auditoria.limpando_teste` pode ser ligado por qualquer sessão; os gatilhos de imutabilidade das 1.7 e 1.8 não devem tratá-lo como prova de que a limpeza oficial está rodando.
  evidence: `imutavel_salvo_em_teste` só compara o valor da configuração com o id da empresa.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-2-ambiente-teste-producao-apagar.md`
  summary: A rota de limpar dados de teste não limita tentativas de senha nem registra tentativas falhas na trilha.
  evidence: `ambiente.go` só grava trilha no caminho de sucesso.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-3-virar-producao-faixa-ambiente.md`
  summary: O aviso das contas vindas de arquivo (CSV) aparece só na sessão da tela em que a empresa vira Produção; depois não há como consultá-lo.
  evidence: `AmbienteVirarHandler` devolve `contas_de_arquivo` só na resposta da virada.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-3-virar-producao-faixa-ambiente.md`
  summary: A tela reconhece o 409 da virada pelo texto "já está em Produção", porque o helper `api` não expõe o status.
  evidence: `ConfigAmbienteEmpresa.tsx`; mudar a mensagem do backend quebra esse tratamento.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-4-ids-de-area-novos-e-menu.md`
  summary: Durante o deploy, o container antigo pode atender por instantes com os ids de área novos já gravados e responder 403.
  evidence: a migration 005 roda no boot do container novo, e o antigo só valida os ids antigos.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-4-ids-de-area-novos-e-menu.md`
  summary: O frontend não tem testes; redirecionamentos, landing e módulo ativo só se verificam abrindo o app.
  evidence: não há harness de teste em `frontend/`; só `typecheck`.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-4-importacao-csv-ingestor.md`
  summary: Uma falha de banco no meio do INSERT de uma importação (por exemplo, conexão caída) agora responde 400 com a linha, e não 500.
  evidence: `Ingestor.Importar` embrulha qualquer erro de INSERT em `ErroLinhas` para manter o 400 com linha do handler antigo.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-4-importacao-csv-ingestor.md`
  summary: O valor do lançamento passa por `float64` antes do `NUMERIC(18,2)`; o ideal é decimal ou centavos.
  evidence: herdado do handler antigo; `fontes/csv` usa `strconv.ParseFloat`.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-5-fechamento-servico-dre.md`
  summary: A guarda de arquitetura do fechamento só olha `INSERT INTO` em `handlers/`; deveria cobrir UPDATE e DELETE, outros espaçamentos e todos os pacotes fora de `dre`.
  evidence: `backend/arquitetura_test.go`, `TestSoODreEscreveFechamento`; não há violação hoje.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-6-guarda-de-periodo.md`
  summary: Quem espera o lock do período (Fechar ou outra importação) fica bloqueado sem limite enquanto uma importação longa não termina; falta `lock_timeout` com resposta 409 ou 503.
  evidence: `Periodo.Guardar` e `travar` tomam `pg_advisory_xact_lock` e o mantêm até o commit.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-7-fechamento-unico-reabertura.md`
  summary: Excluir um usuário que já fechou ou reabriu um mês falha fora de Teste (o gatilho de imutabilidade bloqueia o `SET NULL`); o `DeleteUserHandler` precisa tratar isso ou a rota deve bloquear.
  evidence: `fechado_por` tem `ON DELETE SET NULL`, que é um UPDATE; `006_fechamento_reabertura.sql` o bloqueia.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-8-mes-fechado-do-snapshot.md`
  summary: O snapshot completo é reconhecido por `orcado IS NOT NULL` nas linhas; trocar por uma coluna explícita em `dre_fechamentos`.
  evidence: `dre/leitura.go`, `Ativos`; fechamento antigo tem orçado nulo, e a regra depende disso.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-8-mes-fechado-do-snapshot.md`
  summary: `Fechar` grava linhas e contas uma a uma por filial; com muitas filiais e contas convém gravar em lote, e os valores passam por `float64` antes do `NUMERIC(18,2)`.
  evidence: `dre/fechamento.go`; o consolidado pode diferir da soma das filiais em centavos.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-8-mes-fechado-do-snapshot.md`
  summary: A chave do snapshot ainda é a filial do ERP; ao entrar o Mapa de filiais (2.14) ela deve virar a filial gerencial.
  evidence: `dre_fechamento_linhas` e `dre_fechamento_contas` guardam `filial` do lançamento.
- source_spec: `_bmad-output/implementation-artifacts/spec-1-9-sem-endereco-do-cliente.md`
  summary: O modo `-resumo` do `jc-probe` mantém nomes de views e do schema do cliente como literais nas consultas; revisar na limpeza de privacidade do Git, junto do `review-rubric.md` e do histórico.
  evidence: `backend/cmd/jc-probe/main.go`; fora do alcance da 1.9 por decisão da spec.
- source_spec: `_bmad-output/implementation-artifacts/spec-2-14-mapa-de-filiais.md`
  summary: `dre.FilialGerencialDe` não tem chamador e faz uma consulta por filial; a história que trocar a chave do snapshot e do realizado para a filial gerencial deve criar uma leitura em lote (`map[filialERP]codigo`).
  evidence: `backend/dre/filiais.go`; nenhum caminho do DRE usa o mapa ainda.
- source_spec: `_bmad-output/implementation-artifacts/spec-2-2-planilha-csv-xlsx.md`
  summary: O formato antigo do CSV não normaliza a filial (`01` fica `01`) e o da planilha remove zeros à esquerda (`01` vira `1`); uma mesma filial pode entrar com chaves diferentes nas duas origens.
  evidence: `backend/dre/fontes/csv/csv.go`, `normFilial`; alinhar quando entrar o resolvedor de filial gerencial (2.14) na ingestão.
- source_spec: `_bmad-output/implementation-artifacts/spec-2-15-agrupamentos-de-filiais.md`
  summary: `?agrupamento=` e `?grupo=1` leem uma filial do ERP por vez (`Leitor.LerFiliais` soma N chamadas de `Ler`); com muitas filiais convém uma leitura em lote (`filial = ANY($)`), junto da troca da chave do snapshot para a filial gerencial.
  evidence: `backend/dre/agrupamentos.go`, `LerFiliais` e `ContasPorDepartamentoFiliais`.
- source_spec: `_bmad-output/implementation-artifacts/spec-2-15-agrupamentos-de-filiais.md`
  summary: A tela do DRE ainda não tem seletor de agrupamento nem de Grupo; os endpoints aceitam os parâmetros. O detalhe por lançamento (`/api/aud/dre/lancamentos`) segue só com `?filial=`.
  evidence: `frontend/src/pages/aud/`; `backend/handlers/dre.go`, `DRELancamentosHandler`.
- source_spec: `_bmad-output/implementation-artifacts/spec-2-16-atualizar-dados.md`
  summary: A fonte Oracle real (realizado e orçado) entra quando a história 2.1 sair: basta registrar `dre.ViaIngestor(ing, fonteOracle)` em `dre.RegistroPadrao` (realizado) e uma `Atualizavel` do orçado com guarda de período; hoje o registro está vazio e o botão responde "nenhuma fonte configurada".
  evidence: `backend/dre/atualizar.go`, `RegistroPadrao`; depende da resposta do cliente sobre o acesso ao Oracle.
- source_spec: `_bmad-output/implementation-artifacts/spec-2-16-atualizar-dados.md`
  summary: Sem trava contra dois pedidos simultâneos de atualização nem timeout por fonte; definir ao entrar a fonte real (por exemplo, advisory lock por empresa e `context.WithTimeout`).
  evidence: `dre.Atualizador.Atualizar` roda as fontes na requisição.
- source_spec: `_bmad-output/implementation-artifacts/spec-3-9-detalhe-ate-o-lancamento.md`
  summary: `lancamentos` não tem data de pagamento nem fornecedor; o detalhe usa competência, `tipo_doc`/`doc_ref` e histórico. Para a fonte Oracle: migration com `data_pagamento DATE` e `fornecedor TEXT` (e, se vier, código/nome separados), preenchidas pelo Ingestor, e as colunas de data e fornecedor na tabela de lançamentos.
  evidence: `backend/migrations/003_dre.sql`; `backend/dre/fonte.go` (`Lancamento`); `backend/dre/detalhe.go`.
- source_spec: `_bmad-output/implementation-artifacts/spec-3-9-detalhe-ate-o-lancamento.md`
  summary: Gestor fica sem as rotas de contas e lançamentos (403, fail closed) e ainda alcança `/api/aud/dre/lancamentos`; o escopo por departamento (4.7) deve fechar o endpoint antigo ou unificá-lo aos novos.
  evidence: `backend/handlers/dre_detalhe.go` (`exigeDetalhe`); `backend/handlers/dre.go` (`DRELancamentosHandler`).
- source_spec: `_bmad-output/implementation-artifacts/spec-3-10-visao-por-trimestre.md`
  summary: O seletor de recorte oferece Grupo, agrupamento e filial do ERP; uma filial gerencial isolada (várias filiais do ERP) não é opção porque `resolverRecorte` não tem parâmetro para ela. Acrescentar `?filial_gerencial=<codigo>` (resolvido pelo mapa da 2.14, mesmas regras de exclusão) e a opção no seletor.
  evidence: `backend/handlers/recorte_filial.go`; `frontend/src/pages/aud/Recorte.tsx`.
- source_spec: `_bmad-output/implementation-artifacts/spec-3-10-visao-por-trimestre.md`
  summary: A visão por trimestre fica negada ao `gestor` (fail closed) e o orçado de trimestre futuro não entra no Ano; liberar o gestor com escopo por departamento é a 4.7, e o orçado do ano inteiro depende de o cliente pedir. A tela do DRE por departamento tem o seletor de recorte, mas a aba Trimestres existe só no DRE.
  evidence: `backend/handlers/dre_detalhe.go` (`DRETrimestresHandler`); `backend/dre/trimestres.go`.
- source_spec: `_bmad-output/implementation-artifacts/spec-3-10-visao-por-trimestre.md`
  summary: O seletor de recorte da 2.15 estava pendente e foi entregue com a 3.10 nas telas do DRE e do DRE por departamento; o detalhe antigo por lançamento (`/api/aud/dre/lancamentos`, usado pelo gestor) continua só com `?filial=`.
  evidence: `frontend/src/pages/aud/DRE.tsx` (`qLegado`).
- source_spec: `_bmad-output/implementation-artifacts/spec-4-6-bloqueio-de-usuario.md`
  summary: A coluna `users.trial_ends_at` ficou no banco (dados existentes) e nenhum código a usa mais; remover numa limpeza futura de schema, junto com `extend_days`/`is_official` ignorados em `PromoteUserHandler`.
  evidence: `backend/migrations/001_baseline_auth.sql`; `backend/handlers/admin.go`.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-6-bloqueio-de-usuario.md`
  summary: A tela de plataforma (`AdminUsers`) só mostra o bloqueio; bloquear e desbloquear são feitos pela tela de usuários da empresa ativa (`/api/aud/usuarios/bloquear`). Um usuário fora da empresa ativa só é bloqueado trocando de empresa.
  evidence: `frontend/src/pages/AdminUsers.tsx`; `backend/handlers/aud_usuarios_bloqueio.go`.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-7-escopo-por-departamento.md`
  summary: Conta cujo departamento mudou no dicionário depois de um fechamento: o Gerente de qualquer um dos dois departamentos abre a conta (todos os meses). Se o cliente achar que dá vazamento, filtrar os lançamentos por mês/departamento do snapshot.
  evidence: `backend/dre/escopo.go` (`ContaNoEscopo`); `backend/handlers/dre_detalhe.go`.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-7-escopo-por-departamento.md`
  summary: O dicionário (GET `/api/aud/dicionario`) mostra ao Gerente o departamento de todas as contas, sem valores. Decidir se o Gerente deve ver só as contas dos seus departamentos. O caminho `qLegado` de `DRE.tsx` (detalhe antigo do Gerente) ficou morto e sai com a 4.8. A pendência da 3.9 sobre o Gerente e `/dre/lancamentos` está resolvida (rota fechada, `ler`).
  evidence: `backend/handlers/dicionario.go`; `frontend/src/pages/aud/DRE.tsx`.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-8-meu-departamento.md`
  summary: "Meu departamento" não tem Orçado por conta (mostra "sem orçado" em tudo): `dre_orcado` é por linha e filial. Para o dado por conta falta a história 2.7 (`orcado_conta` lido pelo Mapa), o orçado por conta no snapshot (`dre_fechamento_contas.orcado`, nulo quando não houver) e preencher `ContaComparativo.Orcado`; a variação sobre o orçado já está pronta na montagem e na tela.
  evidence: `backend/dre/meu_departamento.go` (`orcado_por_conta: false`); `backend/dre/leitura.go`, `OrcadoPorLinha`.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-8-meu-departamento.md`
  summary: O Gerente recebe o percentual sobre o Faturamento Líquido por conta e total sem a base, mas valor / percentual devolve a base. Se o cliente tratar o faturamento como sigiloso para o Gerente, é preciso ocultar ou arredondar o percentual.
  evidence: `backend/handlers/meu_departamento.go`; critério "calculado no servidor, sem devolver a base".
- source_spec: `_bmad-output/implementation-artifacts/spec-4-8-meu-departamento.md`
  summary: A rota `/api/aud/dre/departamentos` ainda atende o Gerente (restrita, sem percentuais) embora o menu e a tela dele sejam só "Meu departamento"; fechar a rota ao Gerente quando nada mais depender dela.
  evidence: `backend/main.go`; `frontend/src/App.tsx` (rota sem `gestor`).
- source_spec: `_bmad-output/implementation-artifacts/spec-4-9-criar-usuario-da-empresa.md`
  summary: O auto-provisionamento do `LoginHandler` (ambiente "Ambiente de Testes") segue ligado: o usuário criado pela 4.9 sempre tem ambiente e empresa e não o alcança, mas quem entra sem vínculo ainda cai nele. Desligar quando se confirmar que o administrador de plataforma não depende dele.
  evidence: `backend/handlers/auth.go`, `LoginHandler`; `backend/public_routes_test.go`.
- source_spec: `_bmad-output/implementation-artifacts/spec-4-9-criar-usuario-da-empresa.md`
  summary: Não há reenvio do convite nem aposentadoria do `CreateUserHandler` (senha informada pelo administrador, também usado para criar administradores e ambientes); o token de primeiro acesso fica em claro em `verification_tokens`, como o de recuperação de senha.
  evidence: `backend/handlers/admin_usuario_empresa.go`; `backend/handlers/admin.go`.
- source_spec: `_bmad-output/implementation-artifacts/spec-2-12-conferencia-entre-origens.md`
  summary: Comparar as linhas da rotina 124 (resumo de faturamento e CMV, tabela 1 da Entrada_Dados) contra as colunas correspondentes da view do Oracle, quando a view existir; o relatório da 2.12 compara só contas de `lancamentos`.
  evidence: a tabela 1 é importada conta a conta, mas o resumo (faturamento líquido, saldo bruto) é subtotal e não é lançamento; falta o nome e as colunas da view.
- source_spec: `_bmad-output/implementation-artifacts/spec-2-12-conferencia-entre-origens.md`
  summary: Mapeamento de colunas da view do Oracle para `lancamentos` (conta, filial, competência, valor) para o adaptador gravar a sua origem e o relatório comparar com a planilha; também o arquivo de correspondência de conta entre planos diferentes, com fator.
  evidence: o nome e as colunas da view de realizado por conta ainda estão em aberto com o dono dos dados (`respostas-cliente-2026-10-28.md`).
- source_spec: `_bmad-output/implementation-artifacts/spec-2-12-conferencia-entre-origens.md`
  summary: Comparar por filial gerencial (filial do ERP somada pelo Mapa de filiais, 2.14), e não só por filial do ERP; hoje a conferência usa a filial como veio, normalizada por zeros à esquerda.
  evidence: `dre.FilialGerencialDe` existe, mas o relatório não a aplica; a coluna `Loja` da Entrada_Dados pode já vir consolidada (pergunta aberta ao Marcos).
- source_spec: `_bmad-output/implementation-artifacts/spec-2-18-comparativo-fechamento.md`
  summary: Comparar as demais visões da planilha de análise: acumulado do ano, filiais lado a lado, centro de custo e por trimestre. Hoje o comparativo cobre um mês e uma filial por vez, por linha e por conta; a aba MENSAL só traz uma visão por arquivo.
  evidence: `backend/dre/comparativo_externo.go` (`Comparar`); a carga guarda só as linhas da filial resolvida de `MENSAL!D1` e as contas por loja.
- source_spec: `_bmad-output/implementation-artifacts/spec-2-18-comparativo-fechamento.md`
  summary: Quando o adaptador Oracle gravar a sua origem em `lancamentos`, o mesmo comparativo por conta passa a compará-la com a planilha (parâmetro `origem`); falta o nome e as colunas da view do Oracle e, para o nível linha, comparar a linha do DRE do Oracle contra a da planilha.
  evidence: `ParamsComparativo.Origem`; o dono dos dados ainda não passou a view (`respostas-cliente-2026-10-28.md`).
- source_spec: `_bmad-output/implementation-artifacts/spec-2-19-salvaguarda-de-periodo.md`
  summary: A simulação antes da gravação só é garantida pela tela; no backend a gravação direta continua possível. Se aparecer integração que também precise da salvaguarda, exigir um token de simulação (hash do arquivo) na gravação.
  evidence: `backend/handlers/dre_import.go` (`?simular=1` é opcional).
- source_spec: `_bmad-output/implementation-artifacts/spec-2-19-salvaguarda-de-periodo.md`
  summary: Os avisos de plausibilidade não comparam valores com o mês anterior (por exemplo, total muito diferente da mesma filial), nem leem AAAA-MM no nome do arquivo.
  evidence: `backend/dre/ingest_resumo.go` (`competenciasDoNome`, `montarResumo`).
- source_spec: `_bmad-output/implementation-artifacts/spec-2-20-carga-de-departamentos.md`
  summary: A carga de departamentos nunca apaga nem renomeia códigos; para retirar os que sobraram é preciso excluir um a um (e só sem contas ligadas). Se pedirem, oferecer a lista dos departamentos fora do arquivo para exclusão em lote.
  evidence: `backend/dre/departamentos_carga.go` (`CarregarDepartamentos` só cria e atualiza).
- source_spec: `_bmad-output/implementation-artifacts/spec-2-21-carga-do-mapa-de-filiais-e-agrupamentos.md`
  summary: As cargas do mapa e dos agrupamentos nunca removem filial gerencial, ligação nem membro; para retirar o que sobrou é preciso usar o cadastro manual. Se pedirem, oferecer o modo "substituir" (com confirmação) por arquivo.
  evidence: `backend/dre/filiais_carga.go` (`CarregarMapaFiliais` e `CarregarAgrupamentos` só criam, ligam e movem).
