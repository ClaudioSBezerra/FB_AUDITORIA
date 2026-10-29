package csv

import (
	"bytes"
	"context"
	"testing"

	"github.com/xuri/excelize/v2"

	"fb_auditoria/dre"
)

// A aba Entrada_Dados vale mesmo quando outra aba, antes dela, também tem cabeçalho.
func TestXLSXPrefereAbaEntradaDados(t *testing.T) {
	x := excelize.NewFile()
	x.SetSheetName("Sheet1", "Outra")
	cab := []any{"CONTA", "DESCRIÇÃO", "VL. Realizado", "Mês", "Loja"}
	_ = x.SetSheetRow("Outra", "A1", &cab)
	r := []any{"9990001", "conta da outra aba", 1.0, 202601, 1}
	_ = x.SetSheetRow("Outra", "A2", &r)
	_, _ = x.NewSheet("Entrada_Dados")
	_ = x.SetSheetRow("Entrada_Dados", "A1", &cab)
	r2 := []any{"9990002", "conta da entrada", 2.0, 202601, 1}
	_ = x.SetSheetRow("Entrada_Dados", "A2", &r2)
	var buf bytes.Buffer
	if err := x.Write(&buf); err != nil {
		t.Fatal(err)
	}
	ls, err := Nova(&buf, "x.xlsx").Ler(context.Background(), dre.Escopo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 1 || ls[0].Conta != "9990002" {
		t.Fatalf("esperava só a conta da aba Entrada_Dados, veio %+v", ls)
	}
}
