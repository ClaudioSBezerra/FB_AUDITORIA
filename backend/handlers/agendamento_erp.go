package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"fb_auditoria/dre"
)

// AgendamentoERPHandler — /api/aud/importacao-erp/agendamento (capacidade
// carregar). GET: a configuração. PUT {ativo, frequencia, dia, hora}: salva.
func AgendamentoERPHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { agendamentoERP(db, ERPPadrao.Load())(w, r) }
}

func agendamentoERP(db *sql.DB, erp *dre.ImportacaoERP) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if !c.Pode(CapCarregar) {
			writeAuditLog(db, c.EmpresaID, c.UserID, "agendamento_erp", "", "negado", map[string]any{"perfil": c.Perfil})
			http.Error(w, "Forbidden: perfil sem a capacidade de carregar dados", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet:
		case http.MethodPut:
			var in dre.Agendamento
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil {
				http.Error(w, "corpo inválido", http.StatusBadRequest)
				return
			}
			// Desligar sempre pode; ligar exige a conexão da própria empresa.
			if in.Ativo && (erp == nil || erp.EmpresaID != c.EmpresaID) {
				http.Error(w, "a conexão com o banco do cliente não está configurada para esta empresa", http.StatusServiceUnavailable)
				return
			}
			err := dre.SalvarAgendamento(r.Context(), db, c.EmpresaID, c.UserID, in, time.Now())
			var entrada *dre.ErroEntrada
			if errors.As(err, &entrada) {
				http.Error(w, entrada.Error(), http.StatusBadRequest)
				return
			}
			if err != nil {
				log.Printf("agendamento erp: salvar: %v", err)
				http.Error(w, "Erro ao salvar o agendamento", http.StatusInternalServerError)
				return
			}
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a, err := dre.LerAgendamento(r.Context(), db, c.EmpresaID, time.Now())
		if err != nil {
			log.Printf("agendamento erp: ler: %v", err)
			http.Error(w, "Erro ao ler o agendamento", http.StatusInternalServerError)
			return
		}
		writeJSON(w, a)
	}
}
