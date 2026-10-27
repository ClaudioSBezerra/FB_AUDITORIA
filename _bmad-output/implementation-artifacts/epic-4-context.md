# Epic 4 Context: Acesso seguro por perfil e departamento

<!-- Compiled from planning artifacts. Edit freely. Regenerate with compile-epic-context if planning docs change. -->

## Goal

Cada pessoa vê e faz só o que cabe ao seu papel, o Gerente enxerga apenas o seu departamento, e o cadastro público deixa de existir. O site já é público, então fechar o cadastro sai antes de tudo; o acesso passa a terminar por bloqueio explícito, nunca por prazo. A confidencialidade por departamento é garantida pelo servidor, não pela tela.

## Stories

- Story 4.1: Fechar o cadastro público (a primeira a sair)
- Story 4.2: Perfil `analista` e matriz de capacidades
- Story 4.3: Migrar as rotas para capacidades
- Story 4.4: Ids de área novos e menu
- Story 4.5: Usuários com perfil e departamentos
- Story 4.6: Bloqueio de usuário no lugar da expiração
- Story 4.7: Escopo por departamento que nega por padrão
- Story 4.8: "Meu departamento" para o Gerente
- Story 4.9: Criar usuário da empresa

## Requirements & Constraints

- Acesso controlado por perfil e, para o Gerente, por departamento, validado no backend em toda rota; a interface só reflete.
- Cadastro de usuários restrito ao administrador; cadastro público desativado por padrão (variável `ALLOW_SELF_REGISTER` com padrão `false` no `.env.example` e no compose).
- Bloquear e desbloquear usuário exigem motivo e vão à trilha; nenhum usuário expira por tempo; ninguém bloqueia a si mesmo; o último administrador de plataforma ativo não pode ser bloqueado.
- O servidor nunca devolve a um Gerente dado fora do seu departamento, incluindo subtotais e percentuais; Gerente sem departamento recebe vazio, nunca tudo.
- Alterações de perfil, departamento, bloqueio e criação de usuário gravam na trilha imutável.
- Fora do A1: perfil `ti` (criar e bloquear usuários); até lá, gerir usuários é do Controller e do admin de plataforma.

## Technical Decisions

- Autorização por capacidade, mecanismo único: `perfis.go` define capacidades e a matriz; o middleware recebe uma capacidade. Durante a transição aceita também a lista de perfis atual; a assinatura antiga é removida ao migrar a última rota, e um teste falha se rota nova usar lista de perfis.
- Matriz: `analista` (novo no `CHECK` de `users.perfil`) lê, carrega, ajusta, edita mapa/estrutura/dicionário, configura rateio; `controladoria` soma fechar, reabrir, aplicar rateio, definir limite, gerir usuários; `gestor` lê e justifica só no seu escopo; `diretoria` e `auditoria` só leitura. Admin de plataforma (`users.role = 'admin'`) mantém acesso total.
- Escopo: `struct {Irrestrito bool; Deptos []string}` cujo valor zero não dá acesso; todo SQL que devolve valor por conta aplica `dre.FiltroEscopo`; conta sem departamento só aparece a perfis irrestritos; casamento por prefixo em Go, nunca `LIKE`; percentual sobre Faturamento Líquido calculado no servidor sem devolver a base; mês fechado filtra o snapshot por contas. A rota do DRE do grupo é negada ao Gerente.
- Tabelas: `departamentos`, `conta_departamento`, `usuario_departamentos (user_id, departamento_id)` (configuração, preservada por `limpar_dados_teste`); só `gestor` pode ter departamentos ligados.
- Áreas: migração única com `UPDATE ... CASE` em `users.areas` (`estrategico`→`empresa`, `administrativo`→`configuracoes`, `operacional`→`operacao`), idempotente, atualizando o `DEFAULT`; ids antigos recusados e nunca reaproveitados; backend, rotas e `navigation.ts` saem no mesmo release. Rótulos: Administrativo (id `empresa`), Configurações, Comercial, Operação; administração da plataforma passa a "Plataforma".
- Bloqueio: `users.bloqueado_em`, `bloqueado_por`, `motivo_bloqueio`, verificados no login, na renovação de sessão e no middleware (vale na requisição seguinte). Remover a verificação por `trial_ends_at` do login (`auth.go`) e do middleware (`auditoria_auth.go`), a extensão de teste do `PromoteUserHandler`, o prazo de 14 dias do `CreateUserHandler`, a coluna Trial de `AdminUsers.tsx` e o "Licença vence" do `AppRail.tsx`.

## UX & Interaction Patterns

- Tela de usuários: escolher perfil, ligar departamentos ao `gestor`, coluna "Bloqueado" no lugar de "Trial"; acessível só a quem tem "gerir usuários".
- "Meu departamento": aba visível só ao `gestor`, com Realizado, Orçado, Ano anterior, totais e variação das contas dos seus departamentos; nenhum outro departamento visível, nem por URL direta.
- Rota `/register` redireciona para o login.
- Criação de usuário: link para o usuário definir a própria senha; sem SMTP, a tela mostra o link para o administrador repassar.

## Cross-Story Dependencies

- 4.1 não depende de nada e pode ser entregue sozinha, antes de qualquer outra história, inclusive do épico 1.
- 4.2 precede 4.3; 4.3 usa os testes de caracterização da 1.1.
- 4.4 (menu e áreas) e 4.5 (usuários e departamentos) alimentam 4.7 e 4.8; 4.7 depende dos departamentos e do mapa de contas do épico 3 (`conta_departamento`, `dre.ResolverConta`).
- 4.8 depende de 4.7 e da comparação com orçado e ano anterior do épico 3.
- 4.6 e 4.9 removem o prazo de teste de usuário; o épico 5 (justificativas, e-mail por departamento) e o 6 dependem do escopo da 4.7.
- O perfil `gestor` não vê dado de departamento até este épico.
