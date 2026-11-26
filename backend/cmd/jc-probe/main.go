// jc-probe — sonda de leitura do Oracle da JC para o DRE.
//
// Confirma, em ordem: conexão, permissão de leitura nas views do DRE e a
// estrutura (colunas) de cada uma. Não faz SELECT sem filtro: as views da JC têm
// milhões de linhas e um scan completo pode nunca retornar. A amostra de linhas
// é opcional (-amostra), tem timeout e sai só no terminal.
//
// Env (mesmos do FB_FAROL; credencial nunca vai para o repositório):
//
//	JC_ORACLE_HOST       obrigatório
//	JC_ORACLE_PORT       default 1521
//	JC_ORACLE_SERVICE    obrigatório
//	JC_ORACLE_USER       obrigatório
//	JC_ORACLE_PASS       obrigatório
//	JC_ORACLE_DRE_VIEWS  obrigatório (lista separada por vírgula)
//
// Rodar: cd backend && set -a && . ./.env && set +a && go run ./cmd/jc-probe [-amostra]
//
// -realizado sonda CONTR_DRE_REALIZ, VW_DRE_CONTAS e VW_DRE_FILIAL (estrutura,
// períodos e contagens). Com -totais AAAA-MM -filial N, soma o realizado por
// conta daquele mês e filial: sai com VALORES, só no terminal.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	go_ora "github.com/sijms/go-ora/v2"
)

var identificador = regexp.MustCompile(`^[A-Z][A-Z0-9_$#.]{0,127}$`)

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func main() {
	amostra := flag.Bool("amostra", false, "imprime 1 linha de cada view (timeout 20s)")
	resumoF := flag.Bool("resumo", false, "contagens e períodos das views + outras views do mesmo schema (sem valores financeiros)")
	realizadoF := flag.Bool("realizado", false, "sonda a tabela de realizado por conta e as views de contas e filiais")
	totais := flag.String("totais", "", "com -realizado: mês AAAA-MM para somar o realizado por conta (imprime valores)")
	filial := flag.String("filial", "", "com -totais: código da filial")
	flag.Parse()
	var mes time.Time
	if *totais != "" {
		m, err := time.Parse("2006-01", *totais)
		if err != nil || !*realizadoF || strings.TrimSpace(*filial) == "" {
			fail("use -realizado -totais AAAA-MM -filial N")
		}
		mes = m
	}

	var faltam []string
	for _, k := range []string{"JC_ORACLE_HOST", "JC_ORACLE_SERVICE", "JC_ORACLE_USER", "JC_ORACLE_PASS", "JC_ORACLE_DRE_VIEWS"} {
		if env(k, "") == "" {
			faltam = append(faltam, k)
		}
	}
	if len(faltam) > 0 {
		fail("variáveis obrigatórias não configuradas (backend/.env): %s", strings.Join(faltam, ", "))
	}
	user, pass := env("JC_ORACLE_USER", ""), env("JC_ORACLE_PASS", "")
	porta, err := strconv.Atoi(env("JC_ORACLE_PORT", "1521"))
	if err != nil {
		porta = 1521
	}
	dsn := go_ora.BuildUrl(env("JC_ORACLE_HOST", ""), porta,
		env("JC_ORACLE_SERVICE", ""), user, pass, map[string]string{"PREFETCH_ROWS": "2000"})

	db, err := sql.Open("oracle", dsn)
	if err != nil {
		fail("abrir conexão: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fail("conectar (rede/allowlist do IP, service name ou credencial): %v", err)
	}
	var banner string
	if err := db.QueryRowContext(ctx, "SELECT banner FROM v$version WHERE ROWNUM = 1").Scan(&banner); err == nil {
		fmt.Println("conectado:", banner)
	} else {
		fmt.Println("conectado (v$version sem permissão)")
	}

	if *realizadoF {
		realizado(db, mes, strings.TrimSpace(*filial))
		return
	}

	views := strings.Split(env("JC_ORACLE_DRE_VIEWS", ""), ",")
	for _, v := range views {
		sonda(db, strings.ToUpper(strings.TrimSpace(v)), *amostra)
	}
	if *resumoF {
		resumo(db, views)
	}
}

// sonda resolve o objeto (view direta ou sinônimo) e lista as colunas.
func sonda(db *sql.DB, nome string, amostra bool) {
	fmt.Printf("\n== %s ==\n", nome)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	owner, objeto := "", nome
	if i := strings.Index(nome, "."); i > 0 {
		owner, objeto = nome[:i], nome[i+1:]
	}
	// Sinônimo → objeto real (o Farol lê via IAUSER → IAADMIN).
	var dono, alvo sql.NullString
	q := "SELECT table_owner, table_name FROM all_synonyms WHERE synonym_name = :1"
	args := []any{objeto}
	if owner != "" {
		q += " AND owner = :2"
		args = append(args, owner)
	}
	if err := db.QueryRowContext(ctx, q+" FETCH FIRST 1 ROWS ONLY", args...).Scan(&dono, &alvo); err == nil && alvo.Valid {
		fmt.Printf("sinônimo → %s.%s\n", dono.String, alvo.String)
		owner, objeto = dono.String, alvo.String
	}

	q = `SELECT owner, column_id, column_name, data_type, data_length, data_precision, data_scale, nullable
	       FROM all_tab_columns WHERE table_name = :1`
	args = []any{objeto}
	if owner != "" {
		q += " AND owner = :2"
		args = append(args, owner)
	}
	rows, err := db.QueryContext(ctx, q+" ORDER BY owner, column_id", args...)
	if err != nil {
		fmt.Println("ERRO ao ler colunas:", err)
		return
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var o, c, t, nul string
		var id, ln int
		var p, s sql.NullInt64
		if err := rows.Scan(&o, &id, &c, &t, &ln, &p, &s, &nul); err != nil {
			fmt.Println("ERRO scan:", err)
			return
		}
		tipo := t
		if p.Valid {
			tipo = fmt.Sprintf("%s(%d,%d)", t, p.Int64, s.Int64)
		} else if strings.Contains(t, "CHAR") {
			tipo = fmt.Sprintf("%s(%d)", t, ln)
		}
		fmt.Printf("  %-12s %3d  %-32s %-18s null=%s\n", o, id, c, tipo, nul)
		n++
	}
	if n == 0 {
		fmt.Println("nenhuma coluna visível: objeto inexistente ou sem GRANT SELECT para este usuário")
		return
	}
	if !amostra {
		return
	}
	cctx, cc := context.WithTimeout(context.Background(), 20*time.Second)
	defer cc()
	alvoFull := objeto
	if owner != "" {
		alvoFull = owner + "." + objeto
	}
	r, err := db.QueryContext(cctx, "SELECT * FROM "+alvoFull+" FETCH FIRST 1 ROWS ONLY")
	if err != nil {
		fmt.Println("amostra indisponível:", err)
		return
	}
	defer r.Close()
	cols, _ := r.Columns()
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if r.Next() && r.Scan(ptrs...) == nil {
		for i, c := range cols {
			fmt.Printf("  %s = %v\n", c, vals[i])
		}
	}
}

func fail(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "ERRO: "+f+"\n", a...)
	os.Exit(1)
}

// resumo responde o que as colunas não dizem: quais períodos/filiais/contas as
// views cobrem e que outras views o mesmo schema oferece (ex.: realizado por
// conta, plano de contas). Só nomes e contagens — nenhum valor financeiro.
func resumo(db *sql.DB, views []string) {
	fmt.Println("\n== RESUMO ==")
	consulta := func(titulo, q string, args ...any) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rows, err := db.QueryContext(ctx, q, args...)
		if err != nil {
			fmt.Printf("%s: ERRO %v\n", titulo, err)
			return
		}
		defer rows.Close()
		var itens []string
		for rows.Next() {
			var s sql.NullString
			if rows.Scan(&s) == nil {
				itens = append(itens, s.String)
			}
		}
		fmt.Printf("%s (%d): %s\n", titulo, len(itens), strings.Join(itens, ", "))
	}

	owners := map[string]bool{}
	for _, v := range views {
		var o sql.NullString
		_ = db.QueryRow("SELECT table_owner FROM all_synonyms WHERE synonym_name = :1 FETCH FIRST 1 ROWS ONLY",
			strings.ToUpper(strings.TrimSpace(v))).Scan(&o)
		if o.Valid {
			owners[o.String] = true
		}
	}

	consulta("VW_DRE_CONSOLIDADO · MES_ANO distintos",
		"SELECT DISTINCT MES_ANO FROM IACONTR.VW_DRE_CONSOLIDADO ORDER BY 1")
	consulta("VW_DRE_CONSOLIDADO · filiais",
		"SELECT DISTINCT CODFILIAL||'/'||UF FROM IACONTR.VW_DRE_CONSOLIDADO ORDER BY 1")
	consulta("VW_DRE_CONSOLIDADO · linhas",
		"SELECT TO_CHAR(COUNT(*)) FROM IACONTR.VW_DRE_CONSOLIDADO")
	consulta("VW_DRE_ORCAMENTO · anos",
		"SELECT DISTINCT TO_CHAR(ANO) FROM IACONTR.VW_DRE_ORCAMENTO ORDER BY 1")
	consulta("VW_DRE_ORCAMENTO · filiais",
		"SELECT DISTINCT CODFILIAL FROM IACONTR.VW_DRE_ORCAMENTO ORDER BY 1")
	consulta("VW_DRE_ORCAMENTO · contas (quantidade)",
		"SELECT TO_CHAR(COUNT(DISTINCT CODCONTA)) FROM IACONTR.VW_DRE_ORCAMENTO")

	for o := range owners {
		consulta("views visíveis em "+o, "SELECT view_name FROM all_views WHERE owner = :1 ORDER BY 1", o)
		consulta("tabelas visíveis em "+o, "SELECT table_name FROM all_tables WHERE owner = :1 ORDER BY 1", o)
	}
}

// alvo devolve OWNER.OBJETO seguindo o sinônimo; sem sinônimo, assume o schema
// IACONTR, onde estão as demais views do DRE.
func alvo(db *sql.DB, nome string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var dono, obj sql.NullString
	err := db.QueryRowContext(ctx,
		"SELECT table_owner, table_name FROM all_synonyms WHERE synonym_name = :1 FETCH FIRST 1 ROWS ONLY",
		nome).Scan(&dono, &obj)
	if err == nil && dono.Valid && obj.Valid {
		return dono.String + "." + obj.String
	}
	return "IACONTR." + nome
}

// tabela imprime o resultado de uma consulta como linhas separadas por ';'.
func tabela(db *sql.DB, titulo, q string, args ...any) {
	fmt.Printf("\n-- %s\n", titulo)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		fmt.Println("ERRO:", err)
		return
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		fmt.Println("ERRO:", err)
		return
	}
	fmt.Println(strings.Join(cols, ";"))
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	n := 0
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			fmt.Println("ERRO:", err)
			return
		}
		linha := make([]string, len(vals))
		for i, v := range vals {
			linha[i] = v.String
		}
		fmt.Println(strings.Join(linha, ";"))
		n++
	}
	if err := rows.Err(); err != nil {
		fmt.Println("ERRO (resultado incompleto):", err)
		return
	}
	fmt.Printf("(%d linhas)\n", n)
}

// realizado responde o que a integração precisa saber da tabela de realizado por
// conta. Sem mês, sai só estrutura e contagens; com mês, os totais por conta.
func realizado(db *sql.DB, mes time.Time, filial string) {
	nome, data := strings.ToUpper(env("JC_ORACLE_REALIZ_OBJETO", "CONTR_DRE_REALIZ")), strings.ToUpper(env("JC_ORACLE_REALIZ_DATA", "DTLANC"))
	if !identificador.MatchString(nome) || !identificador.MatchString(data) {
		fail("JC_ORACLE_REALIZ_OBJETO/DATA inválidos")
	}
	for _, n := range []string{nome, "VW_DRE_CONTAS", "VW_DRE_FILIAL"} {
		sonda(db, n, false)
	}
	real := alvo(db, nome)

	if !mes.IsZero() {
		ini := mes.Format("2006-01-02")
		onde := " FROM " + real + " WHERE " + data + ` >= TO_DATE(:1,'YYYY-MM-DD')
			AND ` + data + ` < ADD_MONTHS(TO_DATE(:2,'YYYY-MM-DD'),1)
			AND LTRIM(TRIM(CODFILIAL),'0') = LTRIM(:3,'0')`
		tabela(db, "total por conta, "+mes.Format("2006-01")+", filial "+filial+" (VALORES)",
			`SELECT CODCONTA, COUNT(*) LINHAS,
				TO_CHAR(SUM(VALOR),'FM999999999999990.00') TOTAL,
				SUM(CASE WHEN VALOR < 0 THEN 1 ELSE 0 END) NEGATIVAS`+onde+
				" GROUP BY CODCONTA ORDER BY CODCONTA", ini, ini, filial)
		return
	}

	tabela(db, "linhas por mês de "+data,
		"SELECT TO_CHAR("+data+",'YYYY-MM') MES, COUNT(*) LINHAS, COUNT(DISTINCT TRUNC("+data+")) DIAS FROM "+
			real+" GROUP BY TO_CHAR("+data+",'YYYY-MM') ORDER BY 1")
	tabela(db, "linhas e sinal por filial",
		`SELECT CODFILIAL, COUNT(*) LINHAS, SUM(CASE WHEN VALOR > 0 THEN 1 ELSE 0 END) POSITIVAS,
			SUM(CASE WHEN VALOR < 0 THEN 1 ELSE 0 END) NEGATIVAS FROM `+real+` GROUP BY CODFILIAL ORDER BY 1`)
	tabela(db, "quantidade de contas e grupos",
		"SELECT COUNT(DISTINCT CODCONTA) CONTAS, COUNT(DISTINCT GRUPOCONTA) GRUPOS FROM "+real)
	tabela(db, "contas de faturamento, devolução e CMV presentes (1, 2, 3)",
		"SELECT CODCONTA, COUNT(*) LINHAS FROM "+real+" WHERE TRIM(TO_CHAR(CODCONTA)) IN ('1','2','3') GROUP BY CODCONTA ORDER BY 1")
	tabela(db, "VW_DRE_CONTAS por tipo",
		"SELECT TIPO, COUNT(*) CONTAS, COUNT(DISTINCT CODGRUPO) GRUPOS FROM "+alvo(db, "VW_DRE_CONTAS")+" GROUP BY TIPO ORDER BY 1")
	tabela(db, "VW_DRE_FILIAL, códigos",
		"SELECT CODFILIAL FROM "+alvo(db, "VW_DRE_FILIAL")+" ORDER BY 1")
}
