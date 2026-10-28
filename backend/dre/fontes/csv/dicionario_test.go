package csv

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"fb_auditoria/dre"
)

func TestLerDicionario_CSV(t *testing.T) {
	casos := []struct {
		nome, csv string
		n         int
		probs     int
		entrada   bool
		primeiro  dre.ContaDic
	}{
		{nome: "ponto_e_virgula", csv: "conta;descrição;grupo\nC1;Conta um;G1\nC2;Conta dois;G2\n", n: 2,
			primeiro: dre.ContaDic{Linha: 2, Conta: "C1", Descricao: "Conta um", Grupo: "G1"}},
		{nome: "virgula_com_departamento", csv: "Conta,Descricao,Grupo,Departamento\nC1,x,G1,D1\n", n: 1,
			primeiro: dre.ContaDic{Linha: 2, Conta: "C1", Descricao: "x", Grupo: "G1", Departamento: "D1"}},
		{nome: "bom_e_titulo", csv: "\xef\xbb\xbfRelatório;;\nconta;grupo\nC1;7\n", n: 1,
			primeiro: dre.ContaDic{Linha: 3, Conta: "C1", Grupo: "7"}},
		{nome: "conta_vazia_e_longa", csv: "conta;grupo\n;G1\n" + strings.Repeat("x", 61) + ";G1\nC3;G1\n", n: 1, probs: 2,
			primeiro: dre.ContaDic{Linha: 4, Conta: "C3", Grupo: "G1"}},
		{nome: "colunas_diferentes", csv: "conta;grupo\nC1;G1;extra\nC2;G2\n", n: 1, probs: 1,
			primeiro: dre.ContaDic{Linha: 3, Conta: "C2", Grupo: "G2"}},
		{nome: "sem_grupo", csv: "conta;descricao\nC1;x\n", entrada: true},
		{nome: "vazio", csv: "", entrada: true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			out, probs, err := LerDicionario(strings.NewReader(c.csv))
			if c.entrada {
				var ee *dre.ErroEntrada
				if !errors.As(err, &ee) {
					t.Fatalf("esperava ErroEntrada: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != c.n || len(probs) != c.probs {
				t.Fatalf("linhas %d (esp. %d), problemas %d (esp. %d): %+v %+v", len(out), c.n, len(probs), c.probs, out, probs)
			}
			if out[0] != c.primeiro {
				t.Errorf("primeira linha: %+v, esperada %+v", out[0], c.primeiro)
			}
		})
	}
}

func TestLerDicionario_XLSX(t *testing.T) {
	x := excelize.NewFile()
	defer x.Close()
	s := x.GetSheetName(0)
	linhas := [][]any{
		{"Título qualquer"},
		{"CONTA", "DESCRIÇÃO", "Grupo", "Departamento"},
		{"C1", "Conta um", 7, 7}, // grupo e departamento numéricos
		{"C2", "Conta dois", "G2", ""},
	}
	for i, l := range linhas {
		cel, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := x.SetSheetRow(s, cel, &l); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := x.Write(&buf); err != nil {
		t.Fatal(err)
	}
	out, probs, err := LerDicionario(&buf)
	if err != nil || len(probs) != 0 || len(out) != 2 {
		t.Fatalf("%v %v %+v", err, probs, out)
	}
	if out[0].Grupo != "7" || out[0].Departamento != "7" || out[0].Descricao != "Conta um" || out[1].Grupo != "G2" {
		t.Errorf("valores: %+v", out)
	}
	if _, _, err := LerDicionario(strings.NewReader("PK\x03\x04lixo")); err == nil {
		t.Error("XLSX corrompido deveria falhar")
	}
}
