package handlers

// dre_detalhe.go — descer do grupo (linha do DRE) à conta e da conta ao
// lançamento, em mês aberto e fechado (história 3.9).

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"fb_auditoria/dre"
)

const (
	detalheLimitePadrao = 200
	detalheLimiteMax    = 1000
)

// exigeDetalhe aplica o mesmo fail-closed da 3.5: `gestor` (sem escopo por
// departamento até a 4.7) não desce às contas e lançamentos. 403 registrado.
func exigeDetalhe(w http.ResponseWriter, c *AudContext, db *sql.DB, entidade string) bool {
	if c.Tem("controladoria", "auditoria", "diretoria") {
		return true
	}
	writeAuditLog(db, c.EmpresaID, c.UserID, entidade, "", "negado", map[string]any{"perfil": c.Perfil})
	http.Error(w, "Forbidden: o perfil não tem acesso ao detalhe por conta e lançamento", http.StatusForbidden)
	return false
}

// DREContasHandler — GET /api/aud/dre/contas?linha=&ano=&mes=&visao=&filial=|agrupamento=|grupo=1
// Contas que compõem a linha (ou, num subtotal, as linhas que o compõem), com o
// valor do DRE: mês fechado do snapshot, mês aberto ao vivo.
func DREContasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if !exigeDetalhe(w, c, db, "dre_contas") {
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
		contas, semContas, err := leitor.ContasDaLinha(r.Context(), c.EmpresaID, rec.Filiais, p, mapa, ativos, alvo)
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
func DREContaLancamentosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if !exigeDetalhe(w, c, db, "dre_lancamentos") {
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
		det, err := (dre.Leitor{Q: db}).DetalheConta(r.Context(), c.EmpresaID, rec.Filiais, p, conta, mapa, limite, offset)
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
