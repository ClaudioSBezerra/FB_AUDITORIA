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

// AgrupamentosFiliaisHandler — agrupamentos de filiais gerenciais (Balcões, Distribuição).
// GET lista (qualquer perfil da área); POST/PUT salvam {id?, codigo, nome, filiais[]}
// (códigos de filiais gerenciais); DELETE ?id= exclui. Escrita exige editar_estrutura.
func AgrupamentosFiliaisHandler(db *sql.DB) http.HandlerFunc {
	svc := &dre.Agrupamentos{DB: db}
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if r.Method != http.MethodGet && !c.Pode(CapEditarEstrutura) {
			http.Error(w, "Forbidden: perfil insuficiente", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet:
			l, err := svc.Listar(r.Context(), c.EmpresaID)
			if err != nil {
				log.Printf("agrupamentos listar: %v", err)
				http.Error(w, "Erro ao listar agrupamentos", http.StatusInternalServerError)
				return
			}
			writeJSON(w, l)
		case http.MethodPost, http.MethodPut:
			var req struct {
				ID      string    `json:"id"`
				Codigo  string    `json:"codigo"`
				Nome    string    `json:"nome"`
				Filiais *[]string `json:"filiais"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req) != nil {
				http.Error(w, "corpo inválido", http.StatusBadRequest)
				return
			}
			if r.Method == http.MethodPut && req.ID == "" {
				http.Error(w, "id obrigatório para editar", http.StatusBadRequest)
				return
			}
			if r.Method == http.MethodPut && req.Filiais == nil {
				http.Error(w, "filiais obrigatório para editar (use [] para nenhuma)", http.StatusBadRequest)
				return
			}
			if r.Method == http.MethodPost {
				req.ID = ""
			}
			var filiais []string
			if req.Filiais != nil {
				filiais = *req.Filiais
			}
			out, err := svc.Salvar(r.Context(), dre.SalvarAgrupamentoIn{
				EmpresaID: c.EmpresaID, UserID: c.UserID, ID: req.ID, Codigo: req.Codigo, Nome: req.Nome, Filiais: filiais,
			})
			if erroAgrupamento(w, err, "salvar") {
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
			if erroAgrupamento(w, svc.Excluir(r.Context(), c.EmpresaID, c.UserID, id), "excluir") {
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// erroAgrupamento mapeia o erro do serviço para HTTP; devolve true se respondeu.
func erroAgrupamento(w http.ResponseWriter, err error, op string) bool {
	if err == nil {
		return false
	}
	var entrada *dre.ErroEntrada
	var emUso *dre.ErrAgrupamentoCodigoEmUso
	var colide *dre.ErrCodigoColide
	var membro *dre.ErrMembroInexistente
	var naoAch *dre.ErrAgrupamentoNaoEncontrado
	switch {
	case errors.As(err, &entrada):
		http.Error(w, entrada.Msg, http.StatusBadRequest)
	case errors.As(err, &membro):
		http.Error(w, membro.Error(), http.StatusBadRequest)
	case errors.As(err, &emUso):
		http.Error(w, emUso.Error(), http.StatusConflict)
	case errors.As(err, &colide):
		http.Error(w, colide.Error(), http.StatusConflict)
	case errors.As(err, &naoAch):
		http.Error(w, naoAch.Error(), http.StatusNotFound)
	default:
		log.Printf("agrupamentos %s: %v", op, err)
		http.Error(w, "Erro ao "+op+" agrupamento", http.StatusInternalServerError)
	}
	return true
}

// AgrupamentosFiliaisCargaHandler — POST /api/aud/agrupamentos-filiais/carga
// (multipart "arquivo", CSV ou XLSX; colunas agrupamento e filial, nome opcional).
// Cria os agrupamentos que faltam e acrescenta os membros; as filiais precisam já
// existir como filiais gerenciais. Não apaga nada. Tudo ou nada. Mesma capacidade
// do cadastro manual (editar_estrutura).
func AgrupamentosFiliaisCargaHandler(db *sql.DB) http.HandlerFunc {
	svc := &dre.Agrupamentos{DB: db}
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
		linhas, probs, err := csvfonte.LerAgrupamentos(f)
		if erroCargaFiliais(w, err, "ler os agrupamentos") {
			return
		}
		out, err := svc.CarregarAgrupamentos(r.Context(), dre.CargaAgrupamentosIn{
			EmpresaID: c.EmpresaID, UserID: c.UserID, Arquivo: hdr.Filename, Linhas: linhas, Problemas: probs,
		})
		if erroCargaFiliais(w, err, "carregar os agrupamentos") {
			return
		}
		writeJSON(w, out)
	}
}
