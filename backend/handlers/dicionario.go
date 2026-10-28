package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"fb_auditoria/dre"
	csvfonte "fb_auditoria/dre/fontes/csv"
)

// DicionarioHandler — GET /api/aud/dicionario?busca=&pagina=&limite=: contas do
// dicionário com departamento e origem do vínculo, e as contas sem departamento
// à parte. Qualquer perfil da área.
func DicionarioHandler(db *sql.DB) http.HandlerFunc {
	svc := &dre.Dicionario{DB: db}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		q := r.URL.Query()
		pagina, _ := strconv.Atoi(q.Get("pagina"))
		limite, _ := strconv.Atoi(q.Get("limite"))
		l, err := svc.ListarContas(r.Context(), c.EmpresaID, q.Get("busca"), pagina, limite)
		if err != nil {
			log.Printf("dicionario listar: %v", err)
			http.Error(w, "Erro ao listar o dicionário", http.StatusInternalServerError)
			return
		}
		writeJSON(w, l)
	}
}

// DicionarioCargaHandler — POST /api/aud/dicionario/carga (multipart "arquivo",
// CSV ou XLSX). Tudo ou nada; só controladoria.
func DicionarioCargaHandler(db *sql.DB) http.HandlerFunc {
	svc := &dre.Dicionario{DB: db}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !c.Tem("controladoria") {
			http.Error(w, "Forbidden: perfil insuficiente", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
		f, hdr, err := r.FormFile("arquivo")
		if err != nil {
			http.Error(w, "arquivo obrigatório", http.StatusBadRequest)
			return
		}
		defer f.Close()
		linhas, probs, err := csvfonte.LerDicionario(f)
		if erroDicionario(w, err, "ler o dicionário") {
			return
		}
		out, err := svc.Carregar(r.Context(), dre.CargaDicionarioIn{
			EmpresaID: c.EmpresaID, UserID: c.UserID, Arquivo: hdr.Filename, Linhas: linhas, Problemas: probs,
		})
		if erroDicionario(w, err, "carregar o dicionário") {
			return
		}
		writeJSON(w, out)
	}
}

// DicionarioContaHandler — PUT /api/aud/dicionario/conta {conta, departamento_id}:
// corrige o departamento de UMA conta (origem manual). departamento_id vazio
// volta a derivar do grupo. Só controladoria.
func DicionarioContaHandler(db *sql.DB) http.HandlerFunc {
	svc := &dre.Dicionario{DB: db}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !c.Tem("controladoria") {
			http.Error(w, "Forbidden: perfil insuficiente", http.StatusForbidden)
			return
		}
		var req struct {
			Conta          string `json:"conta"`
			DepartamentoID string `json:"departamento_id"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req) != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		out, err := svc.CorrigirDepartamento(r.Context(), c.EmpresaID, c.UserID, req.Conta, req.DepartamentoID)
		if erroDicionario(w, err, "corrigir o departamento") {
			return
		}
		writeJSON(w, out)
	}
}
