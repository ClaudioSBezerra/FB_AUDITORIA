package handlers

// escopo.go — escopo por departamento que nega por padrão (AD-10, história 4.7).
//
// O servidor calcula o escopo a partir do perfil; nunca vem do cliente. Quem tem
// a capacidade `ler` é irrestrito (controladoria, analista, diretoria, auditoria,
// admin de plataforma). Quem só tem `ler_no_escopo` (gestor) enxerga os
// departamentos de usuario_departamentos na empresa ativa. Quem não tem nenhuma
// das duas, ou o gestor sem departamento, recebe o valor zero, que não dá acesso.

import (
	"context"
	"database/sql"
	"net/http"

	"fb_auditoria/dre"
)

// escopoDe calcula o escopo do usuário na empresa ativa.
func escopoDe(ctx context.Context, db *sql.DB, c *AudContext) (dre.EscopoDepto, error) {
	if c == nil {
		return dre.EscopoDepto{}, nil
	}
	if c.Pode(CapLer) {
		return dre.EscopoDepto{Irrestrito: true}, nil
	}
	if !c.Pode(CapLerNoEscopo) {
		return dre.EscopoDepto{}, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT d.codigo FROM auditoria.usuario_departamentos ud
		JOIN auditoria.departamentos d ON d.id = ud.departamento_id AND d.empresa_id = ud.empresa_id
		WHERE ud.user_id = $1::uuid AND ud.empresa_id = $2::uuid ORDER BY d.codigo`, c.UserID, c.EmpresaID)
	if err != nil {
		return dre.EscopoDepto{}, err
	}
	defer rows.Close()
	esc := dre.EscopoDepto{}
	for rows.Next() {
		var cod string
		if err := rows.Scan(&cod); err != nil {
			return dre.EscopoDepto{}, err
		}
		esc.Deptos = append(esc.Deptos, cod)
	}
	return esc, rows.Err()
}

// exigeLer recusa (403, registrado na trilha) quem não tem a leitura sem restrição:
// a rota devolve valor do grupo (faturamento, margens), que o escopo por
// departamento não cobre.
func exigeLer(w http.ResponseWriter, c *AudContext, db *sql.DB, entidade string) bool {
	if c.Pode(CapLer) {
		return true
	}
	writeAuditLog(db, c.EmpresaID, c.UserID, entidade, "", "negado", map[string]any{"perfil": c.Perfil})
	http.Error(w, "Forbidden: o perfil só vê o seu departamento", http.StatusForbidden)
	return false
}

// exigeLerNoEscopo recusa (403, registrado) o perfil sem nenhuma leitura. Quem
// passa ainda precisa do escopo calculado por escopoDe.
func exigeLerNoEscopo(w http.ResponseWriter, c *AudContext, db *sql.DB, entidade string) bool {
	if c.Pode(CapLerNoEscopo) {
		return true
	}
	writeAuditLog(db, c.EmpresaID, c.UserID, entidade, "", "negado", map[string]any{"perfil": c.Perfil})
	http.Error(w, "Forbidden: o perfil não tem acesso a este detalhe", http.StatusForbidden)
	return false
}
