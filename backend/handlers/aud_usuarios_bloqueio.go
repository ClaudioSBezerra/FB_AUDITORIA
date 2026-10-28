package handlers

// aud_usuarios_bloqueio.go — bloquear e desbloquear usuário da empresa ativa
// (história 4.6, FR-22, AD-19). Capacidade gerir_usuarios; motivo obrigatório;
// o registro vai à trilha na mesma transação.

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"
)

const motivoBloqueioMax = 500

// AudUsuarioBloquearHandler — POST /api/aud/usuarios/bloquear {user_id, motivo}.
func AudUsuarioBloquearHandler(db *sql.DB) http.HandlerFunc { return bloqueioHandler(db, true) }

// AudUsuarioDesbloquearHandler — POST /api/aud/usuarios/desbloquear {user_id, motivo}.
func AudUsuarioDesbloquearHandler(db *sql.DB) http.HandlerFunc { return bloqueioHandler(db, false) }

func bloqueioHandler(db *sql.DB, bloquear bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if c == nil || !c.Pode(CapGerirUsuarios) {
			http.Error(w, "Forbidden: perfil insuficiente", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			UserID string `json:"user_id"`
			Motivo string `json:"motivo"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req) != nil || req.UserID == "" {
			http.Error(w, "user_id e motivo obrigatórios", http.StatusBadRequest)
			return
		}
		motivo := strings.TrimSpace(req.Motivo)
		if motivo == "" {
			http.Error(w, "motivo obrigatório", http.StatusBadRequest)
			return
		}
		if utf8.RuneCountInString(motivo) > motivoBloqueioMax {
			http.Error(w, "motivo longo demais (máximo 500 caracteres)", http.StatusBadRequest)
			return
		}
		if !reUUIDHandler.MatchString(req.UserID) {
			http.Error(w, "usuário não encontrado nesta empresa", http.StatusNotFound)
			return
		}
		if bloquear && strings.EqualFold(req.UserID, c.UserID) {
			http.Error(w, "você não pode bloquear a si mesmo", http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		var role string
		var jaBloqueado bool
		var motivoAntes sql.NullString
		err = tx.QueryRow(`SELECT u.role, u.bloqueado_em IS NOT NULL, u.motivo_bloqueio FROM users u
			WHERE u.id = $1::uuid AND `+usuariosDaEmpresa("$2")+` FOR UPDATE OF u`, req.UserID, c.EmpresaID).Scan(&role, &jaBloqueado, &motivoAntes)
		if err == sql.ErrNoRows {
			http.Error(w, "usuário não encontrado nesta empresa", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("bloqueio: %v", err)
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}

		if bloquear {
			if jaBloqueado {
				http.Error(w, "usuário já está bloqueado", http.StatusConflict)
				return
			}
			if role == "admin" {
				// Administrador de plataforma: só outro administrador o bloqueia, e nunca o último ativo.
				if !c.Admin {
					http.Error(w, "Forbidden: só o administrador da plataforma bloqueia outro administrador", http.StatusForbidden)
					return
				}
				// Trava os administradores para duas requisições não bloquearem os dois ao mesmo tempo.
				rows, err := tx.Query(`SELECT id FROM users WHERE role = 'admin' ORDER BY id FOR UPDATE`)
				if err != nil {
					http.Error(w, "Erro", http.StatusInternalServerError)
					return
				}
				rows.Close()
				var outros int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin' AND bloqueado_em IS NULL AND id <> $1::uuid`, req.UserID).Scan(&outros); err != nil {
					http.Error(w, "Erro", http.StatusInternalServerError)
					return
				}
				if outros == 0 {
					http.Error(w, "o último administrador da plataforma ativo não pode ser bloqueado", http.StatusConflict)
					return
				}
			}
			if _, err := tx.Exec(`UPDATE users SET bloqueado_em = now(), bloqueado_por = $2::uuid, motivo_bloqueio = $3, updated_at = now()
				WHERE id = $1::uuid`, req.UserID, c.UserID, motivo); err != nil {
				log.Printf("bloqueio: %v", err)
				http.Error(w, "Erro ao bloquear", http.StatusInternalServerError)
				return
			}
			err = writeAuditLogTx(tx, c.EmpresaID, c.UserID, "usuario", req.UserID, "bloquear",
				map[string]any{"motivo": motivo, "antes": map[string]any{"bloqueado": false}, "depois": map[string]any{"bloqueado": true}})
		} else {
			if !jaBloqueado {
				http.Error(w, "usuário não está bloqueado", http.StatusConflict)
				return
			}
			if _, err := tx.Exec(`UPDATE users SET bloqueado_em = NULL, bloqueado_por = NULL, motivo_bloqueio = NULL, updated_at = now()
				WHERE id = $1::uuid`, req.UserID); err != nil {
				log.Printf("desbloqueio: %v", err)
				http.Error(w, "Erro ao desbloquear", http.StatusInternalServerError)
				return
			}
			err = writeAuditLogTx(tx, c.EmpresaID, c.UserID, "usuario", req.UserID, "desbloquear",
				map[string]any{"motivo": motivo, "antes": map[string]any{"bloqueado": true, "motivo_bloqueio": motivoAntes.String}, "depois": map[string]any{"bloqueado": false}})
		}
		if err != nil || tx.Commit() != nil {
			http.Error(w, "Erro ao registrar auditoria", http.StatusInternalServerError)
			return
		}
		if bloquear {
			revogarSessoesDe(req.UserID)
		}
		writeJSON(w, map[string]any{"user_id": req.UserID, "bloqueado": bloquear})
	}
}

// revogarSessoesDe apaga os refresh tokens do usuário. O access token em uso morre
// na requisição seguinte pela consulta de bloqueio, não por esta limpeza.
func revogarSessoesDe(userID string) {
	refreshTokenStore.Range(func(k, v any) bool {
		if d, ok := v.(refreshTokenData); ok && strings.EqualFold(d.UserID, userID) {
			refreshTokenStore.Delete(k)
		}
		return true
	})
}
