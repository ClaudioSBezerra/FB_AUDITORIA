package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"fb_auditoria/dre"
	csvfonte "fb_auditoria/dre/fontes/csv"
)

// DepartamentosHandler — cadastro de departamentos. GET lista (qualquer perfil da
// área); POST/PUT salvam {id?, codigo, nome, grupo}; DELETE ?id= exclui.
// Escrita exige editar_estrutura.
func DepartamentosHandler(db *sql.DB) http.HandlerFunc {
	svc := &dre.Dicionario{DB: db}
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if r.Method != http.MethodGet && !c.Pode(CapEditarEstrutura) {
			http.Error(w, "Forbidden: perfil insuficiente", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet:
			l, err := svc.ListarDepartamentos(r.Context(), c.EmpresaID)
			if err != nil {
				log.Printf("departamentos listar: %v", err)
				http.Error(w, "Erro ao listar departamentos", http.StatusInternalServerError)
				return
			}
			writeJSON(w, l)
		case http.MethodPost, http.MethodPut:
			var req struct {
				ID     string `json:"id"`
				Codigo string `json:"codigo"`
				Nome   string `json:"nome"`
				Grupo  string `json:"grupo"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req) != nil {
				http.Error(w, "corpo inválido", http.StatusBadRequest)
				return
			}
			if r.Method == http.MethodPut && req.ID == "" {
				http.Error(w, "id obrigatório para editar", http.StatusBadRequest)
				return
			}
			if r.Method == http.MethodPost {
				req.ID = ""
			}
			out, err := svc.SalvarDepartamento(r.Context(), dre.SalvarDepartamentoIn{
				EmpresaID: c.EmpresaID, UserID: c.UserID, ID: req.ID, Codigo: req.Codigo, Nome: req.Nome, Grupo: req.Grupo,
			})
			if erroDicionario(w, err, "salvar departamento") {
				return
			}
			if r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(out)
				return
			}
			writeJSON(w, out)
		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id obrigatório", http.StatusBadRequest)
				return
			}
			if erroDicionario(w, svc.ExcluirDepartamento(r.Context(), c.EmpresaID, c.UserID, id), "excluir departamento") {
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// DepartamentosCargaHandler — POST /api/aud/departamentos/carga (multipart
// "arquivo", CSV ou XLSX; colunas codigo, nome, grupo). Upsert por código: cria os
// novos e atualiza nome e grupo; não apaga os que não vierem. Tudo ou nada.
// Mesma capacidade do cadastro manual (editar_estrutura).
func DepartamentosCargaHandler(db *sql.DB) http.HandlerFunc {
	svc := &dre.Dicionario{DB: db}
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
		linhas, probs, err := csvfonte.LerDepartamentos(f)
		if erroDicionario(w, err, "ler os departamentos") {
			return
		}
		out, err := svc.CarregarDepartamentos(r.Context(), dre.CargaDepartamentosIn{
			EmpresaID: c.EmpresaID, UserID: c.UserID, Arquivo: hdr.Filename, Linhas: linhas, Problemas: probs,
		})
		if erroDicionario(w, err, "carregar os departamentos") {
			return
		}
		writeJSON(w, out)
	}
}

// erroDicionario mapeia o erro do serviço para HTTP; devolve true se respondeu.
func erroDicionario(w http.ResponseWriter, err error, op string) bool {
	if err == nil {
		return false
	}
	var entrada *dre.ErroEntrada
	var linhas *dre.ErroLinhas
	var codigo *dre.ErrDepartamentoCodigoEmUso
	var emUso *dre.ErrDepartamentoEmUso
	var naoAch *dre.ErrDepartamentoNaoEncontrado
	var contaNaoAch *dre.ErrContaNaoEncontrada
	switch {
	case errors.As(err, &entrada), errors.As(err, &linhas):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.As(err, &codigo), errors.As(err, &emUso):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.As(err, &naoAch), errors.As(err, &contaNaoAch):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		log.Printf("dicionario %s: %v", op, err)
		http.Error(w, "Erro ao "+op, http.StatusInternalServerError)
	}
	return true
}
