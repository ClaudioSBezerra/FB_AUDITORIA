package dre_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"fb_auditoria/dre"
	csvfonte "fb_auditoria/dre/fontes/csv"
	"fb_auditoria/internal/testdb"
)

func codigosAviso(r dre.Resumo) map[string]bool {
	m := map[string]bool{}
	for _, a := range r.Avisos {
		m[a.Codigo] = true
	}
	return m
}

// Avisos de competência antiga e futura, com a data de hoje fixada.
func TestSimular_CompetenciaForaDaJanela(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ing := &dre.Ingestor{DB: db, Agora: func() time.Time { return time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC) }}
	casos := []struct {
		comp           string
		antiga, futura bool
	}{
		{"2026-10", false, false}, // mês corrente
		{"2025-10", false, false}, // 12 meses atrás
		{"2025-09", true, false},  // 13 meses atrás
		{"2026-11", false, true},  // mês seguinte
	}
	for _, c := range casos {
		csv := "competencia;filial;conta;valor\n" + c.comp + ";01;3.1;1,00\n"
		r, err := ing.Simular(context.Background(), f.EmpresaID, csvfonte.Nova(strings.NewReader(csv), "t.csv"))
		if err != nil {
			t.Fatal(err)
		}
		m := codigosAviso(r)
		if m[dre.AvisoCompetenciaAntiga] != c.antiga || m[dre.AvisoCompetenciaFutura] != c.futura {
			t.Errorf("%s: avisos %+v", c.comp, r.Avisos)
		}
	}
	if n := contar(t, db, total, f.EmpresaID); n != 0 {
		t.Errorf("a simulação gravou %d", n)
	}
}
