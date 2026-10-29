package csv

import (
	"errors"
	"strings"
	"testing"

	"fb_auditoria/dre"
)

func TestLerMapaFiliais(t *testing.T) {
	out, probs, err := LerMapaFiliais(strings.NewReader("Mapa;;\nFilial Gerencial;Filial ERP;Nome\n011;10;Matriz\n11;11;\n;12;\n20;;\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0] != (dre.FilialMapaArquivo{Linha: 3, Gerencial: "11", Nome: "Matriz", FilialERP: "10"}) ||
		out[1].FilialERP != "11" || len(probs) != 2 {
		t.Fatalf("%+v %+v", out, probs)
	}
	var ee *dre.ErroEntrada
	if _, _, err := LerMapaFiliais(strings.NewReader("filial_gerencial;nome\n1;A\n")); !errors.As(err, &ee) {
		t.Errorf("sem filial_erp: %v", err)
	}
	if _, _, err := LerMapaFiliais(strings.NewReader("")); !errors.As(err, &ee) {
		t.Errorf("vazio: %v", err)
	}
}

func TestLerAgrupamentos(t *testing.T) {
	out, probs, err := LerAgrupamentos(strings.NewReader("Agrupamento;Filial;Nome\nBAL;01;Balcões\nBAL;12;\n;1;\nDIS;;\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0] != (dre.MembroAgrupamentoArquivo{Linha: 2, Codigo: "BAL", Nome: "Balcões", Filial: "1"}) || len(probs) != 2 {
		t.Fatalf("%+v %+v", out, probs)
	}
	var ee *dre.ErroEntrada
	if _, _, err := LerAgrupamentos(strings.NewReader("agrupamento;nome\nA;B\n")); !errors.As(err, &ee) {
		t.Errorf("sem filial: %v", err)
	}
}
