package handlers

// aud_usuarios.go — gestão de perfil/áreas dos usuários DA EMPRESA ativa
// (controladoria). O admin de plataforma continua usando /api/admin/users.

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/lib/pq"
)

var perfisValidos = map[string]bool{"auditoria": true, "controladoria": true, "diretoria": true, "gestor": true}
var areasValidas = map[string]bool{AreaEmpresa: true, AreaConfiguracoes: true, AreaComercial: true, AreaOperacao: true, AreaAcompanhamento: true}

// AudUsuariosHandler — GET lista | PUT {user_id, perfil, areas}.
func AudUsuariosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		// Usuários que alcançam a empresa ativa pela cadeia ambiente → grupo → empresa.
		const escopo = `u.id IN (SELECT ue.user_id FROM user_environments ue
			JOIN enterprise_groups eg ON eg.environment_id = ue.environment_id
			JOIN companies c ON c.group_id = eg.id WHERE c.id = $1::uuid)`
		switch r.Method {
		case http.MethodGet:
			rows, err := db.Query(`SELECT u.id, u.full_name, u.email, u.perfil, u.areas FROM users u WHERE `+escopo+` ORDER BY u.full_name`, c.EmpresaID)
			if err != nil {
				http.Error(w, "Erro", http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			type u struct {
				ID     string   `json:"id"`
				Nome   string   `json:"full_name"`
				Email  string   `json:"email"`
				Perfil string   `json:"perfil"`
				Areas  []string `json:"areas"`
			}
			out := []u{}
			for rows.Next() {
				var x u
				var areas pq.StringArray
				if rows.Scan(&x.ID, &x.Nome, &x.Email, &x.Perfil, &areas) == nil {
					x.Areas = []string(areas)
					out = append(out, x)
				}
			}
			writeJSON(w, out)
		case http.MethodPut:
			var req struct {
				UserID string   `json:"user_id"`
				Perfil string   `json:"perfil"`
				Areas  []string `json:"areas"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req) != nil || req.UserID == "" || !perfisValidos[req.Perfil] || len(req.Areas) == 0 {
				http.Error(w, "user_id, perfil e areas obrigatórios", http.StatusBadRequest)
				return
			}
			for _, a := range req.Areas {
				if !areasValidas[a] {
					http.Error(w, "área inválida: "+a, http.StatusBadRequest)
					return
				}
			}
			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Erro", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()
			res, err := tx.Exec(`UPDATE users u SET perfil = $2, areas = $3, updated_at = now()
				WHERE u.id = $1::uuid AND `+`u.id IN (SELECT ue.user_id FROM user_environments ue
				JOIN enterprise_groups eg ON eg.environment_id = ue.environment_id
				JOIN companies c ON c.group_id = eg.id WHERE c.id = $4::uuid)`,
				req.UserID, req.Perfil, pq.Array(req.Areas), c.EmpresaID)
			if err != nil {
				http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
				return
			}
			if n, _ := res.RowsAffected(); n == 0 {
				http.Error(w, "usuário não encontrado nesta empresa", http.StatusNotFound)
				return
			}
			if err := writeAuditLogTx(tx, c.EmpresaID, c.UserID, "usuario", req.UserID, "alterar_perfil", req); err != nil || tx.Commit() != nil {
				http.Error(w, "Erro ao registrar auditoria", http.StatusInternalServerError)
				return
			}
			writeJSON(w, req)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
