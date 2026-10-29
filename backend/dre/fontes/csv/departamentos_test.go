package csv

import (
	"errors"
	"strings"
	"testing"

	"fb_auditoria/dre"
)

func TestLerDepartamentos(t *testing.T) {
	out, probs, err := LerDepartamentos(strings.NewReader("Título;;\nCódigo;Nome;Grupo\n10;Vendas;comercial\nLOG;Logística;LOGISTICA\n;X;Comercial\nA;;Comercial\nB;Y;Outro\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0] != (dre.DepartamentoArquivo{Linha: 3, Codigo: "10", Nome: "Vendas", Grupo: "Comercial"}) ||
		out[1].Grupo != "Logística" || len(probs) != 3 {
		t.Fatalf("%+v %+v", out, probs)
	}
	var ee *dre.ErroEntrada
	if _, _, err := LerDepartamentos(strings.NewReader("codigo;nome\nA;B\n")); !errors.As(err, &ee) {
		t.Errorf("sem coluna grupo: %v", err)
	}
	if _, _, err := LerDepartamentos(strings.NewReader("")); !errors.As(err, &ee) {
		t.Errorf("vazio: %v", err)
	}
}
