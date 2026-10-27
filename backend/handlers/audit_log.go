package handlers

// audit_log.go — trilha imutável (auditoria.audit_log, bloqueada por trigger
// contra UPDATE/DELETE — ver migration 002).

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// writeAuditLog grava uma entrada. Falha é logada, não derruba a operação —
// use writeAuditLogTx onde o registro é obrigatório (fechamento, mapa, etc.).
func writeAuditLog(db *sql.DB, empresaID, userID, entidade, entidadeID, acao string, payload any) {
	var p []byte
	if payload != nil {
		p, _ = json.Marshal(payload)
	}
	if _, err := db.Exec(`
		INSERT INTO auditoria.audit_log (empresa_id, user_id, entidade, entidade_id, acao, payload)
		VALUES ($1::uuid, NULLIF($2,'')::uuid, $3, $4, $5, $6)`,
		empresaID, userID, entidade, entidadeID, acao, p); err != nil {
		log.Printf("writeAuditLog: ERRO ao gravar (%s/%s/%s): %v", entidade, entidadeID, acao, err)
	}
}

func writeAuditLogTx(tx *sql.Tx, empresaID, userID, entidade, entidadeID, acao string, payload any) error {
	var p []byte
	if payload != nil {
		var err error
		if p, err = json.Marshal(payload); err != nil {
			return err
		}
	}
	_, err := tx.Exec(`
		INSERT INTO auditoria.audit_log (empresa_id, user_id, entidade, entidade_id, acao, payload)
		VALUES ($1::uuid, NULLIF($2,'')::uuid, $3, $4, $5, $6)`,
		empresaID, userID, entidade, entidadeID, acao, p)
	return err
}

// AuditLogHandler — GET /api/aud/audit-log?limit=&entidade=
func AuditLogHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		rows, err := db.Query(`
			SELECT l.id, COALESCE(u.full_name,''), l.entidade, l.entidade_id, l.acao,
			       COALESCE(l.payload::text,'null'), l.criado_em
			FROM auditoria.audit_log l LEFT JOIN users u ON u.id = l.user_id
			WHERE l.empresa_id = $1::uuid AND ($2 = '' OR l.entidade = $2)
			ORDER BY l.id DESC LIMIT $3`, c.EmpresaID, r.URL.Query().Get("entidade"), limit)
		if err != nil {
			http.Error(w, "Erro ao consultar log", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type item struct {
			ID         int64           `json:"id"`
			Usuario    string          `json:"usuario"`
			Entidade   string          `json:"entidade"`
			EntidadeID string          `json:"entidade_id"`
			Acao       string          `json:"acao"`
			Payload    json.RawMessage `json:"payload"`
			CriadoEm   string          `json:"criado_em"`
		}
		out := []item{}
		for rows.Next() {
			var it item
			var p string
			if rows.Scan(&it.ID, &it.Usuario, &it.Entidade, &it.EntidadeID, &it.Acao, &p, &it.CriadoEm) == nil {
				it.Payload = json.RawMessage(p)
				out = append(out, it)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}
