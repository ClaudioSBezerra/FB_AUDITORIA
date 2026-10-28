package handlers

// meu_departamento.go — "Meu departamento" do Gerente (história 4.8, FR-18/FR-8).
//
// Visão pessoal: só o perfil `gestor` (e não o admin de plataforma) usa a rota;
// os demais perfis têm o DRE por departamento. Decisão da 4.8: controladoria e
// afins recebem 403 (registrado), porque a visão é do escopo de departamentos do
// próprio usuário e não aceita parâmetro para olhar o de outro.
// O percentual sobre o Faturamento Líquido é calculado aqui e a base nunca vai no corpo.

import (
	"database/sql"
	"log"
	"net/http"
	"strings"

	"fb_auditoria/dre"
)

// MeuDepartamentoHandler — GET /api/aud/meu-departamento?ano=&mes=&visao=&filial=|agrupamento=<codigo>|grupo=1&departamento=<codigo>
// Contas dos departamentos do Gerente com Realizado, Orçado, Ano anterior, variação
// e % do Faturamento Líquido. `departamento=` limita a um dos seus; qualquer outro, 403.
func MeuDepartamentoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if c == nil || c.Admin || c.Perfil != "gestor" {
			if c != nil {
				writeAuditLog(db, c.EmpresaID, c.UserID, "meu_departamento", "", "negado", map[string]any{"perfil": c.Perfil})
			}
			http.Error(w, "Forbidden: a visão é do Gerente", http.StatusForbidden)
			return
		}
		erro := func(err error) {
			log.Printf("meu departamento: %v", err)
			http.Error(w, "Erro ao calcular o seu departamento", http.StatusInternalServerError)
		}
		esc, err := escopoDe(r.Context(), db, c)
		if err != nil {
			erro(err)
			return
		}
		pedido := r.URL.Query().Get("departamento")
		if pedido != "" && !esc.Permite(pedido) {
			writeAuditLog(db, c.EmpresaID, c.UserID, "meu_departamento", pedido, "negado", map[string]any{"perfil": c.Perfil, "motivo": "departamento fora do escopo"})
			http.Error(w, "Forbidden: o departamento não é do seu escopo", http.StatusForbidden)
			return
		}
		if pedido != "" {
			esc = dre.EscopoDepto{Deptos: []string{pedido}}
		}
		p, visao, ok := parsePeriodo(r)
		if !ok {
			http.Error(w, "ano/mes inválidos", http.StatusBadRequest)
			return
		}
		rec, ok := resolverRecorte(w, r, db, c.EmpresaID)
		if !ok {
			return
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
		leitor := dre.Leitor{Q: db}
		avisos := []string{}
		status := dre.PctOK
		var (
			base   float64
			visaoM dre.MeuDepartamentoVisao
			comp   dre.ComparativoContas
		)
		visaoM.Departamentos = []dre.MeuDeptoVisao{}
		if !esc.Vazio() {
			b, err := baseFaturamentoLiquido(r.Context(), leitor, c.EmpresaID, rec.Filiais, p, linhas, mapa)
			if err != nil {
				erro(err)
				return
			}
			base, status = b.Valor, b.Status
			if comp, err = leitor.ContasComparativo(r.Context(), c.EmpresaID, rec.Filiais, p, mapa, b.Ativos, esc); err != nil {
				erro(err)
				return
			}
			deps, err := leitor.Departamentos(r.Context(), c.EmpresaID)
			if err != nil {
				erro(err)
				return
			}
			desc, err := dre.CarregarDescritorConta(r.Context(), db, c.EmpresaID)
			if err != nil {
				erro(err)
				return
			}
			visaoM = dre.MontarMeuDepartamento(comp.Contas, deps, desc, base)
		} else {
			visaoM.Total = dre.MeuContaVisao{Descricao: "Total do escopo"}
		}
		if status == dre.PctSemPapel {
			avisos = append(avisos, "Nenhuma linha do DRE está marcada como Faturamento Líquido: os percentuais não são calculados.")
		}
		if len(comp.SemContas) > 0 {
			avisos = append(avisos, "Fechamento anterior ao detalhe por conta: sem contas nem ano anterior em "+strings.Join(comp.SemContas, ", ")+".")
		}
		writeAuditLog(db, c.EmpresaID, c.UserID, "meu_departamento", p.Fim.Format("2006-01"), "visualizar", rec.comVisao(visao))
		// A base (faturamento líquido) não entra no corpo; só a situação do percentual.
		writeJSON(w, map[string]any{
			"ini": p.Ini.Format("2006-01-02"), "fim": p.Fim.Format("2006-01-02"), "visao": visao,
			"pct_status": status, "tem_departamento": len(esc.Deptos) > 0,
			"orcado_por_conta": false, "avisos": avisos,
			"departamentos": visaoM.Departamentos, "total": visaoM.Total,
		})
	}
}
