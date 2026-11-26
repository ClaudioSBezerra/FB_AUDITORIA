package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"fb_auditoria/dre"
)

// RetiradaHandler — /api/aud/movimentacao/retirada (capacidade carregar).
// GET ?competencia=AAAA-MM (vazia = tudo): o que seria apagado.
// POST {competencia, confirmacao}: apaga a movimentação.
func RetiradaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if !c.Pode(CapCarregar) {
			writeAuditLog(db, c.EmpresaID, c.UserID, "lancamentos", "", "negado", map[string]any{"perfil": c.Perfil})
			http.Error(w, "Forbidden: perfil sem a capacidade de carregar dados", http.StatusForbidden)
			return
		}
		ing := &dre.Ingestor{DB: db}
		var out dre.Retirada
		var err error
		switch r.Method {
		case http.MethodGet:
			out, err = ing.PreverRetirada(r.Context(), c.EmpresaID, r.URL.Query().Get("competencia"))
		case http.MethodPost:
			var in struct{ Competencia, Confirmacao string }
			if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); e != nil {
				http.Error(w, "corpo inválido", http.StatusBadRequest)
				return
			}
			out, err = ing.Retirar(r.Context(), c.EmpresaID, c.UserID, in.Competencia, in.Confirmacao)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var entrada *dre.ErroEntrada
		var fechado *dre.ErrPeriodoFechado
		switch {
		case errors.As(err, &entrada):
			http.Error(w, entrada.Error(), http.StatusBadRequest)
		case errors.As(err, &fechado):
			http.Error(w, fechado.Error()+". Nada foi apagado.", http.StatusConflict)
		case errors.Is(err, dre.ErrImportacaoEmAndamento):
			http.Error(w, err.Error(), http.StatusConflict)
		case err != nil:
			log.Printf("retirada: %v", err)
			http.Error(w, "Erro ao retirar a movimentação", http.StatusInternalServerError)
		default:
			writeJSON(w, out)
		}
	}
}
