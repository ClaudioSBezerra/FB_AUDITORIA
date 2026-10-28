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
