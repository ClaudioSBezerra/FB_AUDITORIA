package handlers

// ambiente.go — marcador Teste/Produção e limpeza dos dados de teste (AD-18).

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

// conferirSenha confere a senha digitada de novo contra o hash do usuário (mesmo
// bcrypt do login). Devolve status 0 quando confere; senão o status e a mensagem.
func conferirSenha(db *sql.DB, userID, senha string) (int, string) {
	var hash string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE id = $1::uuid`, userID).Scan(&hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return http.StatusForbidden, "Senha incorreta"
		}
		return http.StatusInternalServerError, "Erro ao conferir senha"
	}
	if !CheckPasswordHash(senha, hash) {
		return http.StatusForbidden, "Senha incorreta"
	}
	return 0, ""
}

// FraseVirarProducao é a confirmação digitada para virar a empresa para Produção.
const FraseVirarProducao = "VIRAR PARA PRODUCAO"

// AmbienteVirarHandler — POST /api/aud/ambiente/virar, corpo {"senha","confirmacao"}.
// Só admin de plataforma, com senha e frase exatas; a virada é de mão única.
func AmbienteVirarHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if c == nil || !c.Admin {
			http.Error(w, "Apenas o administrador da plataforma pode virar para Produção", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var body struct {
			Senha       string `json:"senha"`
			Confirmacao string `json:"confirmacao"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Senha == "" {
			http.Error(w, "Senha obrigatória", http.StatusForbidden)
			return
		}
		if status, msg := conferirSenha(db, c.UserID, body.Senha); status != 0 {
			http.Error(w, msg, status)
			return
		}
		if body.Confirmacao != FraseVirarProducao {
			http.Error(w, "Frase de confirmação incorreta", http.StatusForbidden)
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
			http.Error(w, "Empresa já está em Produção", http.StatusConflict)
			return
		}
		if _, err := tx.Exec(`UPDATE companies SET ambiente = 'producao' WHERE id = $1::uuid`, c.EmpresaID); err != nil {
			http.Error(w, "Erro ao virar para Produção", http.StatusInternalServerError)
			return
		}

		contas := []string{}
		rows, err := tx.Query(`SELECT DISTINCT COALESCE(conta,'') FROM auditoria.lancamentos
			WHERE empresa_id = $1::uuid AND (origem = 'csv' OR origem LIKE 'csv.%') ORDER BY 1`, c.EmpresaID)
		if err != nil {
			http.Error(w, "Erro ao listar contas de arquivo", http.StatusInternalServerError)
			return
		}
		for rows.Next() {
			var conta string
			if err := rows.Scan(&conta); err != nil {
				rows.Close()
				http.Error(w, "Erro ao listar contas de arquivo", http.StatusInternalServerError)
				return
			}
			contas = append(contas, conta)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			http.Error(w, "Erro ao listar contas de arquivo", http.StatusInternalServerError)
			return
		}

		if err := writeAuditLogTx(tx, c.EmpresaID, c.UserID, "ambiente", c.EmpresaID, "virar_producao", map[string]any{"contas_de_arquivo": len(contas)}); err != nil {
			http.Error(w, "Erro ao gravar trilha", http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, "Erro ao confirmar", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"ambiente": "producao", "contas_de_arquivo": contas}); err != nil {
			log.Printf("ambiente virar: erro ao responder: %v", err)
		}
	}
}

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
		if status, msg := conferirSenha(db, c.UserID, body.Senha); status != 0 {
			http.Error(w, msg, status)
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
