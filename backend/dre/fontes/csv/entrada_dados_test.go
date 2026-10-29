package csv

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"fb_auditoria/dre"
)

// Fixture fictícia com a ESTRUTURA da aba Entrada_Dados: tabela 1 (resumo) com
// subtotais sem código, linha em branco, segundo cabeçalho, contas, TOTAL e
// linhas de prova sem Mês e Loja. Nenhum valor ou código é real.

type opcEntrada struct {
	totalExtra  float64 // somado ao TOTAL para forçar divergência
	semMesLoja  bool    // XLSX: fórmulas sem valor calculado
	ajusteConta float64 // ajuste numa conta
	lojaVazia   bool    // uma conta com Mês e sem Loja
}

const (
	mesRef    = 202602
	lojaRef   = 7
	cabTab1   = "Código;Conta;Vl. Realizado;Ajuste;Saldo;Grupo;Mês;Loja"
	cabTab2Pd = "CONTA;DESCRIÇÃO;VL. Realizado;Ajuste;Saldo;Grupo;Mês;Loja"
)

type lin struct {
	a, b       any // código / descrição
	c          float64
	ajuste     any
	mesLoja    bool // tem Mês e Loja
	ehPlaceBlk bool
}

func linhasEntrada(o opcEntrada) []lin {
	l := []lin{
		{a: "1", b: "Venda bruta ficticia", c: 1000.50, mesLoja: true},
		{a: "2", b: "Devolucao ficticia", c: 100, mesLoja: true},
		{a: "2", b: "Devolucao avulsa ficticia", c: 20, mesLoja: true},
		{a: "OUT", b: "Outros ficticio", c: 30, mesLoja: true},
		{a: "", b: "Faturamento Liquido", c: 910.50, mesLoja: true},
		{a: "3", b: "CMV devolucao ficticio", c: 5, mesLoja: true},
		{a: "3", b: "CMV devolucao avulsa ficticio", c: 1, mesLoja: true},
		{a: "3", b: "CMV ficticio", c: 400, mesLoja: true},
		{a: "", b: "Saldo Bruto", c: 515.50, mesLoja: true},
		{ehPlaceBlk: true},
	}
	return l
}

func contasEntrada(o opcEntrada) []lin {
	cs := []lin{
		{a: 4010001, b: "Conta A ficticia", c: 100.10, mesLoja: true, ajuste: o.ajusteConta},
		{a: 4010002, b: "Conta B ficticia", c: 200.20, mesLoja: true},
		{a: "4010003.0", b: "Conta C ficticia", c: -50.05, mesLoja: true},
	}
	return cs
}

// celulas monta a linha de 8 colunas (A..H); mes/loja como valores.
func (l lin) celulas() []any {
	var aj any = ""
	if l.ajuste != nil {
		aj = l.ajuste
	}
	r := []any{l.a, l.b, l.c, aj, "", "G", "", ""}
	if l.mesLoja {
		r[6], r[7] = mesRef, lojaRef
	}
	return r
}

func entradaXLSX(t *testing.T, o opcEntrada) []byte {
	t.Helper()
	x := excelize.NewFile()
	sh := "Sheet1"
	put := func(row int, vals []any) {
		cel, _ := excelize.CoordinatesToCellName(1, row)
		if err := x.SetSheetRow(sh, cel, &vals); err != nil {
			t.Fatal(err)
		}
	}
	formulas := func(row int) {
		g, _ := excelize.CoordinatesToCellName(7, row)
		h, _ := excelize.CoordinatesToCellName(8, row)
		if o.semMesLoja { // fórmula sem valor guardado
			x.SetCellValue(sh, g, nil)
			x.SetCellValue(sh, h, nil)
		}
		x.SetCellFormula(sh, g, "$I$2")
		x.SetCellFormula(sh, h, "$J$2")
	}
	put(1, []any{"Código", "Conta", "Vl. Realizado", "Ajuste", "Saldo", "Grupo", "Mês", "Loja", "MÊS REFERENCIA", "FILIAL"})
	row := 2
	for _, l := range linhasEntrada(o) {
		if l.ehPlaceBlk {
			row++
			continue
		}
		put(row, l.celulas())
		formulas(row)
		row++
	}
	x.SetCellValue(sh, "I2", mesRef)
	x.SetCellValue(sh, "J2", lojaRef)
	put(row, []any{"CONTA", "DESCRIÇÃO", "VL. Realizado", "Ajuste", "Saldo", "Grupo", "Mês", "Loja"})
	row++
	soma := 0.0
	for i, l := range contasEntrada(o) {
		c := l.celulas()
		if o.lojaVazia && i == 1 {
			c[7] = ""
		}
		put(row, c)
		if !(o.lojaVazia && i == 1) {
			formulas(row)
		}
		soma += l.c
		row++
	}
	put(row, []any{"TOTAL", "", soma + o.totalExtra})
	row++
	put(row, []any{"Calculo para prova real ficticio", "", 12.5})
	row++
	put(row, []any{"Diferença (tem que ser zero)", "", 0})
	var b bytes.Buffer
	if err := x.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func entradaCSV(o opcEntrada) []byte {
	var sb strings.Builder
	cel := func(v any) string {
		if f, ok := v.(float64); ok {
			return strings.ReplaceAll(fmt.Sprintf("%.2f", f), ".", ",")
		}
		return fmt.Sprint(v)
	}
	w := func(vals ...any) {
		for i, v := range vals {
			if i > 0 {
				sb.WriteString(";")
			}
			sb.WriteString(cel(v))
		}
		sb.WriteString("\n")
	}
	sb.WriteString(cabTab1 + "\n")
	for _, l := range linhasEntrada(o) {
		if l.ehPlaceBlk {
			sb.WriteString(";;;;;;;\n")
			continue
		}
		w(l.celulas()...)
	}
	sb.WriteString(cabTab2Pd + "\n")
	soma := 0.0
	for i, l := range contasEntrada(o) {
		c := l.celulas()
		if o.lojaVazia && i == 1 {
			c[7] = ""
		}
		w(c...)
		soma += l.c
	}
	w("TOTAL", "", soma+o.totalExtra, "", "", "", "", "")
	w("Calculo para prova real ficticio", "", 12.5, "", "", "", "", "")
	w("Diferença (tem que ser zero)", "", 0.0, "", "", "", "", "")
	return []byte(sb.String())
}

func verificarEntrada(t *testing.T, f *Fonte, out []dre.Lancamento, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if f.Nome() != "csv.contas" || len(out) != 10 || f.LinhasIgnoradas() != 5 {
		t.Fatalf("nome=%s n=%d ignoradas=%d", f.Nome(), len(out), f.LinhasIgnoradas())
	}
	for _, l := range out {
		if l.Filial != "7" || l.Competencia.Format("2006-01") != "2026-02" {
			t.Fatalf("mês/loja inesperados: %+v", l)
		}
	}
	// tabela 1: Código é a conta e Conta é a descrição
	if out[0].Conta != "1" || out[0].ContaDesc != "Venda bruta ficticia" || out[3].Conta != "OUT" {
		t.Fatalf("tabela 1: %+v", out[:4])
	}
	// tabela 2: CONTA é o código; código numérico de célula e "123.0" viram 123
	if out[7].Conta != "4010001" || out[7].ContaDesc != "Conta A ficticia" || out[9].Conta != "4010003" {
		t.Fatalf("tabela 2: %+v", out[7:])
	}
	if f.Conferencia() == nil {
		t.Fatal("esperava conferência do TOTAL")
	}
}

func TestEntradaDadosXLSX(t *testing.T) {
	f, out, err := lerFonte(t, entradaXLSX(t, opcEntrada{}))
	verificarEntrada(t, f, out, err)
	if c := f.Conferencia(); !c.OK || c.Esperado != 250.25 || c.Calculado != 250.25 {
		t.Fatalf("conferência: %+v", c)
	}
}

func TestEntradaDadosCSV(t *testing.T) {
	f, out, err := lerFonte(t, entradaCSV(opcEntrada{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 10 || f.LinhasIgnoradas() != 5 || out[0].Conta != "1" || out[0].ContaDesc != "Venda bruta ficticia" {
		t.Fatalf("n=%d ignoradas=%d out=%+v", len(out), f.LinhasIgnoradas(), out[:1])
	}
	if c := f.Conferencia(); c == nil || !c.OK {
		t.Fatalf("conferência: %+v", c)
	}
	// o segundo cabeçalho nunca vira dado
	for _, l := range out {
		if l.Conta == "CONTA" || l.Conta == "Código" {
			t.Fatalf("cabeçalho virou dado: %+v", l)
		}
	}
}

func TestEntradaDadosTotalDivergeEAviso(t *testing.T) {
	for nome, b := range map[string][]byte{"xlsx": entradaXLSX(t, opcEntrada{totalExtra: 5}), "csv": entradaCSV(opcEntrada{totalExtra: 5})} {
		t.Run(nome, func(t *testing.T) {
			f, out, err := lerFonte(t, b)
			if err != nil || len(out) != 10 {
				t.Fatalf("divergência não bloqueia: n=%d err=%v", len(out), err)
			}
			if c := f.Conferencia(); c == nil || c.OK || c.Esperado != 255.25 || c.Calculado != 250.25 {
				t.Fatalf("conferência: %+v", c)
			}
		})
	}
}

func TestEntradaDadosAjusteSingular(t *testing.T) {
	for nome, b := range map[string][]byte{"xlsx": entradaXLSX(t, opcEntrada{ajusteConta: 3}), "csv": entradaCSV(opcEntrada{ajusteConta: 3})} {
		t.Run(nome, func(t *testing.T) {
			f, out, err := lerFonte(t, b)
			if err != nil || len(out) != 10 || f.AjustesIgnorados() != 1 {
				t.Fatalf("n=%d ajustes=%d err=%v", len(out), f.AjustesIgnorados(), err)
			}
			if out[7].Valor != 100.10 { // o ajuste não entra no valor
				t.Fatalf("valor: %v", out[7].Valor)
			}
		})
	}
}

func TestEntradaDadosXLSXSemFormulaCalculada(t *testing.T) {
	_, _, err := lerFonte(t, entradaXLSX(t, opcEntrada{semMesLoja: true}))
	var ee *dre.ErroEntrada
	if !errors.As(err, &ee) || !strings.Contains(ee.Msg, "fórmulas não têm valor calculado") || !strings.Contains(ee.Msg, "salve de novo") {
		t.Fatalf("esperava a mensagem das fórmulas sem valor: %v", err)
	}
}

func TestEntradaDadosLojaVazia(t *testing.T) {
	for nome, b := range map[string][]byte{"xlsx": entradaXLSX(t, opcEntrada{lojaVazia: true}), "csv": entradaCSV(opcEntrada{lojaVazia: true})} {
		t.Run(nome, func(t *testing.T) {
			_, _, err := lerFonte(t, b)
			var el *dre.ErroLinhas
			if !errors.As(err, &el) || len(el.Problemas) != 1 || el.Problemas[0].Linha != 14 || el.Problemas[0].Msg != "Loja vazia" {
				t.Fatalf("esperava Loja vazia na linha 14: %v", err)
			}
		})
	}
}

func TestEntradaDadosValorNaoNumericoComCodigo(t *testing.T) {
	c := cabTab1 + "\n1;Venda;abc;;;G;202602;7\n"
	_, _, err := lerFonte(t, []byte(c))
	var el *dre.ErroLinhas
	if !errors.As(err, &el) || el.Problemas[0].Linha != 2 {
		t.Fatalf("valor não numérico com Mês e Loja é erro de linha: %v", err)
	}
	// código e valor sem Mês nem Loja é conferência; só um dos dois é erro
	c = cabTab1 + "\n1;Venda;5,00;;;G;202602;7\nTOTAL;;5,00;;;;;\nx;prova;3,00;;;;;\ny;prova;3,00;;;G;202602;\nz;prova;3,00;;;G;;7\n"
	f, out, err := lerFonte(t, []byte(c))
	var el2 *dre.ErroLinhas
	if !errors.As(err, &el2) || len(el2.Problemas) != 2 || el2.Problemas[0].Linha != 5 || el2.Problemas[1].Linha != 6 || len(out) != 1 || f.LinhasIgnoradas() != 2 {
		t.Fatalf("problemas=%+v n=%d ignoradas=%d err=%v", el2, len(out), f.LinhasIgnoradas(), err)
	}
}

func TestCodigoSemConta(t *testing.T) {
	c := "Código;Vl. Realizado;Mês;Loja\nA1;5,00;202602;3\n"
	f, out, err := lerFonte(t, []byte(c))
	if err != nil || len(out) != 1 || out[0].Conta != "A1" || f.Nome() != "csv.contas" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestPlanilhaSemTotalNaoTemConferencia(t *testing.T) {
	f, _, err := lerFonte(t, []byte(cabPlanilha+"1;D;1;0;0;G;202609;1\n"))
	if err != nil || f.Conferencia() != nil || f.LinhasIgnoradas() != 0 {
		t.Fatalf("conf=%v ignoradas=%d err=%v", f.Conferencia(), f.LinhasIgnoradas(), err)
	}
}
