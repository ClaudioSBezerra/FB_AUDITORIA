package acompanhamento

import (
	"os"
	"strings"
	"testing"
)

const raizPlanejamento = "../../_bmad-output"

// Falha se o dados.json versionado não corresponder a epics.md e sprint-status.yaml.
// Pula quando _bmad-output não existe (por exemplo, no build do container).
func TestDadosJSONEstaAtualizado(t *testing.T) {
	if _, err := os.Stat(raizPlanejamento); err != nil {
		t.Skip("_bmad-output ausente; nada a conferir")
	}
	novo, err := Gerar(raizPlanejamento)
	if err != nil {
		t.Fatal(err)
	}
	atual, err := Embutido()
	if err != nil {
		t.Fatal(err)
	}
	if !MesmasHistorias(atual, novo) {
		t.Fatalf("backend/acompanhamento/dados.json está desatualizado em relação a epics.md/sprint-status.yaml.\n" +
			"Regenere com: cd backend && go run ./cmd/gerar-acompanhamento   (e inclua o dados.json no commit)")
	}
}

func TestDadosEmbutidosValidos(t *testing.T) {
	d, err := Embutido()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Historias) == 0 {
		t.Fatal("sem histórias")
	}
	colunas := map[string]bool{ColBacklog: true, ColPronto: true, ColEmAndamento: true, ColEmRevisao: true, ColFeito: true}
	ids := map[string]bool{}
	for _, h := range d.Historias {
		if ids[h.ID] {
			t.Errorf("id repetido %s", h.ID)
		}
		ids[h.ID] = true
		if !colunas[h.Status] {
			t.Errorf("%s: status %q fora das colunas", h.ID, h.Status)
		}
		if h.Titulo == "" || h.Epico.Nome == "" {
			t.Errorf("%s sem título ou épico", h.ID)
		}
		if h.BloqueadaCliente && h.MotivoBloqueio == "" {
			t.Errorf("%s bloqueada sem motivo", h.ID)
		}
	}
	for _, descartada := range []string{"5.5", "5.7"} {
		if ids[descartada] {
			t.Errorf("história descartada %s não deve entrar", descartada)
		}
	}
	// Notas e arquivos do cliente nunca entram.
	if b := string(JSON()); strings.Contains(b, "respostas-cliente") || strings.Contains(b, "deferred-work") {
		t.Error("dados.json cita arquivos do cliente")
	}
}
