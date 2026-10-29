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
//
// Com ?simular=1 (história 2.19) lê e valida o arquivo, aplica a guarda de
// período e devolve o MESMO resumo (pares competência/filial, avisos, conferência
// do TOTAL) sem gravar nada: nem carga, nem lançamentos, nem trilha. A tela faz a
// simulação antes de gravar. No backend a gravação direta (sem simular) continua
// possível, por compatibilidade com o CSV do sistema e com o Atualizar dados.
func DREImportarHandler(db *sql.DB) http.HandlerFunc {
	ing := &dre.Ingestor{DB: db}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		r.Body = http.MaxBytesReader(w, r.Body, limiteCargaBytes())
		f, hdr, err := r.FormFile("arquivo")
		if err != nil {
			erroUploadCarga(w, err)
			return
		}
		defer f.Close()

		fonte := csvfonte.Nova(f, hdr.Filename)
		simular := r.URL.Query().Get("simular") == "1"
		var res dre.Resultado
		var resumo dre.Resumo
		if simular {
			resumo, err = ing.Simular(r.Context(), c.EmpresaID, fonte)
		} else {
			res, err = ing.Importar(r.Context(), c.EmpresaID, c.UserID, fonte)
			resumo = res.Resumo
		}
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
		resp := map[string]any{"simulacao": simular, "linhas": resumo.Total, "origem": resumo.Origem, "arquivo": resumo.Arquivo,
			"pares": resumo.Pares, "avisos": resumo.Avisos,
			"ajustes_ignorados": fonte.AjustesIgnorados(), "linhas_ignoradas": fonte.LinhasIgnoradas()}
		if !simular {
			resp["carga_id"] = res.CargaID
		}
		if c := fonte.Conferencia(); c != nil {
			resp["conferencia_total"] = c
		}
		writeJSON(w, resp)
	}
}
