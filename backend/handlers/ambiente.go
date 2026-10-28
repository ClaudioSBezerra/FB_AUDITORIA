package handlers

// ambiente.go — marcador Teste/Produção e limpeza dos dados de teste (AD-18).

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

// AmbienteLimparHandler — POST /api/aud/ambiente/limpar, corpo {"senha":"..."}.
// Só admin de plataforma, com a senha conferida; só em ambiente 'teste'.
func AmbienteLimparHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if c == nil || !c.Admin {
			http.Error(w, "Apenas o administrador da plataforma pode apagar dados de teste", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var body struct {
			Senha string `json:"senha"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Senha == "" {
			http.Error(w, "Senha obrigatória", http.StatusForbidden)
			return
		}
		var hash string
		if err := db.QueryRow(`SELECT password_hash FROM users WHERE id = $1::uuid`, c.UserID).Scan(&hash); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Senha incorreta", http.StatusForbidden)
			} else {
				http.Error(w, "Erro ao conferir senha", http.StatusInternalServerError)
			}
			return
		}
		if !CheckPasswordHash(body.Senha, hash) {
			http.Error(w, "Senha incorreta", http.StatusForbidden)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Erro ao iniciar transação", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		var amb string
		if err := tx.QueryRow(`SELECT ambiente FROM companies WHERE id = $1::uuid FOR UPDATE`, c.EmpresaID).Scan(&amb); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "Empresa não encontrada", http.StatusNotFound)
			} else {
				http.Error(w, "Erro ao consultar empresa", http.StatusInternalServerError)
			}
			return
		}
		if amb != "teste" {
			http.Error(w, "Empresa em Produção: dados não podem ser apagados", http.StatusConflict)
			return
		}

		var contagens []byte
		if err := tx.QueryRow(`SELECT auditoria.limpar_dados_teste($1::uuid)`, c.EmpresaID).Scan(&contagens); err != nil {
			http.Error(w, "Erro ao apagar dados de teste", http.StatusInternalServerError)
			return
		}
		var apagadas map[string]int64
		if err := json.Unmarshal(contagens, &apagadas); err != nil {
			http.Error(w, "Erro ao ler contagens da limpeza", http.StatusInternalServerError)
			return
		}
		if err := writeAuditLogTx(tx, c.EmpresaID, c.UserID, "ambiente", c.EmpresaID, "limpar_dados_teste", map[string]any{"apagadas": apagadas}); err != nil {
			http.Error(w, "Erro ao gravar trilha", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, "Erro ao confirmar", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"apagadas": apagadas}); err != nil {
			log.Printf("ambiente limpar: erro ao responder: %v", err)
		}
	}
}
