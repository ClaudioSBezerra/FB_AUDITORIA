package dre_test

import (
	"context"
	"testing"
	"time"

	"fb_auditoria/dre"
	"fb_auditoria/dre/fontes/oracle"
	"fb_auditoria/internal/testdb"
)

func TestDetalheDoLancamento_GravaELeParceiroENota(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	pago := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	l := linhaReal("11", "4010001", -10)
	l.ERPID, l.Nota, l.Data, l.DataLancamento = "900001", "123", pago, pago.AddDate(0, 0, -7)
	l.ParceiroTipo, l.ParceiroCod, l.Parceiro = "F", "55", "Fornecedor Fictício"
	i := novaImportacao(t, f, &erpFalso{real: map[string][]oracle.LinhaRealizado{"2026-09": {l}}})
	importarERP(t, i, f, "2026-09", "2026-09")

	per := dre.Periodo{Ini: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Fim: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	det, err := (dre.Leitor{Q: db}).DetalheConta(context.Background(), f.EmpresaID, []string{dre.FilialGrupo}, per, "4010001",
		[]dre.MapaItem{{ContaPadrao: "4010001", LinhaCodigo: "X", Sinal: 1}}, 10, 0)
	if err != nil || len(det.Lancamentos) != 1 {
		t.Fatalf("%+v %v", det, err)
	}
	x := det.Lancamentos[0]
	if x.Parceiro != "Fornecedor Fictício" || x.DocRef != "123" || x.TipoDoc != "NF" || x.ERPID != "900001" ||
		x.Data != "03/09/2026" || x.DataLancamento != "27/08/2026" {
		t.Errorf("%+v", x)
	}
}

func TestRestringirParceiro(t *testing.T) {
	visiveis := map[string]bool{"F": true}
	for _, c := range []struct {
		tipo, nome string
		restrito   bool
	}{{"F", "Empresa", false}, {"R", "Pessoa", true}, {"", "Sem tipo", true}, {"", "", false}} {
		l := dre.LancamentoConta{ParceiroTipo: c.tipo, Parceiro: c.nome, Historico: "h"}
		l.RestringirParceiro(visiveis)
		if l.Restrito != c.restrito || (c.restrito && (l.Parceiro != "" || l.Historico != "")) {
			t.Errorf("tipo %q nome %q: %+v", c.tipo, c.nome, l)
		}
	}
}
