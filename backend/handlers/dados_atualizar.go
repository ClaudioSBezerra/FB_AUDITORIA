package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"fb_auditoria/dre"
)

// DadosAtualizarHandler — POST /api/aud/dados/atualizar: dispara as fontes
// atualizáveis registradas (realizado e orçado) e devolve o resultado e a hora.
// Só controladoria e admin de plataforma. Sem fonte registrada: 200 com status
// "sem_fonte" (nunca falso sucesso); competência com fechamento ativo: 409
// (guarda de período); falha de fonte: 502. O corpo é sempre dre.Atualizacao.
func DadosAtualizarHandler(db *sql.DB) http.HandlerFunc {
	return dadosAtualizar(db, dre.RegistroPadrao)
}

func dadosAtualizar(db *sql.DB, reg *dre.Registro) http.HandlerFunc {
	at := &dre.Atualizador{DB: db, Registro: reg}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !c.Tem("controladoria") {
			writeAuditLog(db, c.EmpresaID, c.UserID, "atualizacao_dados", "", "negado", map[string]any{"perfil": c.Perfil})
			http.Error(w, "Forbidden: só a controladoria atualiza os dados", http.StatusForbidden)
			return
		}
		res, err := at.Atualizar(r.Context(), dre.EscopoAtualizacao{EmpresaID: c.EmpresaID, UserID: c.UserID})
		if err != nil {
			log.Printf("atualizar dados: %v", err)
			http.Error(w, "Erro ao registrar a atualização na trilha", http.StatusInternalServerError)
			return
		}
		code := http.StatusOK
		switch res.Status {
		case dre.AtualizacaoRecusada:
			code = http.StatusConflict
		case dre.AtualizacaoErro:
			code = http.StatusBadGateway
		case dre.AtualizacaoParcial:
			code = http.StatusBadGateway
			for _, f := range res.Fontes {
				if f.Status == dre.AtualizacaoRecusada {
					code = http.StatusConflict
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(res)
	}
}
