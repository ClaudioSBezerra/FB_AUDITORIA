# Revisão de versões e de locks — ARCHITECTURE-SPINE (2026-10-27)

Veredito: a seção Stack bate com os arquivos do projeto e a dívida de versões está reconhecida, mas a spine omite a regra de lock de sessão com pool (AD-15) e deixa afirmações sem fonte (go-ora v3, chave dos advisory locks, semântica do outbox).

## 1. Stack contra os arquivos do projeto

| Item da spine | Arquivo | Resultado |
| --- | --- | --- |
| Go 1.26.1 | backend/go.mod `go 1.26.1` | confere |
| golang:1.26.10-alpine | backend/Dockerfile linha 8 | confere |
| lib/pq v1.11.2, jwt/v5 v5.3.1, x/crypto v0.49.0 | backend/go.mod | confere |
| go-ora/v2 v2.9.0 | go.mod `// indirect` | confere. Só `backend/cmd/jc-probe/main.go` importa. A spine já cita o `indirect` na Deferred; o `go mod tidy` o tornaria direto |
| postgres:15-alpine | docker-compose.yml linha 55 | confere |
| React/react-dom ^18.3.1, router-dom ^6.30.6, Vite ^5.2.0, TS ^5.2.2, Tailwind ^3.4.3, react-query ^5.90.20, Recharts ^3.7.0 | frontend/package.json | confere; são faixas semver, não versões instaladas |
| nginx stable-alpine | frontend/Dockerfile linha 15 | confere |
| node:18-alpine (Deferred) | frontend/Dockerfile linha 2 | confere |
| cloudflared:latest, alpine:latest (Deferred) | compose linha 75; backend/Dockerfile linha 21 | confere |

Observações:
- A spine diz "versões lidas dos arquivos", o que é verdade. O `package.json` traz faixas (`^`), então a versão realmente instalada depende do lockfile. Não conferi o lockfile.
- A coluna Version mistura "o que o arquivo declara" com "o que está em produção". Convém dizer isso.

## 2. Estado atual das tecnologias (consulta de 2026-10-27)

| Tecnologia | No projeto | Atual | Suporte | Fonte |
| --- | --- | --- | --- | --- |
| Go | 1.26.1 / imagem 1.26.10 | 1.27.3 e 1.26.10 (ambos de 2026-10-19) | 1.26 e 1.27 suportados; 1.25 EOL em 2026-08-19. 1.26 encerra quando o 1.28 sair (sem data publicada) | https://endoflife.date/api/go.json ; https://go.dev/doc/devel/release |
| PostgreSQL 15 | 15-alpine | 15.19 (2026-08-10) | EOL 2027-11-11 (última versão menor) | https://endoflife.date/api/postgresql.json ; política: https://www.postgresql.org/support/versioning/ |
| Node 18 | node:18-alpine | Node 24 é o LTS ativo (até 2028-04-30). Node 22 vai até 2027-04-30 | Node 18 EOL desde 2025-04-30; Node 20 EOL em 2026-04-30 | https://endoflife.date/nodejs ; https://nodesource.com/blog/nodejs-versions-lts-eol-outdated-release-risks |
| React 18 | ^18.3.1 | 19.3.1 (2026-10-14) | 18.3.1 é de 2024-04-26 e não recebe mais lançamentos (a série 18 está sem manutenção ativa) | https://endoflife.date/api/react.json ; https://registry.npmjs.org/react |
| Vite 5 | ^5.2.0 | 8.3.5 | Suportados: 8.3 (ativo), 7.3 e 8.2 (correções importantes e de segurança), 6.4 e 8.1 (só segurança). Vite 5 não aparece, ou seja, sem suporte | https://vite.dev/releases ; https://registry.npmjs.org/vite |
| Tailwind 3 | ^3.4.3 | 4.3.3 | A dist-tag `v3-lts` aponta para 3.4.19 (a 3.x ainda recebe patches) | https://registry.npmjs.org/-/package/tailwindcss/dist-tags |
| react-router-dom 6 | ^6.30.6 | 7.18.5 | A tag `version-6` está em 6.30.6, exatamente o declarado | https://registry.npmjs.org/-/package/react-router-dom/dist-tags |
| Recharts | ^3.7.0 | 3.10.1 | mesma major, atual | https://registry.npmjs.org/recharts |
| TanStack Query | ^5.90.20 | 5.105.0 | mesma major, atual | https://registry.npmjs.org/@tanstack/react-query |
| TypeScript | ^5.2.2 | 7.0.2 | só atualização | https://registry.npmjs.org/typescript |
| lib/pq | v1.11.2 | v1.12.3 (2026-04-03); v1.12.0 a v1.12.2 saíram em mar-abr/2026 | Lançamentos retomados em 2026 após v1.10.9 (2023). O README diz que as versões de PostgreSQL mantidas são suportadas | https://pkg.go.dev/github.com/lib/pq?tab=versions ; https://github.com/lib/pq |
| go-ora v2 | v2.9.0 | v2.9.0 (2025-06-09) é a última v2 | README: exige Oracle 10.2 ou mais novo | https://pkg.go.dev/github.com/sijms/go-ora/v2?tab=versions ; https://github.com/sijms/go-ora |
| go-ora v3 | não usada | v3.0.1 (2026-07-24); v3.0.0 (2026-07-22) | README: v3 é "a mais recente, com suporte a Oracle 23ai" | https://pkg.go.dev/github.com/sijms/go-ora/v3?tab=versions |
| golang-jwt/jwt v5 | v5.3.1 | v5.3.1 (2026-01-28) | atual | https://pkg.go.dev/github.com/golang-jwt/jwt/v5?tab=versions |

Pontos de fonte fraca: não consegui abrir a página oficial de cronograma do PostgreSQL. A data de 2027-11-11 vem do endoflife.date, que replica a política de 5 anos. A data de fim do Go 1.26 depende do Go 1.28, sem data publicada. A busca na web não retornou nada útil sobre o go-ora; os dados dele vêm do pkg.go.dev e do README.

## 3. Locks e SKIP LOCKED no PostgreSQL 15 com lib/pq

Fontes: https://www.postgresql.org/docs/15/explicit-locking.html ; https://www.postgresql.org/docs/15/functions-admin.html ; https://www.postgresql.org/docs/15/sql-select.html

### AD-8 — `pg_advisory_xact_lock`: funciona como a spine pressupõe, com ressalvas
- `pg_advisory_xact_lock` é de nível de transação, bloqueia se preciso e solta no COMMIT ou ROLLBACK. Isso casa com o pool: o `*sql.Tx` fixa uma conexão até o fim. Sem risco de vazamento. É a escolha certa para a guarda.
- Os locks de sessão e de transação com o mesmo identificador se bloqueiam entre si, então Guardar, Fechar e Reabrir precisam usar o mesmo par de chaves. A spine diz "o mesmo lock" mas não define a derivação da chave. Os dois espaços de chave (um `bigint` ou dois `integer`) não se sobrepõem. Recomendo `pg_advisory_xact_lock(int4, int4)` com namespace fixo por finalidade e `(empresa_id, competência como inteiro)`. Se o `empresa_id` for UUID ou maior que 32 bits, é preciso hash, com risco de colisão (só causa espera extra, não perda de dado).
- A guarda só é segura em READ COMMITTED (o padrão do Postgres, e do lib/pq). Cada comando enxerga o estado já confirmado depois que o lock é obtido. Em REPEATABLE READ ou SERIALIZABLE o snapshot já pode ter sido tomado antes do lock e a checagem de "fechado" leria dado velho. A spine deveria fixar o nível de isolamento.
- Ordem obrigatória dentro da transação: tomar o lock antes de ler o estado do fechamento. Fechar deve tomar o lock antes de gerar o snapshot. A spine diz isso para a guarda, mas não para o Fechar.
- Se a transação de ingestão for longa (Oracle por escopo), o lock por `(empresa, competência)` segura Fechar e demais escritas pelo tempo todo. Isso é aceitável com uma réplica, mas vale deixar a leitura da fonte fora da transação e só abrir a transação para a gravação.

### AD-15 — `pg_try_advisory_lock`: ponto traiçoeiro, não coberto pela spine
- Pela documentação do PG 15, `pg_try_advisory_lock` é de nível de SESSÃO, e não de transação. Ele fica retido até `pg_advisory_unlock` ou até a sessão acabar, e não respeita ROLLBACK.
- Com `database/sql` isso é perigoso. `db.Exec` e `db.QueryRow` pegam qualquer conexão do pool. O lock fica na conexão A, mas o `pg_advisory_unlock` seguinte pode cair na B, retornando `false` com warning, e a conexão A devolvida ao pool segura o lock até ser fechada (`SetConnMaxLifetime` de 15 min no main.go, então até 15 min depois, ou nunca em caso de reuso). O resultado é um agendador travado: nenhuma execução futura obtém o lock, ou o try devolve `false` para sempre.
- Padrão correto: `conn, _ := db.Conn(ctx)`; `conn.QueryRowContext(... pg_try_advisory_lock ...)`; fazer toda a ingestão nessa mesma `*sql.Conn` ou ao menos manter o lock nela; `pg_advisory_unlock` na mesma `*sql.Conn`; `conn.Close()` no `defer`. Alternativa mais simples e sem armadilha de pool: `pg_try_advisory_xact_lock` dentro de uma transação que cobre a execução inteira, mas isso conflita com o tamanho da transação de ingestão. Uma terceira opção é usar o lock de sessão só como "eleição" de uma conexão dedicada e fazer a ingestão em transações próprias por escopo (que continuam passando pela guarda do AD-8).
- Se a conexão dedicada cair no meio da execução (rede, reinício do Postgres), o lock some silenciosamente. A execução precisa checar o erro da conexão e abortar. Com `driver.ErrBadConn` o `database/sql` pode tentar outra conexão sem avisar, e o lock já não é mais do chamador. O agendador deve tratar qualquer erro da `*sql.Conn` como perda do lock.
- Se algum dia houver pgbouncer em modo transaction, o lock de sessão deixa de funcionar. O compose atual liga a API direto ao `db:5432`, sem pooler, então hoje não é problema; vale registrar como restrição.
- A frase da spine "os travamentos por advisory lock já são seguros" (Deferred, várias réplicas) só é verdadeira se o lock de sessão seguir o padrão acima. Como está escrita, é uma afirmação sem confirmação.

### AD-13 — `SELECT ... FOR UPDATE SKIP LOCKED`: funciona no PG 15, com semântica a explicitar
- Existe desde o PG 9.5 e a documentação do 15 o recomenda para tabelas tipo fila com vários consumidores. Funciona com lib/pq sem nada especial.
- O lock de linha só vale dentro da transação. O worker precisa abrir `BeginTx`, rodar o `SELECT ... FOR UPDATE SKIP LOCKED LIMIT n`, enviar, marcar e dar COMMIT. Se o `SELECT` rodar fora de transação (autocommit), o lock é liberado de imediato e a proteção desaparece.
- O envio de SMTP dentro da transação segura a linha e uma conexão do pool durante o envio. Se o processo cair depois do SMTP e antes do COMMIT, o e-mail sai e a linha volta a ficar pendente: duas entregas. O `UNIQUE (fechamento_id, user_id)` só evita duas linhas, não dois envios. A entrega é "ao menos uma vez". A spine promete prevenir "envio duplicado"; deve dizer isso explicitamente, ou usar um estado `enviando` com lease.
- `SKIP LOCKED` não pode ser usado com `WITH TIES`, agregação, `DISTINCT`, `GROUP BY`, `UNION` etc. (docs do PG 15). Uma consulta que junte as tabelas deve usar `FOR UPDATE OF envios`.
- A visão é inconsistente por definição, o que é aceitável para fila.
- Com uma réplica da API o SKIP LOCKED é redundante com o lock de sessão do AD-15, mas é a escolha certa para o dia em que houver duas.

## 4. Itens possivelmente desatualizados ou sem confirmação

| # | Item | Severidade | Ponto afetado |
| --- | --- | --- | --- |
| 1 | `pg_try_advisory_lock` é de sessão; o pool do `database/sql` pode soltar o lock na conexão errada ou deixá-lo vazar. A spine não trata | Alta | AD-15, Deferred (réplicas) |
| 2 | "a v3 não conecta no servidor do cliente" está afirmado sem evidência no repositório nem nas fontes. A v3.0.1 existe desde 2026-07-24, e a v2.9.0 é a última v2 (2025-06-09). O README só diz que a v3 foi feita para 23ai e que a v2 exige Oracle 10.2+. Falta registrar a versão do Oracle do cliente, o erro observado e a data do teste | Média | Stack, AD-15 |
| 3 | Node 18 está EOL desde 2025-04-30 (6 meses). A Deferred diz "nada bloqueia" e isso procede para o A1 (só compila o frontend), mas é o item de maior atraso. Alvo: Node 22 ou 24 | Média | Stack, Deferred |
| 4 | Vite 5 não aparece na lista de suporte (vite.dev/releases). A Deferred trata como dívida leve; é dívida de segurança na cadeia de build | Média | Stack, Deferred |
| 5 | Nível de isolamento e derivação da chave do advisory lock não definidos; a segurança da guarda depende de READ COMMITTED | Média | AD-8, AD-9 |
| 6 | Outbox é "ao menos uma vez": o `UNIQUE` não impede e-mail duplicado após falha entre SMTP e COMMIT | Média | AD-13 |
| 7 | lib/pq v1.11.2 está atrás da v1.12.3. O projeto voltou a lançar versões em 2026, então a noção de "driver em manutenção" não se sustenta com os dados atuais; subir é opcional | Baixa | Stack |
| 8 | Go 1.26 segue suportado, mas o 1.27 está estável (1.27.3). O 1.26 sai de suporte quando o 1.28 for lançado (sem data) | Baixa | Stack |
| 9 | React 18, Tailwind 3 e react-router 6 estão uma major atrás. A tag `version-6` do router está igual ao declarado e a `v3-lts` do Tailwind continua recebendo patches. Sem urgência | Baixa | Stack |
| 10 | PostgreSQL 15 tem suporte até 2027-11-11, ou seja, cerca de 12 meses depois de hoje. A spine não registra essa data; vale incluir na Deferred | Baixa | Stack, Deferred |
| 11 | `cloudflared:latest` e `alpine:latest` sem tag: já listados na Deferred. Reprodutibilidade do build | Baixa | Deferred |
