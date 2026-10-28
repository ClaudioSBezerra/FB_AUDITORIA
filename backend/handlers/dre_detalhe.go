package handlers

// dre_detalhe.go — descer do grupo (linha do DRE) à conta e da conta ao
// lançamento, em mês aberto e fechado (história 3.9).

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"fb_auditoria/dre"
)

const (
	detalheLimitePadrao = 200
	detalheLimiteMax    = 1000
)

// DREContasHandler — GET /api/aud/dre/contas?linha=&ano=&mes=&visao=&filial=|agrupamento=|grupo=1
// Contas que compõem a linha (ou, num subtotal, as linhas que o compõem), com o
// valor do DRE: mês fechado do snapshot, mês aberto ao vivo. Só as contas do
// escopo do usuário (4.7): o Gerente recebe as dos seus departamentos; sem
// departamento, lista vazia.
func DREContasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if !exigeLerNoEscopo(w, c, db, "dre_contas") {
			return
		}
		esc, err := escopoDe(r.Context(), db, c)
		if err != nil {
			log.Printf("dre contas: escopo: %v", err)
			http.Error(w, "Erro ao listar as contas", http.StatusInternalServerError)
			return
		}
		p, visao, ok := parsePeriodo(r)
		linha := r.URL.Query().Get("linha")
		if !ok || linha == "" {
			http.Error(w, "linha/ano/mes obrigatórios", http.StatusBadRequest)
			return
		}
		rec, ok := resolverRecorte(w, r, db, c.EmpresaID)
		if !ok {
			return
		}
		erro := func(err error) {
			log.Printf("dre contas: %v", err)
			http.Error(w, "Erro ao listar as contas", http.StatusInternalServerError)
		}
		linhas, err := carregarLinhas(db, c.EmpresaID)
		if err != nil {
			erro(err)
			return
		}
		alvo := alcancarContas(linhas, linha)
		if len(alvo) == 0 {
			http.Error(w, "linha desconhecida", http.StatusNotFound)
			return
		}
		mapa, err := carregarMapa(db, c.EmpresaID)
		if err != nil {
			erro(err)
			return
		}
		leitor := dre.Leitor{Q: db}
		ativos, err := leitor.Ativos(r.Context(), c.EmpresaID, p)
		if err != nil {
			erro(err)
			return
		}
		contas, semContas, err := leitor.ContasDaLinha(r.Context(), c.EmpresaID, rec.Filiais, p, mapa, ativos, alvo, esc)
		if err != nil {
			erro(err)
			return
		}
		desc, err := dre.CarregarDescritorConta(r.Context(), db, c.EmpresaID)
		if err != nil {
			erro(err)
			return
		}
		type conta struct {
			Conta     string  `json:"conta"`
			Descricao string  `json:"descricao"`
			Linha     string  `json:"linha"`
			Valor     float64 `json:"valor"`
		}
		out := make([]conta, 0, len(contas))
		for _, x := range contas {
			out = append(out, conta{x.Conta, desc.De(x.Conta), x.Linha, x.Valor})
		}
		avisos := []string{}
		if len(semContas) > 0 {
			avisos = append(avisos, "Fechamento anterior ao detalhe por conta: sem contas congeladas em "+strings.Join(semContas, ", ")+".")
		}
		writeAuditLog(db, c.EmpresaID, c.UserID, "dre_contas", linha, "visualizar", rec.comVisao(visao))
		writeJSON(w, map[string]any{
			"linha": linha, "ini": p.Ini.Format("2006-01-02"), "fim": p.Fim.Format("2006-01-02"), "visao": visao,
			"contas": out, "avisos": avisos,
		})
	}
}

// DREContaLancamentosHandler — GET /api/aud/dre/contas/lancamentos?conta=&ano=&mes=&visao=&filial=|agrupamento=|grupo=1&limite=&offset=
// Lançamentos da conta (data/competência, documento, histórico como veio, valor,
// filial). O total é o do DRE (snapshot em mês fechado); os lançamentos são lidos
// ao vivo e, se a soma de um mês fechado divergir do congelado, a resposta avisa.
// O Gerente só abre conta dos seus departamentos (4.7); outra conta, ou o
// Gerente sem departamento, recebe 403.
func DREContaLancamentosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if !exigeLerNoEscopo(w, c, db, "dre_lancamentos") {
			return
		}
		esc, err := escopoDe(r.Context(), db, c)
		if err != nil {
			log.Printf("dre lancamentos da conta: escopo: %v", err)
			http.Error(w, "Erro ao consultar lançamentos", http.StatusInternalServerError)
			return
		}
		p, visao, ok := parsePeriodo(r)
		q := r.URL.Query()
		conta := q.Get("conta")
		if !ok || conta == "" {
			http.Error(w, "conta/ano/mes obrigatórios", http.StatusBadRequest)
			return
		}
		limite, offset := detalheLimitePadrao, 0
		if s := q.Get("limite"); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || n > detalheLimiteMax {
				http.Error(w, "limite: de 1 a "+strconv.Itoa(detalheLimiteMax), http.StatusBadRequest)
				return
			}
			limite = n
		}
		if s := q.Get("offset"); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				http.Error(w, "offset inválido", http.StatusBadRequest)
				return
			}
			offset = n
		}
		rec, ok := resolverRecorte(w, r, db, c.EmpresaID)
		if !ok {
			return
		}
		mapa, err := carregarMapa(db, c.EmpresaID)
		if err != nil {
			http.Error(w, "Erro ao carregar mapa", http.StatusInternalServerError)
			return
		}
		leitor := dre.Leitor{Q: db}
		if !esc.Irrestrito {
			dentro, err := leitor.ContaNoEscopo(r.Context(), c.EmpresaID, rec.Filiais, p, mapa, conta, esc)
			if err != nil {
				log.Printf("dre lancamentos da conta: escopo: %v", err)
				http.Error(w, "Erro ao consultar lançamentos", http.StatusInternalServerError)
				return
			}
			if !dentro {
				writeAuditLog(db, c.EmpresaID, c.UserID, "dre_lancamentos", conta, "negado", map[string]any{"perfil": c.Perfil, "motivo": "conta fora do escopo"})
				http.Error(w, "Forbidden: a conta não é de um departamento do seu escopo", http.StatusForbidden)
				return
			}
		}
		det, err := leitor.DetalheConta(r.Context(), c.EmpresaID, rec.Filiais, p, conta, mapa, limite, offset)
		var semMapa *dre.ErrContaSemMapa
		if errors.As(err, &semMapa) {
			http.Error(w, semMapa.Error(), http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("dre lancamentos da conta: %v", err)
			http.Error(w, "Erro ao consultar lançamentos", http.StatusInternalServerError)
			return
		}
		desc, err := dre.CarregarDescritorConta(r.Context(), db, c.EmpresaID)
		if err != nil {
			log.Printf("dre lancamentos da conta: %v", err)
			http.Error(w, "Erro ao consultar lançamentos", http.StatusInternalServerError)
			return
		}
		aviso := ""
		if len(det.Divergencias) > 0 {
			meses := make([]string, 0, len(det.Divergencias))
			for _, d := range det.Divergencias {
				meses = append(meses, d.Competencia)
			}
			aviso = "A soma dos lançamentos não bate com o valor congelado do fechamento em " + strings.Join(meses, ", ") +
				". O total exibido é o do fechamento; os lançamentos listados são os atuais."
		}
		avisos := []string{}
		if len(det.SemSnapshot) > 0 {
			avisos = append(avisos, "Fechamento anterior ao detalhe por conta: sem valor congelado da conta em "+strings.Join(det.SemSnapshot, ", ")+".")
		}
		writeAuditLog(db, c.EmpresaID, c.UserID, "dre_lancamentos", conta, "visualizar", rec.comVisao(visao))
		writeJSON(w, map[string]any{
			"conta": det.Conta, "descricao": desc.De(det.Conta), "linha": det.Linha,
			"ini": p.Ini.Format("2006-01-02"), "fim": p.Fim.Format("2006-01-02"), "visao": visao,
			"total": det.Total, "soma_lancamentos": det.SomaLancamentos, "meses_fechados": det.MesesFechados,
			"divergente": len(det.Divergencias) > 0, "aviso_divergencia": aviso, "divergencias": det.Divergencias,
			"avisos": avisos, "total_lancamentos": det.TotalLancamentos, "limite": limite, "offset": offset,
			"lancamentos": det.Lancamentos,
		})
	}
}

// DRETrimestresHandler — GET /api/aud/dre/trimestres?ano=AAAA&filial=|agrupamento=<codigo>|grupo=1
// DRE por trimestre (T1 a T4 e Ano): para cada linha, realizado, orçado, ano
// anterior e percentual sobre o Faturamento Líquido do próprio trimestre. Cada
// trimestre soma três meses lidos pelo Leitor (mês fechado do snapshot, mês
// aberto ao vivo). É DRE do grupo (faturamento, margens): `gestor` recebe 403 sem
// receber a base (4.7).
func DRETrimestresHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if !exigeLer(w, c, db, "dre_trimestres") {
			return
		}
		ano, err := strconv.Atoi(r.URL.Query().Get("ano"))
		if err != nil || ano < 2000 || ano > 2100 {
			http.Error(w, "ano inválido", http.StatusBadRequest)
			return
		}
		rec, ok := resolverRecorte(w, r, db, c.EmpresaID)
		if !ok {
			return
		}
		erro := func(err error) {
			log.Printf("dre trimestres: %v", err)
			http.Error(w, "Erro ao calcular o DRE por trimestre", http.StatusInternalServerError)
		}
		linhas, err := carregarLinhas(db, c.EmpresaID)
		if err != nil {
			erro(err)
			return
		}
		mapa, err := carregarMapa(db, c.EmpresaID)
		if err != nil {
			erro(err)
			return
		}
		cols, err := (dre.Leitor{Q: db}).LerTrimestres(r.Context(), c.EmpresaID, rec.Filiais, ano, time.Now(), linhas, mapa)
		if err != nil {
			erro(err)
			return
		}
		anoCol := cols[len(cols)-1]

		// Linhas exibidas: a estrutura congelada se o ano todo está fechado; senão a
		// viva, mais as linhas que só existem no congelado de algum trimestre.
		layout := linhas
		if anoCol.Congelada {
			layout = anoCol.Estrutura
		} else {
			vistas := map[string]bool{}
			for _, l := range layout {
				vistas[l.Codigo] = true
			}
			layout = append([]dreLinha(nil), layout...)
			for _, col := range cols {
				if !col.Congelada {
					continue
				}
				for _, l := range col.Estrutura {
					if !vistas[l.Codigo] {
						vistas[l.Codigo] = true
						layout = append(layout, l)
					}
				}
			}
		}

		type valor struct {
			Rotulo      string   `json:"rotulo"`
			Realizado   float64  `json:"realizado"`
			Orcado      float64  `json:"orcado"`
			AnoAnterior float64  `json:"ano_anterior"`
			PctFat      *float64 `json:"pct_faturamento"`
		}
		type coluna struct {
			Rotulo        string   `json:"rotulo"`
			Ini           string   `json:"ini"`
			Fim           string   `json:"fim"`
			Vazio         bool     `json:"vazio"`
			Parcial       bool     `json:"parcial"`
			MesesFechados []string `json:"meses_fechados"`
			PctStatus     string   `json:"pct_status"`
			FatLiqLinha   string   `json:"faturamento_liquido_linha"`
			FatLiq        float64  `json:"faturamento_liquido"`
		}
		type linhaOut struct {
			Codigo    string  `json:"codigo"`
			Descricao string  `json:"descricao"`
			Nivel     int     `json:"nivel"`
			Tipo      string  `json:"tipo"`
			Valores   []valor `json:"valores"`
		}
		colunas := make([]coluna, 0, len(cols))
		pcts := make([]map[string]*float64, len(cols))
		avisoFat := ""
		for i, col := range cols {
			x := coluna{Rotulo: col.Rotulo, Ini: col.Ini.Format("2006-01-02"), Fim: col.Fim.Format("2006-01-02"),
				Vazio: col.Vazio, Parcial: col.Parcial, MesesFechados: col.MesesFechados, PctStatus: dre.PctOK, FatLiqLinha: col.CodBase}
			if !col.Vazio {
				pcts[i], x.PctStatus = dre.PercentuaisSobreFaturamento(col.Estrutura, col.Real, col.CodBase)
				if col.CodBase != "" {
					x.FatLiq = col.Real[col.CodBase]
				}
				if x.PctStatus == dre.PctSemPapel {
					avisoFat = "Nenhuma linha do DRE está marcada como Faturamento Líquido — marque uma em Mapa do DRE para ver os percentuais."
				}
			}
			colunas = append(colunas, x)
		}
		res := make([]linhaOut, 0, len(layout))
		for _, l := range layout {
			lo := linhaOut{Codigo: l.Codigo, Descricao: l.Descricao, Nivel: l.Nivel, Tipo: l.Tipo, Valores: make([]valor, 0, len(cols))}
			for i, col := range cols {
				v := valor{Rotulo: col.Rotulo, Realizado: col.Real[l.Codigo], Orcado: col.Orc[l.Codigo], AnoAnterior: col.AA[l.Codigo]}
				if pcts[i] != nil {
					v.PctFat = pcts[i][l.Codigo]
				}
				lo.Valores = append(lo.Valores, v)
			}
			res = append(res, lo)
		}
		type nm struct {
			Conta string  `json:"conta"`
			Valor float64 `json:"valor"`
		}
		naoMapeado := []nm{}
		for k, v := range anoCol.Sem {
			naoMapeado = append(naoMapeado, nm{k, v})
		}
		sort.Slice(naoMapeado, func(i, j int) bool { return absf(naoMapeado[i].Valor) > absf(naoMapeado[j].Valor) })

		writeAuditLog(db, c.EmpresaID, c.UserID, "dre_trimestres", strconv.Itoa(ano), "visualizar", rec.comVisao("trimestres"))
		writeJSON(w, map[string]any{
			"ano": ano, "colunas": colunas, "linhas": res, "nao_mapeado": naoMapeado, "aviso_faturamento": avisoFat,
		})
	}
}
