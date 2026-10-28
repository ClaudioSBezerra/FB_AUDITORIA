package handlers

// dre_import.go — upload de CSV ou XLSX de lançamentos. O handler só decodifica o
// multipart e chama dre.Ingestor, o único escritor de lançamentos.

import (
	"database/sql"
	"errors"
	"log"
	"net/http"

	"fb_auditoria/dre"
	csvfonte "fb_auditoria/dre/fontes/csv"
)

// DREImportarHandler — POST /api/aud/dre/lancamentos/importar (multipart "arquivo").
// Tudo ou nada; reenviar substitui os escopos (competência, filial) do arquivo.
func DREImportarHandler(db *sql.DB) http.HandlerFunc {
	ing := &dre.Ingestor{DB: db}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
		f, hdr, err := r.FormFile("arquivo")
		if err != nil {
			http.Error(w, "arquivo obrigatório", http.StatusBadRequest)
			return
		}
		defer f.Close()

		fonte := csvfonte.Nova(f, hdr.Filename)
		res, err := ing.Importar(r.Context(), c.EmpresaID, c.UserID, fonte)
		if err != nil {
			var ee *dre.ErroEntrada
			var el *dre.ErroLinhas
			var pf *dre.ErrPeriodoFechado
			switch {
			case errors.As(err, &pf):
				http.Error(w, err.Error(), http.StatusConflict)
			case errors.As(err, &ee), errors.As(err, &el):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				log.Printf("dre import: %v", err)
				http.Error(w, "Erro ao importar", http.StatusInternalServerError)
			}
			return
		}
		writeJSON(w, map[string]any{"carga_id": res.CargaID, "linhas": res.Linhas, "ajustes_ignorados": fonte.AjustesIgnorados()})
	}
}
