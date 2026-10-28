// Package trilha grava na trilha imutável (auditoria.audit_log). Fica fora de
// handlers para que o pacote dre também possa gravá-la dentro da sua transação.
package trilha

import (
	"database/sql"
	"encoding/json"
)

// Gravar insere uma entrada na trilha dentro da transação tx. O erro deve
// abortar a operação quando o registro é obrigatório.
func Gravar(tx *sql.Tx, empresaID, userID, entidade, entidadeID, acao string, payload any) error {
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
