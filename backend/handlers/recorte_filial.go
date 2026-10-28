package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"

	"fb_auditoria/dre"
)

// recorteFilial é o recorte de filiais do ERP que um endpoint do DRE soma.
type recorteFilial struct {
	Filiais []string       // filiais do ERP; {""} = todas (FilialGrupo)
	Rotulo  map[string]any // vai para a trilha de visualização
}

// resolverRecorte lê ?filial=, ?agrupamento=<codigo> e ?grupo=1 (no máximo um
// deles), resolve o agrupamento pelo mapa de filiais e responde o erro HTTP
// quando falha (devolve ok=false). Sem nenhum parâmetro vale "todas".
func resolverRecorte(w http.ResponseWriter, r *http.Request, db *sql.DB, empresaID string) (recorteFilial, bool) {
	q := r.URL.Query()
	filial, agr, grupo := q.Get("filial"), q.Get("agrupamento"), q.Get("grupo")
	n := 0
	for _, s := range []string{filial, agr} {
		if s != "" {
			n++
		}
	}
	if grupo != "" {
		if grupo != "1" {
			http.Error(w, "grupo: use grupo=1", http.StatusBadRequest)
			return recorteFilial{}, false
		}
		n++
	}
	if n > 1 {
		http.Error(w, "use apenas um entre filial, agrupamento e grupo", http.StatusBadRequest)
		return recorteFilial{}, false
	}
	switch {
	case agr != "":
		alvo, err := dre.FiliaisDoAgrupamento(r.Context(), db, empresaID, agr)
		var nf *dre.ErrAgrupamentoNaoEncontrado
		switch {
		case errors.As(err, &nf):
			http.Error(w, nf.Error(), http.StatusNotFound)
			return recorteFilial{}, false
		case err != nil:
			log.Printf("dre agrupamento: %v", err)
			http.Error(w, "Erro ao resolver o agrupamento", http.StatusInternalServerError)
			return recorteFilial{}, false
		case len(alvo.ERP) == 0:
			http.Error(w, (&dre.ErrAgrupamentoVazio{Codigo: agr}).Error(), http.StatusConflict)
			return recorteFilial{}, false
		}
		return recorteFilial{Filiais: alvo.ERP, Rotulo: map[string]any{"agrupamento": agr}}, true
	case grupo != "":
		return recorteFilial{Filiais: []string{dre.FilialGrupo}, Rotulo: map[string]any{"grupo": true}}, true
	}
	return recorteFilial{Filiais: []string{filial}, Rotulo: map[string]any{"filial": filial}}, true
}

func (r recorteFilial) comVisao(visao string) map[string]any {
	out := map[string]any{"visao": visao}
	for k, v := range r.Rotulo {
		out[k] = v
	}
	return out
}
