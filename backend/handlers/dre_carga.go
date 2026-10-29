package handlers

// dre_carga.go — história 3.2: carga da estrutura e do Mapa do DRE por arquivo
// e lista das contas sem Mapa (FR-5, FR-6, FR-9).

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"log"
	"net/http"
	"strconv"

	"fb_auditoria/dre"
	csvfonte "fb_auditoria/dre/fontes/csv"
)

// erroCargaDRE mapeia o erro das cargas para HTTP (409 quando a estrutura
// deixaria linhas com contas no Mapa); o resto segue o do dicionário.
func erroCargaDRE(w http.ResponseWriter, err error, op string) bool {
	var emUso *dre.ErrEstruturaEmUso
	if errors.As(err, &emUso) {
		http.Error(w, err.Error(), http.StatusConflict)
		return true
	}
	return erroDicionario(w, err, op)
}

// DREEstruturaCargaHandler — POST /api/aud/dre/estrutura/carga (multipart
// "arquivo", CSV ou XLSX). Tudo ou nada; capacidade editar_estrutura.
func DREEstruturaCargaHandler(db *sql.DB) http.HandlerFunc {
	svc := &dre.EstruturaCarga{DB: db}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !c.Pode(CapEditarEstrutura) {
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
		linhas, probs, err := csvfonte.LerEstrutura(f)
		if erroCargaDRE(w, err, "ler a estrutura") {
			return
		}
		out, err := svc.CarregarEstrutura(r.Context(), dre.CargaEstruturaIn{
			EmpresaID: c.EmpresaID, UserID: c.UserID, Arquivo: hdr.Filename, Linhas: linhas, Problemas: probs,
		})
		if erroCargaDRE(w, err, "carregar a estrutura") {
			return
		}
		writeJSON(w, out)
	}
}

// DREMapaCargaHandler — POST /api/aud/dre/mapa/carga (multipart "arquivo", CSV
// ou XLSX). Substitui o Mapa inteiro; tudo ou nada; capacidade editar_mapa.
func DREMapaCargaHandler(db *sql.DB) http.HandlerFunc {
	svc := &dre.EstruturaCarga{DB: db}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !c.Pode(CapEditarMapa) {
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
		itens, probs, err := csvfonte.LerMapa(f)
		if erroCargaDRE(w, err, "ler o Mapa") {
			return
		}
		out, err := svc.CarregarMapaArquivo(r.Context(), dre.CargaMapaIn{
			EmpresaID: c.EmpresaID, UserID: c.UserID, Arquivo: hdr.Filename, Itens: itens, Problemas: probs,
		})
		if erroCargaDRE(w, err, "carregar o Mapa") {
			return
		}
		writeJSON(w, out)
	}
}

const (
	semMapaLimitePadrao = 200
	semMapaLimiteMax    = 5000
)

// DREMapaSemMapaHandler — GET /api/aud/dre/mapa/sem-mapa?ano=&mes=&visao=&limite=&formato=csv:
// contas com lançamento no período sem linha no Mapa, com a soma por conta
// (meses ao vivo). Mostra valor: exige `ler` (o Gerente recebe 403, registrado).
func DREMapaSemMapaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !exigeLer(w, c, db, "dre_sem_mapa") {
			return
		}
		p, visao, ok := parsePeriodo(r)
		if !ok {
			http.Error(w, "ano/mes inválidos", http.StatusBadRequest)
			return
		}
		mapa, err := carregarMapa(db, c.EmpresaID)
		if err != nil {
			http.Error(w, "Erro ao carregar mapa", http.StatusInternalServerError)
			return
		}
		contas, err := (dre.Leitor{Q: db}).ContasSemMapa(r.Context(), c.EmpresaID, p, mapa)
		if err != nil {
			log.Printf("dre sem mapa: %v", err)
			http.Error(w, "Erro ao listar as contas sem Mapa", http.StatusInternalServerError)
			return
		}
		var soma float64
		for _, x := range contas {
			soma += x.Valor
		}
		if r.URL.Query().Get("formato") == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="contas-sem-mapa-`+p.Fim.Format("2006-01")+`.csv"`)
			w.Write([]byte("\xef\xbb\xbf"))
			cw := csv.NewWriter(w)
			cw.Comma = ';'
			cw.Write([]string{"conta", "descricao", "valor"})
			for _, x := range contas {
				cw.Write([]string{celulaSegura(x.Conta), celulaSegura(x.Descricao), numBR(x.Valor)})
			}
			cw.Flush()
			return
		}
		limite, _ := strconv.Atoi(r.URL.Query().Get("limite"))
		if limite <= 0 {
			limite = semMapaLimitePadrao
		}
		if limite > semMapaLimiteMax {
			limite = semMapaLimiteMax
		}
		total := len(contas)
		if total > limite {
			contas = contas[:limite]
		}
		writeJSON(w, map[string]any{
			"ini": p.Ini.Format("2006-01-02"), "fim": p.Fim.Format("2006-01-02"), "visao": visao,
			"total": total, "soma": soma, "truncado": total > len(contas), "itens": contas,
		})
	}
}
