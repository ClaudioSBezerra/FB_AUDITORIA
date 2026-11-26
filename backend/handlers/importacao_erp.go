package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"fb_auditoria/dre"
)

// ERPPadrao é a importação do banco do cliente do processo; vazia quando as
// variáveis JC_ORACLE_* e JC_EMPRESA_ID não estão completas. Definida no boot,
// depois das migrations (por isso atômica: o servidor já atende).
var ERPPadrao atomic.Pointer[dre.ImportacaoERP]

// ImportacaoERPHandler — /api/aud/importacao-erp (capacidade carregar).
// GET: se está configurada para a empresa e as últimas execuções.
// POST {de, ate} (AAAA-MM): inicia a importação, que segue no servidor depois da
// resposta (202); o resultado aparece na lista.
func ImportacaoERPHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { importacaoERP(db, ERPPadrao.Load())(w, r) }
}

func importacaoERP(db *sql.DB, erp *dre.ImportacaoERP) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if !c.Pode(CapCarregar) {
			writeAuditLog(db, c.EmpresaID, c.UserID, "importacao_erp", "", "negado", map[string]any{"perfil": c.Perfil})
			http.Error(w, "Forbidden: perfil sem a capacidade de carregar dados", http.StatusForbidden)
			return
		}
		motivo := ""
		switch {
		case erp == nil:
			motivo = "a conexão com o banco do cliente não está configurada no servidor"
		case erp.EmpresaID != c.EmpresaID:
			motivo = "a conexão com o banco do cliente está configurada para outra empresa"
		}
		switch r.Method {
		case http.MethodGet:
			ex, err := dre.ListarExecucoes(r.Context(), db, c.EmpresaID, 20)
			if err != nil {
				log.Printf("importação erp: listar: %v", err)
				http.Error(w, "Erro ao listar as importações", http.StatusInternalServerError)
				return
			}
			obs := []string{}
			if motivo == "" {
				obs = append(obs, erp.Observacoes...)
			}
			writeJSON(w, map[string]any{"configurado": motivo == "", "motivo": motivo, "observacoes": obs,
				"max_meses": dre.MaxMesesImportacao, "execucoes": ex})
		case http.MethodPost:
			if motivo != "" {
				http.Error(w, motivo, http.StatusServiceUnavailable)
				return
			}
			var in struct{ De, Ate string }
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil {
				http.Error(w, "corpo inválido", http.StatusBadRequest)
				return
			}
			meses, err := dre.MesesDoPeriodo(in.De, in.Ate, time.Now())
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			p, err := erp.Iniciar(r.Context(), dre.EscopoAtualizacao{EmpresaID: c.EmpresaID, UserID: c.UserID}, "manual", meses)
			if errors.Is(err, dre.ErrImportacaoEmAndamento) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if err != nil {
				log.Printf("importação erp: iniciar: %v", err)
				http.Error(w, "Erro ao iniciar a importação", http.StatusInternalServerError)
				return
			}
			go func() {
				if _, err := erp.Executar(context.Background(), p); err != nil {
					log.Printf("importação erp %d: gravar resultado: %v", p.ID, err)
				}
			}()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{"execucao_id": p.ID})
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
