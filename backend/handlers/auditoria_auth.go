package handlers

// auditoria_auth.go — AuditoriaAuthMiddleware
//
// Mesma cadeia do FarolAuthMiddleware (ver FB_FAROL):
//   1. AuthMiddleware (auth.go) valida o JWT e injeta o user_id
//   2. AuditoriaAuthMiddleware resolve a empresa ativa (X-Company-ID), carrega
//      perfil/áreas do banco e aplica licença e bloqueio de empresa.
//
// Perfis (users.perfil): auditoria | controladoria | diretoria | gestor.
// users.role = 'admin' é o admin de plataforma e passa em qualquer perfil.

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/lib/pq"
)

type audCtxKey string

const AudContextKey audCtxKey = "aud_context"

// Áreas do menu. Cada rota declara a área a que pertence.
const (
	AreaEmpresa       = "empresa" // rótulo "Administrativo"
	AreaConfiguracoes = "configuracoes"
	AreaComercial     = "comercial"
	AreaOperacao      = "operacao"
	// AreaAcompanhamento (história 4.10): só enxerga o quadro de andamento do
	// projeto. Não entra no DEFAULT de users.areas; é concedida pela tela de usuários.
	AreaAcompanhamento = "acompanhamento"
)

type AudContext struct {
	UserID    string
	EmpresaID string
	Perfil    string
	Areas     []string
	Admin     bool // role=admin (plataforma)
}

func GetAudContext(r *http.Request) *AudContext {
	c, _ := r.Context().Value(AudContextKey).(*AudContext)
	return c
}

func (c *AudContext) TemArea(area string) bool {
	if c.Admin {
		return true
	}
	for _, a := range c.Areas {
		if a == area {
			return true
		}
	}
	return false
}

// Tem retorna true se o perfil do usuário está entre os aceitos.
func (c *AudContext) Tem(perfis ...string) bool {
	if c.Admin {
		return true
	}
	for _, p := range perfis {
		if c.Perfil == p {
			return true
		}
	}
	return false
}

// PodeGerirUsuarios — só admin de plataforma e controladoria.
func (c *AudContext) PodeGerirUsuarios() bool { return c.Tem("controladoria") }

// AuditoriaAuthMiddleware: area == "" só exige autenticação; perfis vazio =
// qualquer perfil com acesso à área.
func AuditoriaAuthMiddleware(db *sql.DB, next http.HandlerFunc, area string, perfis ...string) http.HandlerFunc {
	return AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserIDFromContext(r)
		if userID == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		empresaID, err := GetEffectiveCompanyID(db, userID, r.Header.Get("X-Company-ID"))
		if err != nil || empresaID == "" {
			http.Error(w, "Could not determine active company", http.StatusUnauthorized)
			return
		}

		var role, perfil string
		var trialEndsAt sql.NullTime
		var areas pq.StringArray
		if err := db.QueryRow(
			`SELECT role, perfil, trial_ends_at, areas FROM users WHERE id = $1`, userID,
		).Scan(&role, &perfil, &trialEndsAt, &areas); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "User not found", http.StatusUnauthorized)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		c := &AudContext{UserID: userID, EmpresaID: empresaID, Perfil: perfil, Areas: []string(areas), Admin: role == "admin"}

		if area != "" && !c.TemArea(area) {
			http.Error(w, "Forbidden: sem acesso à área "+area, http.StatusForbidden)
			return
		}
		if len(perfis) > 0 && !c.Tem(perfis...) {
			http.Error(w, "Forbidden: perfil insuficiente", http.StatusForbidden)
			return
		}

		if !c.Admin && trialEndsAt.Valid && trialEndsAt.Time.Before(time.Now()) {
			http.Error(w, "Licença expirada. Contate o administrador MASTER para renovar.", http.StatusForbidden)
			return
		}
		if !c.Admin {
			var blockedAt sql.NullTime
			_ = db.QueryRow(`SELECT blocked_at FROM companies WHERE id = $1::uuid`, empresaID).Scan(&blockedAt)
			if blockedAt.Valid {
				http.Error(w, "Empresa bloqueada. Contate o administrador MASTER.", http.StatusForbidden)
				return
			}
		}

		next(w, r.WithContext(context.WithValue(r.Context(), AudContextKey, c)))
	}, "")
}

// AudMeHandler — GET /api/aud/me: o que o front precisa pra montar o menu.
func AudMeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if c == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		var ambiente string
		if err := db.QueryRow(`SELECT ambiente FROM companies WHERE id = $1::uuid`, c.EmpresaID).Scan(&ambiente); err != nil {
			log.Printf("aud/me: erro ao consultar ambiente da empresa %s: %v", c.EmpresaID, err)
			ambiente = ""
		}
		areas := c.Areas
		if c.Admin {
			areas = []string{AreaEmpresa, AreaConfiguracoes, AreaComercial, AreaOperacao, AreaAcompanhamento}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"perfil":           c.Perfil,
			"areas":            areas,
			"admin":            c.Admin,
			"can_manage_users": c.PodeGerirUsuarios(),
			"ambiente":         ambiente,
		})
	}
}
