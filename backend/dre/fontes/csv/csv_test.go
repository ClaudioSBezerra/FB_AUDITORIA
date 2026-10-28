package csv

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"fb_auditoria/dre"
)

func ler(conteudo string) ([]dre.Lancamento, error) {
	return Nova(strings.NewReader(conteudo), "t.csv").Ler(context.Background(), dre.Escopo{})
}

func TestLer(t *testing.T) {
	casos := []struct {
		nome, csv string
		n         int
		entrada   string // mensagem de ErroEntrada esperada
		linhas    []int  // linhas com problema esperadas
	}{
		{nome: "ponto_e_virgula", csv: "competencia;filial;conta;valor\n2026-09;01;3.1;1.234,56\n", n: 1},
		{nome: "virgula", csv: "competencia,filial,conta,valor\n2026-09,01,3.1,1234.56\n", n: 1},
		{nome: "bom", csv: "\xef\xbb\xbfcompetencia;filial;conta;valor\n2026-09;01;3.1;1,00\n", n: 1},
		{nome: "mm_aaaa", csv: "competencia;filial;conta;valor\n09/2026;01;3.1;1,00\n", n: 1},
		{nome: "data_completa", csv: "competencia;filial;conta;valor\n2026-09-01;01;3.1;1,00\n", n: 1},
		{nome: "sem_filial", csv: "competencia;conta;valor\n2026-09;3.1;1,00\n", entrada: "coluna obrigatória ausente: filial"},
		{nome: "sem_conta", csv: "competencia;filial;valor\n2026-09;01;1,00\n", entrada: "coluna obrigatória ausente: conta"},
		{nome: "competencia_invalida", csv: "competencia;filial;conta;valor\n2026-09;01;3.1;1,00\nxx;01;3.1;1,00\n", n: 1, linhas: []int{3}},
		{nome: "nan", csv: "competencia;filial;conta;valor\n2026-09;01;3.1;NaN\n", linhas: []int{2}},
		{nome: "inf", csv: "competencia;filial;conta;valor\n2026-09;01;3.1;Inf\n", linhas: []int{2}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			out, err := ler(c.csv)
			if c.entrada != "" {
				var ee *dre.ErroEntrada
				if !errors.As(err, &ee) || ee.Msg != c.entrada {
					t.Fatalf("esperava ErroEntrada %q: %v", c.entrada, err)
				}
				return
			}
			if c.linhas != nil {
				var el *dre.ErroLinhas
				if !errors.As(err, &el) || len(el.Problemas) != len(c.linhas) {
					t.Fatalf("esperava problemas nas linhas %v: %v", c.linhas, err)
				}
				for i, l := range c.linhas {
					if el.Problemas[i].Linha != l {
						t.Errorf("problema %d: linha %d, esperada %d", i, el.Problemas[i].Linha, l)
					}
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(out) != c.n {
				t.Errorf("linhas válidas: %d, esperado %d", len(out), c.n)
			}
		})
	}
}

const cabPlanilha = "CONTA;DESCRIÇÃO;VL. Realizado;Ajustes;Saldo;Grupo;Mês;Loja\n"

func lerFonte(t *testing.T, conteudo []byte) (*Fonte, []dre.Lancamento, error) {
	t.Helper()
	f := Nova(bytes.NewReader(conteudo), "t")
	out, err := f.Ler(context.Background(), dre.Escopo{})
	return f, out, err
}

func xlsx(t *testing.T, linhas [][]any) []byte {
	t.Helper()
	x := excelize.NewFile()
	for i, l := range linhas {
		cel, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := x.SetSheetRow("Sheet1", cel, &l); err != nil {
			t.Fatal(err)
		}
	}
	var b bytes.Buffer
	if err := x.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestPlanilhaCSV(t *testing.T) {
	c := "titulo\n\nrelatorio ficticio\n" + cabPlanilha +
		"4010001;Venda;1.234,56;10,00;0;G1;202609;01\n" +
		"4010002;Custo;-50,5;0;0;G1;202609;1\n" +
		";;;;;;;\n" +
		"4010003;Outra;5;3;0;G1;202610;11\n"
	f, out, err := lerFonte(t, []byte(c))
	if err != nil {
		t.Fatal(err)
	}
	if f.Nome() != "csv.contas" || len(out) != 3 || f.AjustesIgnorados() != 2 {
		t.Fatalf("nome=%s n=%d ajustes=%d", f.Nome(), len(out), f.AjustesIgnorados())
	}
	if out[0].Filial != "1" || out[1].Filial != "1" || out[2].Filial != "11" || out[0].Valor != 1234.56 ||
		out[0].ContaDesc != "Venda" || out[0].Competencia.Format("2006-01") != "2026-09" || out[0].Linha != 5 {
		t.Fatalf("lançamentos inesperados: %+v", out)
	}
}

func TestPlanilhaXLSX(t *testing.T) {
	b := xlsx(t, [][]any{
		{"titulo"}, {},
		{"CONTA", "DESCRIÇÃO", "VL. Realizado", "Ajustes", "Saldo", "Grupo", "Mês", "Loja"},
		{"4010001", "Venda", 1234.56, 7, 0, "G1", 202609, 11},
		{"4010002", "Custo", "1.000,00", 0, 0, "G1", "202609", "01"},
	})
	f, out, err := lerFonte(t, b)
	if err != nil {
		t.Fatal(err)
	}
	if f.Nome() != "csv.contas" || len(out) != 2 || f.AjustesIgnorados() != 1 {
		t.Fatalf("nome=%s n=%d ajustes=%d", f.Nome(), len(out), f.AjustesIgnorados())
	}
	if out[0].Filial != "11" || out[0].Valor != 1234.56 || out[1].Filial != "1" || out[1].Valor != 1000 ||
		out[0].Competencia.Format("2006-01") != "2026-09" || out[0].Linha != 4 {
		t.Fatalf("lançamentos inesperados: %+v", out)
	}
}

func TestSistemaEmXLSX(t *testing.T) {
	b := xlsx(t, [][]any{{"competencia", "filial", "conta", "valor"}, {"2026-09", "01", "3.1", 10.5}})
	f, out, err := lerFonte(t, b)
	if err != nil || f.Nome() != "csv" || len(out) != 1 || out[0].Filial != "01" {
		t.Fatalf("nome=%s out=%+v err=%v", f.Nome(), out, err)
	}
}

func TestPlanilhaInvalida(t *testing.T) {
	casos := []struct {
		nome, csv string
		linhas    []int
	}{
		{"mes_aaaa_mm", cabPlanilha + "1;D;1;0;0;G;2026-09;1\n", []int{2}},
		{"mes_mm_aaaa", cabPlanilha + "1;D;1;0;0;G;13/2026;1\n", []int{2}},
		{"mes_13", cabPlanilha + "1;D;1;0;0;G;202613;1\n", []int{2}},
		{"loja_vazia", cabPlanilha + "1;D;1;0;0;G;202609;\n", []int{2}},
		{"nan", cabPlanilha + "1;D;NaN;0;0;G;202609;1\n", []int{2}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, _, err := lerFonte(t, []byte(c.csv))
			var el *dre.ErroLinhas
			if !errors.As(err, &el) || len(el.Problemas) != len(c.linhas) || el.Problemas[0].Linha != c.linhas[0] {
				t.Fatalf("esperava problema na linha %v: %v", c.linhas, err)
			}
		})
	}
}

func TestCabecalhoNaoReconhecidoEZipCorrompido(t *testing.T) {
	_, _, err := lerFonte(t, []byte("a;b;c\n1;2;3\n"))
	var ee *dre.ErroEntrada
	if !errors.As(err, &ee) || !strings.Contains(ee.Msg, "esperado") {
		t.Fatalf("esperava ErroEntrada listando as colunas: %v", err)
	}
	b := xlsx(t, [][]any{{"CONTA", "VL. Realizado", "Mês", "Loja"}, {"1", 1, 202609, 1}})
	_, _, err = lerFonte(t, b[:len(b)/2])
	if !errors.As(err, &ee) || !strings.Contains(ee.Msg, "XLSX") {
		t.Fatalf("esperava ErroEntrada de XLSX corrompido: %v", err)
	}
	_, _, err = lerFonte(t, []byte("CONTA;VL. Realizado;Mês\n1;1;202609\n"))
	if !errors.As(err, &ee) || !strings.Contains(ee.Msg, "esperado") {
		t.Fatalf("esperava coluna loja ausente: %v", err)
	}
}

func TestXLSXValorBrutoEFormatado(t *testing.T) {
	x := excelize.NewFile()
	linhas := [][]any{
		{"CONTA", "VL. Realizado", "Mês", "Loja"},
		{"1", 1234.567, 202609, 1},
		{"2", 1235, 202609, 1},
		{"3", "1.234,56", "202609", "01"},
	}
	for i, l := range linhas {
		cel, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := x.SetSheetRow("Sheet1", cel, &l); err != nil {
			t.Fatal(err)
		}
	}
	dois, _ := x.NewStyle(&excelize.Style{NumFmt: 2})   // 0.00
	milhar, _ := x.NewStyle(&excelize.Style{NumFmt: 3}) // #,##0
	x.SetCellStyle("Sheet1", "B2", "B2", dois)
	x.SetCellStyle("Sheet1", "B3", "B3", milhar)
	var b bytes.Buffer
	if err := x.Write(&b); err != nil {
		t.Fatal(err)
	}
	_, out, err := lerFonte(t, b.Bytes())
	if err != nil || len(out) != 3 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if out[0].Valor != 1234.567 || out[1].Valor != 1235 || out[2].Valor != 1234.56 || out[2].Filial != "1" {
		t.Fatalf("valores brutos esperados: %+v", out)
	}
}

func TestXLSXSemCabecalhoEMultiplasPlanilhas(t *testing.T) {
	var ee *dre.ErroEntrada
	b := xlsx(t, [][]any{{"a", "b"}, {1, 2}})
	if _, _, err := lerFonte(t, b); !errors.As(err, &ee) || !strings.Contains(ee.Msg, "cabeçalho não reconhecido; esperado") {
		t.Fatalf("esperava cabeçalho não reconhecido: %v", err)
	}
	// cabeçalho só na segunda planilha
	x := excelize.NewFile()
	x.SetSheetRow("Sheet1", "A1", &[]any{"nada"})
	x.NewSheet("Dados")
	x.SetSheetRow("Dados", "A1", &[]any{"CONTA", "VL. Realizado", "Mês", "Loja"})
	x.SetSheetRow("Dados", "A2", &[]any{"1", 5, 202609, 2})
	var buf bytes.Buffer
	x.Write(&buf)
	if _, out, err := lerFonte(t, buf.Bytes()); err != nil || len(out) != 1 || out[0].Filial != "2" {
		t.Fatalf("segunda planilha: %+v %v", out, err)
	}
	// cabeçalho depois da linha 20
	var ls [][]any
	for i := 0; i < 21; i++ {
		ls = append(ls, []any{"titulo"})
	}
	ls = append(ls, []any{"CONTA", "VL. Realizado", "Mês", "Loja"}, []any{"1", 5, 202609, 2})
	if _, _, err := lerFonte(t, xlsx(t, ls)); !errors.As(err, &ee) || !strings.Contains(ee.Msg, "esperado") {
		t.Fatalf("cabeçalho após a linha 20 devia ser rejeitado: %v", err)
	}
}

func TestTituloNaoViraCabecalhoESeparadorPelaLinhaDoCabecalho(t *testing.T) {
	c := "Relatório, de \"teste, solto\nlinha competencia, aqui\nCONTA,DESCRIÇÃO,VL. Realizado,Ajustes,Saldo,Grupo,Mês,Loja\n" +
		"1,Venda,\"1.234,56\",0,0,G,202609,01\n"
	f, out, err := lerFonte(t, []byte(c))
	if err != nil || f.Nome() != "csv.contas" || len(out) != 1 || out[0].Valor != 1234.56 || out[0].Linha != 4 {
		t.Fatalf("nome=%s out=%+v err=%v", f.Nome(), out, err)
	}
}

func TestWindows1252(t *testing.T) {
	c := "CONTA;DESCRI\xc7\xc3O;VL. Realizado;M\xeas;Loja\n1;Venda \x96 teste;1,00;202609;1\n"
	_, out, err := lerFonte(t, []byte(c))
	if err != nil || len(out) != 1 || out[0].ContaDesc != "Venda \u2013 teste" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestPlanilhaColunasDeslocadas(t *testing.T) {
	c := cabPlanilha + "1;Venda; extra;1,00;0;0;G;202609;1\n"
	_, _, err := lerFonte(t, []byte(c))
	var el *dre.ErroLinhas
	if !errors.As(err, &el) || el.Problemas[0].Linha != 2 {
		t.Fatalf("esperava problema na linha 2: %v", err)
	}
}

func TestCabecalhoDuplicadoEReuso(t *testing.T) {
	c := "CONTA;VL. Realizado;Mês;Loja;Loja;;\n1;5;202609;3;9;;\n"
	f := Nova(strings.NewReader(c), "t")
	out, err := f.Ler(context.Background(), dre.Escopo{})
	if err != nil || len(out) != 1 || out[0].Filial != "3" {
		t.Fatalf("o primeiro nome duplicado devia vencer: %+v %v", out, err)
	}
	// um CSV iniciado por "PK" não é XLSX
	if _, _, err := lerFonte(t, []byte("PK;x\n")); err == nil || strings.Contains(err.Error(), "XLSX") {
		t.Fatalf("CSV com PK não deve ser tratado como XLSX: %v", err)
	}
}
