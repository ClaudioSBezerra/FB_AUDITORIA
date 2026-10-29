package handlers

// comparativo.go — comparativo de fechamento: planilha de análise do cliente ×
// sistema (história 2.18). A planilha fica gravada em tabelas próprias; a regra
// da comparação é de dre.Comparar.

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/lib/pq"

	"fb_auditoria/dre"
	"fb_auditoria/dre/fontes/planilha"
)

const (
	comparativoLimitePadrao = 500
	comparativoLimiteMax    = 5000
)

// ComparativoCargaHandler — POST /api/aud/comparativo/carga (multipart "arquivo",
// XLSX da planilha de análise; "ano" opcional, padrão MENSAL!C2). Capacidade
// `carregar`. Substitui o escopo do ano; tudo ou nada.
func ComparativoCargaHandler(db *sql.DB) http.HandlerFunc {
	svc := &dre.ComparativoExterno{DB: db}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !c.Pode(CapCarregar) {
			writeAuditLog(db, c.EmpresaID, c.UserID, "comparativo_externo", "", "negado", map[string]any{"perfil": c.Perfil})
			http.Error(w, "Forbidden: perfil insuficiente", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limiteCargaBytes())
		f, hdr, err := r.FormFile("arquivo")
		if err != nil {
			erroUploadCarga(w, err)
			return
		}
		defer f.Close()
		ano := 0
		if s := strings.TrimSpace(r.FormValue("ano")); s != "" {
			if ano, err = strconv.Atoi(s); err != nil || ano < 2000 || ano > 2100 {
				http.Error(w, "ano inválido", http.StatusBadRequest)
				return
			}
		}
		// O XLSX vai para um arquivo temporário e é lido em fluxo: nada de carregar
		// a planilha inteira na memória.
		tmp, err := os.CreateTemp("", "comparativo-*.xlsx")
		if err != nil {
			log.Printf("comparativo carga: %v", err)
			http.Error(w, "Erro ao receber o arquivo", http.StatusInternalServerError)
			return
		}
		defer os.Remove(tmp.Name())
		_, err = io.Copy(tmp, f)
		if cerr := tmp.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			erroUploadCarga(w, err)
			return
		}
		a, err := planilha.Ler(tmp.Name(), ano)
		if erroDicionario(w, err, "ler a planilha") {
			return
		}
		out, err := svc.Carregar(r.Context(), dre.CargaExternaIn{
			EmpresaID: c.EmpresaID, UserID: c.UserID, Arquivo: hdr.Filename, Ano: a.Ano,
			Visao: a.Visao, Filial: a.Filial, Contas: a.Contas, Linhas: a.Linhas, Ignoradas: a.Ignoradas, Avisos: a.Avisos,
		})
		if erroDicionario(w, err, "carregar a planilha") {
			return
		}
		writeJSON(w, out)
	}
}

// ComparativoPeriodosHandler — GET /api/aud/comparativo/periodos: o que há de
// planilha gravada (anos, filiais, meses e últimos arquivos). Sem valores.
func ComparativoPeriodosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !exigeLer(w, c, db, "comparativo_externo") {
			return
		}
		type escopo struct {
			Ano     int      `json:"ano"`
			Filiais []string `json:"filiais"`
			Meses   []string `json:"meses"`
		}
		le := func(tabela string) ([]escopo, error) {
			rows, err := db.QueryContext(r.Context(), `SELECT EXTRACT(YEAR FROM competencia)::int, filial,
					array_agg(DISTINCT to_char(competencia, 'YYYY-MM') ORDER BY to_char(competencia, 'YYYY-MM'))
				FROM auditoria.`+tabela+` WHERE empresa_id = $1::uuid GROUP BY 1, 2 ORDER BY 1 DESC, 2`, c.EmpresaID)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			por := map[int]*escopo{}
			var ordem []int
			for rows.Next() {
				var ano int
				var filial string
				var meses []string
				if err := rows.Scan(&ano, &filial, pq.Array(&meses)); err != nil {
					return nil, err
				}
				e := por[ano]
				if e == nil {
					e = &escopo{Ano: ano, Filiais: []string{}, Meses: []string{}}
					por[ano], ordem = e, append(ordem, ano)
				}
				e.Filiais = append(e.Filiais, filial)
				for _, m := range meses {
					if !contemString(e.Meses, m) {
						e.Meses = append(e.Meses, m)
					}
				}
			}
			out := make([]escopo, 0, len(ordem))
			for _, a := range ordem {
				out = append(out, *por[a])
			}
			return out, rows.Err()
		}
		contas, err := le("comparativo_externo_contas")
		var linhas []escopo
		if err == nil {
			linhas, err = le("comparativo_externo_linhas")
		}
		if err != nil {
			log.Printf("comparativo periodos: %v", err)
			http.Error(w, "Erro ao listar as cargas", http.StatusInternalServerError)
			return
		}
		rows, err := db.QueryContext(r.Context(), `SELECT c.id, c.arquivo, c.ano, c.visao, c.filial,
				to_char(c.criado_em AT TIME ZONE 'America/Sao_Paulo', 'YYYY-MM-DD HH24:MI'), COALESCE(u.full_name, '')
			FROM auditoria.comparativo_externo_cargas c LEFT JOIN users u ON u.id = c.user_id
			WHERE c.empresa_id = $1::uuid ORDER BY c.id DESC LIMIT 10`, c.EmpresaID)
		if err != nil {
			log.Printf("comparativo periodos: %v", err)
			http.Error(w, "Erro ao listar as cargas", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type carga struct {
			ID      int64  `json:"id"`
			Arquivo string `json:"arquivo"`
			Ano     int    `json:"ano"`
			Visao   string `json:"visao"`
			Filial  string `json:"filial"`
			Quando  string `json:"quando"`
			Usuario string `json:"usuario"`
		}
		cargas := []carga{}
		for rows.Next() {
			var x carga
			if err := rows.Scan(&x.ID, &x.Arquivo, &x.Ano, &x.Visao, &x.Filial, &x.Quando, &x.Usuario); err != nil {
				log.Printf("comparativo periodos: %v", err)
				http.Error(w, "Erro ao listar as cargas", http.StatusInternalServerError)
				return
			}
			cargas = append(cargas, x)
		}
		if contas == nil {
			contas = []escopo{}
		}
		if linhas == nil {
			linhas = []escopo{}
		}
		writeJSON(w, map[string]any{"contas": contas, "linhas": linhas, "cargas": cargas})
	}
}

// ComparativoHandler — GET /api/aud/comparativo?ano=&mes=&filial=&nivel=linha|conta
// [&tolerancia=1,00][&origem=][&status=DIVERGE,SO_PLANILHA][&limite=&offset=][&formato=csv].
// Exige `ler` (mostra valor do grupo); o gestor recebe 403, registrado.
func ComparativoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !exigeLer(w, c, db, "comparativo_externo") {
			return
		}
		q := r.URL.Query()
		ano, e1 := strconv.Atoi(q.Get("ano"))
		mes, e2 := strconv.Atoi(q.Get("mes"))
		if e1 != nil || e2 != nil {
			http.Error(w, "ano e mes são obrigatórios", http.StatusBadRequest)
			return
		}
		tol := dre.ToleranciaComparativoPadrao
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
			limite = comparativoLimitePadrao
		}
		if limite > comparativoLimiteMax {
			limite = comparativoLimiteMax
		}
		offset, _ := strconv.Atoi(q.Get("offset"))
		var status []string
		if s := strings.TrimSpace(q.Get("status")); s != "" {
			status = strings.Split(s, ",")
		}
		csvOut := q.Get("formato") == "csv"
		p := dre.ParamsComparativo{
			Ano: ano, Mes: mes, Filial: q.Get("filial"), Nivel: q.Get("nivel"), Tolerancia: tol,
			Origem: q.Get("origem"), Status: status, Limite: limite, Offset: offset,
		}
		res, err := dre.Comparar(r.Context(), db, c.EmpresaID, p)
		if err != nil {
			if errors.Is(err, dre.ErrComparativo) {
				msg := err.Error()
				if i := strings.LastIndex(msg, "\n"); i >= 0 {
					msg = msg[i+1:]
				}
				http.Error(w, msg, http.StatusBadRequest)
				return
			}
			log.Printf("comparativo: %v", err)
			http.Error(w, "Erro ao comparar", http.StatusInternalServerError)
			return
		}
		acao := "visualizar"
		if csvOut {
			acao = "exportar"
		}
		writeAuditLog(db, c.EmpresaID, c.UserID, "comparativo_externo", res.Competencia, acao, map[string]any{
			"competencia": res.Competencia, "nivel": res.Nivel, "filial": res.Filial, "origem": res.Origem, "tolerancia": tol,
			"ok": res.Resumo.OK, "diverge": res.Resumo.Diverge, "so_planilha": res.Resumo.SoPlanilha, "so_sistema": res.Resumo.SoSistema,
		})
		if csvOut {
			escreverComparativoCSV(w, res)
			return
		}
		writeJSON(w, res)
	}
}

func escreverComparativoCSV(w http.ResponseWriter, res *dre.Comparativo) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="comparativo-`+res.Nivel+`-`+res.Competencia+`.csv"`)
	w.Write([]byte("\xef\xbb\xbf"))
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	// Números ficam fora do celulaSegura: negativo começa com '-' e é número.
	f := func(v *float64) string {
		if v == nil {
			return ""
		}
		return numBR(*v)
	}
	cw.Write([]string{"status", "filial", "chave", "descricao", "planilha", "sistema", "diferenca", "diferenca_pct"})
	for _, x := range res.Todas() {
		pct := ""
		if x.DiferencaPct != nil {
			pct = numBR(*x.DiferencaPct)
		}
		cw.Write([]string{celulaSegura(x.Status), celulaSegura(res.Filial), celulaSegura(x.Chave), celulaSegura(x.Descricao),
			f(x.Planilha), f(x.Sistema), numBR(x.Diferenca), pct})
	}
	cw.Flush()
}

func contemString(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
