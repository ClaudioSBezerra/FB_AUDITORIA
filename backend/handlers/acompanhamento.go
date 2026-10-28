package handlers

// acompanhamento.go — GET /api/aud/acompanhamento (história 4.10): Kanban do
// andamento do projeto, somente leitura. A área `acompanhamento` dá acesso só a
// esta rota; os dados vêm do JSON embutido no binário (pacote acompanhamento).

import (
	"database/sql"
	"net/http"

	"fb_auditoria/acompanhamento"
)

func AcompanhamentoHandler(_ *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "private, max-age=60")
		_, _ = w.Write(acompanhamento.JSON())
	}
}
