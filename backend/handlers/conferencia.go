package handlers

// conferencia.go — relatório de conferência entre origens de lançamentos
// (história 2.12). Só leitura; a regra é de dre.Conferir.

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fb_auditoria/dre"
)

const (
	conferenciaLimitePadrao = 200
	conferenciaLimiteMax    = 1000
)

// ConferenciaOrigensHandler — GET /api/aud/conferencia/origens[?competencia=AAAA-MM]
// Origens de lançamentos da empresa, com a quantidade de linhas e o intervalo de
// competências; com `competencia`, também as linhas daquele mês.
func ConferenciaOrigensHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !exigeLer(w, c, db, "conferencia") {
			return
		}
		comp := time.Time{}
		if s := r.URL.Query().Get("competencia"); s != "" {
			t, err := time.Parse("2006-01", s)
			if err != nil {
				http.Error(w, "competencia inválida (esperado AAAA-MM)", http.StatusBadRequest)
				return
			}
			comp = t
		}
		rows, err := db.QueryContext(r.Context(), `SELECT origem, COUNT(*),
				to_char(MIN(competencia), 'YYYY-MM'), to_char(MAX(competencia), 'YYYY-MM'),
				COUNT(*) FILTER (WHERE $2::date IS NOT NULL AND competencia = $2::date)
			FROM auditoria.lancamentos WHERE empresa_id = $1::uuid GROUP BY origem ORDER BY origem`,
			c.EmpresaID, nullData(comp))
		if err != nil {
			log.Printf("conferencia origens: %v", err)
			http.Error(w, "Erro ao listar as origens", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type origem struct {
			Origem            string `json:"origem"`
			Linhas            int    `json:"linhas"`
			PrimeiraCompet    string `json:"primeira_competencia"`
			UltimaCompet      string `json:"ultima_competencia"`
			LinhasNaCompetenc int    `json:"linhas_na_competencia"`
		}
		out := []origem{}
		for rows.Next() {
			var o origem
			if err := rows.Scan(&o.Origem, &o.Linhas, &o.PrimeiraCompet, &o.UltimaCompet, &o.LinhasNaCompetenc); err != nil {
				log.Printf("conferencia origens: %v", err)
				http.Error(w, "Erro ao listar as origens", http.StatusInternalServerError)
				return
			}
			out = append(out, o)
		}
		if err := rows.Err(); err != nil {
			log.Printf("conferencia origens: %v", err)
			http.Error(w, "Erro ao listar as origens", http.StatusInternalServerError)
			return
		}
		writeJSON(w, out)
	}
}

func nullData(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format("2006-01-02")
}

// ConferenciaHandler — GET /api/aud/conferencia?competencia=AAAA-MM&origem_a=&origem_b=
// [&filial=][&tolerancia=0,01][&limite=&offset=][&formato=csv]
// Compara por (filial, conta) a soma bruta de cada origem. Exige `ler` (o relatório
// traz valor do grupo); o gestor recebe 403.
func ConferenciaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !exigeLer(w, c, db, "conferencia") {
			return
		}
		q := r.URL.Query()
		comp, err := time.Parse("2006-01", q.Get("competencia"))
		if err != nil {
			http.Error(w, "competencia obrigatória (AAAA-MM)", http.StatusBadRequest)
			return
		}
		tol := dre.ToleranciaPadrao
		if s := strings.TrimSpace(q.Get("tolerancia")); s != "" {
			v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
			if err != nil || v < 0 {
				http.Error(w, "tolerancia inválida", http.StatusBadRequest)
				return
			}
			tol = v
		}
		limite, _ := strconv.Atoi(q.Get("limite"))
		if limite <= 0 {
			limite = conferenciaLimitePadrao
		}
		if limite > conferenciaLimiteMax {
			limite = conferenciaLimiteMax
		}
		offset, _ := strconv.Atoi(q.Get("offset"))
		csvOut := q.Get("formato") == "csv"
		p := dre.ParamsConferencia{
			Competencia: comp, OrigemA: q.Get("origem_a"), OrigemB: q.Get("origem_b"),
			Filial: q.Get("filial"), Tolerancia: tol, Limite: limite, Offset: offset,
		}
		res, err := dre.Conferir(r.Context(), db, c.EmpresaID, p)
		if err != nil {
			if errors.Is(err, dre.ErrConferencia) {
				msg := err.Error()
				if i := strings.LastIndex(msg, "\n"); i >= 0 {
					msg = msg[i+1:]
				}
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
			log.Printf("conferencia: %v", err)
			http.Error(w, "Erro ao conferir", http.StatusInternalServerError)
			return
		}
		acao := "visualizar"
		if csvOut {
			acao = "exportar"
		}
		writeAuditLog(db, c.EmpresaID, c.UserID, "conferencia", res.Competencia, acao, map[string]any{
			"competencia": res.Competencia, "origem_a": p.OrigemA, "origem_b": p.OrigemB, "filial": res.Filial,
			"tolerancia": tol, "total_diferencas": res.TotalDiferencas, "so_em_a": res.TotalSoEmA, "so_em_b": res.TotalSoEmB,
		})
		if csvOut {
			escreverConferenciaCSV(w, res)
			return
		}
		writeJSON(w, res)
	}
}

// celulaSegura impede injeção de fórmula ao abrir o CSV numa planilha: texto que
// começa com = + - @ (ou tabulação/retorno) ganha um apóstrofo na frente.
func celulaSegura(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func numBR(v float64) string { return strings.ReplaceAll(strconv.FormatFloat(v, 'f', 2, 64), ".", ",") }

func escreverConferenciaCSV(w http.ResponseWriter, res *dre.Conferencia) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="conferencia-`+res.Competencia+`.csv"`)
	w.Write([]byte("\xef\xbb\xbf"))
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	linha := func(c ...string) {
		for i := range c {
			c[i] = celulaSegura(c[i])
		}
		cw.Write(c)
	}
	// Números ficam fora do celulaSegura: negativo começa com '-' e é número.
	num := func(tipo, filial, conta, desc string, a, b, d *float64) {
		f := func(v *float64) string {
			if v == nil {
				return ""
			}
			return numBR(*v)
		}
		cw.Write([]string{celulaSegura(tipo), celulaSegura(filial), celulaSegura(conta), celulaSegura(desc), f(a), f(b), f(d)})
	}
	linha("tipo", "filial", "conta", "descricao", "origem_a: "+res.A.Origem, "origem_b: "+res.B.Origem, "diferenca")
	dif, soA, soB := res.Todas()
	for _, x := range dif {
		a, b, d := x.ValorA, x.ValorB, x.Diferenca
		num("diferenca", x.Filial, x.Conta, x.Descricao, &a, &b, &d)
	}
	for _, x := range soA {
		v := x.Valor
		num("so_em_a", x.Filial, x.Conta, x.Descricao, &v, nil, nil)
	}
	for _, x := range soB {
		v := x.Valor
		num("so_em_b", x.Filial, x.Conta, x.Descricao, nil, &v, nil)
	}
	for _, x := range res.ResumoLinhas {
		a, b, d := x.ValorA, x.ValorB, x.Diferenca
		num("linha_dre", "", x.Codigo, x.Descricao, &a, &b, &d)
	}
	cw.Flush()
}
