package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// planilhaF ficticia cria uma planilha mínima com as abas MENSAL e METRICAS2 no
// mesmo formato da análise do cliente (fórmulas SUM(Dx:Dy), somas D#+D# e
// subtração). Nada aqui vem de arquivo real.
func planilhaFicticia(t *testing.T, ajusta func(x *excelize.File)) string {
	t.Helper()
	x := excelize.NewFile()
	defer x.Close()
	x.SetSheetName("Sheet1", "MENSAL")
	x.NewSheet("METRICAS2")

	linha := func(aba string, n int, cels ...any) {
		c, _ := excelize.CoordinatesToCellName(1, n)
		if err := x.SetSheetRow(aba, c, &cels); err != nil {
			t.Fatal(err)
		}
	}
	soma := func(n int, f string) { x.SetCellFormula("MENSAL", "D"+itoa(n), f) }

	linha("MENSAL", 1, "", "", "", "=SWITCH(1,2,3)")
	linha("MENSAL", 2, "", "MES", "2026")
	linha("MENSAL", 4, "GRUPO", "CONTA", "DESCRIÇÃO")
	linha("MENSAL", 5, "REC", "1", "Receita fict")
	linha("MENSAL", 6, "DEV", "2", "(-) Devolucao fict")
	linha("MENSAL", 7, "", "", "(=) Venda Líquida fict")
	linha("MENSAL", 8, "CUS", "3", "(-) Custo fict")
	linha("MENSAL", 9, "", "", "(=) Margem fict")
	linha("MENSAL", 10, 0, "", "Comercial")
	linha("MENSAL", 11, "COM A", "", "COM A")
	linha("MENSAL", 12, "COM B", "", "COM B")
	linha("MENSAL", 13, 0, "", "Logística")
	linha("MENSAL", 14, "LOG A", "", "LOG A")
	linha("MENSAL", 15, 0, "", "(=) Administrativo")
	linha("MENSAL", 16, "ADM  UM", "", "ADM  UM")
	// linha 17 em branco, dentro do intervalo
	linha("MENSAL", 18, "ADM DOIS", "", "ADM DOIS")
	linha("MENSAL", 19, 0, "", "Total Despesas")
	linha("MENSAL", 20, 0, "", "(=) Resultado fict")
	for _, n := range []int{5, 6, 8, 11, 12, 14, 16, 18} {
		soma(n, "IF($C$2=2026,SUMIFS(Real!$B:$B,Real!$F:$F,MENSAL!$A"+itoa(n)+"),0)")
	}
	soma(7, "SUM(D5:D6)")
	soma(9, "D7+D8")
	soma(10, "D11+D12")
	soma(13, "D14")
	soma(15, "SUM(D16:D18)")
	soma(19, "D10+D13+D15")
	soma(20, "D9-D19")

	linha("METRICAS2", 1, "", "0")
	linha("METRICAS2", 2, "CONTA", "DESCRIÇÃO", "GRUPO", "", "", "", "", "", "", "", "", "CENTRO DE CUSTO")
	linha("METRICAS2", 3, "1", "Receita", "REC")
	linha("METRICAS2", 4, "", "(=) Venda Líquida fict", "")
	linha("METRICAS2", 5, "2", "Devolucao", "DEV")
	linha("METRICAS2", 6, "3", "Custo", "CUS")
	linha("METRICAS2", 7, 100, "Despesa com A", "COM A", "", "", "", "", "", "", "", "", "CC1")
	linha("METRICAS2", 8, "101", "Despesa com B", "com b")
	linha("METRICAS2", 9, "102", "Despesa log", "LOG A", "", "", "", "", "", "", "", "", "CC2")
	linha("METRICAS2", 10, "103", "Despesa adm", "ADM UM")
	linha("METRICAS2", 11, "104", "Sem grupo", "")
	linha("METRICAS2", 12, "105", "Grupo desconhecido", "NAO EXISTE")
	linha("METRICAS2", 13, "106", "Aponta para subtotal", "Total Despesas")
	linha("METRICAS2", 14, "1", "Receita repetida", "REC")
	linha("METRICAS2", 15, "107", "Outra adm", "ADM DOIS")
	if ajusta != nil {
		ajusta(x)
	}
	p := filepath.Join(t.TempDir(), "ficticia.xlsx")
	if err := x.SaveAs(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func itoa(n int) string { return strconv.Itoa(n) }

func lerCSVSaida(t *testing.T, pasta, nome string) [][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(pasta, nome))
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(b), "\xef\xbb\xbf")))
	r.Comma = ';'
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestGerar_PlanilhaFicticia(t *testing.T) {
	arq := planilhaFicticia(t, nil)
	saida := filepath.Join(t.TempDir(), "saida")
	r, err := Gerar(Opcoes{Arquivo: arq, Saida: saida, AbaEstrutura: "MENSAL", AbaDicionario: "METRICAS2", ColunaValor: "D"})
	if err != nil {
		t.Fatal(err)
	}

	est := lerCSVSaida(t, saida, "estrutura_dre.csv")
	quer := [][]string{
		{"codigo", "descricao", "nivel", "ordem", "tipo", "soma_de", "papel"},
		{"REC", "Receita fict", "1", "10", "conta", "", ""},
		{"DEV", "(-) Devolucao fict", "1", "20", "conta", "", ""},
		{"venda_liquida_fict", "(=) Venda Líquida fict", "0", "30", "subtotal", "REC|DEV", "faturamento_liquido"},
		{"CUS", "(-) Custo fict", "1", "40", "conta", "", ""},
		{"margem_fict", "(=) Margem fict", "0", "50", "subtotal", "venda_liquida_fict|CUS", ""},
		{"comercial", "Comercial", "0", "60", "subtotal", "COM A|COM B", ""},
		{"COM A", "COM A", "1", "70", "conta", "", ""},
		{"COM B", "COM B", "1", "80", "conta", "", ""},
		{"logistica", "Logística", "0", "90", "subtotal", "LOG A", ""},
		{"LOG A", "LOG A", "1", "100", "conta", "", ""},
		{"administrativo", "(=) Administrativo", "0", "110", "subtotal", "ADM  UM|ADM DOIS", ""},
		{"ADM  UM", "ADM  UM", "1", "120", "conta", "", ""},
		{"ADM DOIS", "ADM DOIS", "1", "130", "conta", "", ""},
		{"total_despesas", "Total Despesas", "0", "140", "subtotal", "comercial|logistica|administrativo", ""},
		{"resultado_fict", "(=) Resultado fict", "0", "150", "subtotal", "margem_fict|-total_despesas", ""},
	}
	// A ordem das linhas segue a planilha: a conta COM A vem depois do subtotal Comercial.
	if !reflect.DeepEqual(est, quer) {
		for i := range est {
			if i < len(quer) && !reflect.DeepEqual(est[i], quer[i]) {
				t.Errorf("estrutura linha %d:\n got %q\nwant %q", i, est[i], quer[i])
			}
		}
		t.Fatalf("estrutura difere (%d x %d linhas)", len(est), len(quer))
	}

	mapa := lerCSVSaida(t, saida, "mapa_dre.csv")
	wantMapa := [][]string{
		{"conta_padrao", "linha_codigo", "sinal"},
		{"1", "REC", "1"}, {"2", "DEV", "1"}, {"3", "CUS", "1"}, {"100", "COM A", "1"}, {"101", "COM B", "1"},
		{"102", "LOG A", "1"}, {"103", "ADM  UM", "1"}, {"107", "ADM DOIS", "1"},
	}
	if !reflect.DeepEqual(mapa, wantMapa) {
		t.Errorf("mapa:\n got %q\nwant %q", mapa, wantMapa)
	}

	dep := lerCSVSaida(t, saida, "departamentos.csv")
	wantDep := [][]string{
		{"codigo", "nome", "grupo"},
		{"COM A", "COM A", "Comercial"}, {"COM B", "COM B", "Comercial"}, {"LOG A", "LOG A", "Logística"},
		{"ADM  UM", "ADM  UM", "Administrativo"}, {"ADM DOIS", "ADM DOIS", "Administrativo"},
	}
	if !reflect.DeepEqual(dep, wantDep) {
		t.Errorf("departamentos:\n got %q\nwant %q", dep, wantDep)
	}

	dic := lerCSVSaida(t, saida, "dicionario.csv")
	if dic[0][0] != "conta" || dic[0][3] != "departamento" || dic[0][4] != "centro_custo" {
		t.Errorf("cabeçalho do dicionário: %q", dic[0])
	}
	achado := map[string][]string{}
	for _, l := range dic[1:] {
		achado[l[0]] = l
	}
	if !reflect.DeepEqual(achado["100"], []string{"100", "Despesa com A", "COM A", "COM A", "CC1"}) {
		t.Errorf("conta 100: %q", achado["100"])
	}
	if !reflect.DeepEqual(achado["101"], []string{"101", "Despesa com B", "COM B", "COM B", ""}) {
		t.Errorf("conta 101 (grupo com caixa diferente): %q", achado["101"])
	}
	if achado["1"][3] != "" || achado["104"][2] != "" || achado["105"][2] != "NAO EXISTE" || len(dic) != 1+11 {
		t.Errorf("dicionário: %q", dic)
	}

	sem := lerCSVSaida(t, saida, "contas_sem_linha.csv")
	if len(sem) != 4 || sem[1][0] != "104" || sem[1][4] != "sem_grupo" || sem[2][4] != "grupo_sem_linha" || sem[3][0] != "106" || sem[3][4] != "grupo_sem_linha" {
		t.Errorf("sem linha: %q", sem)
	}

	if r.Linhas != 15 || r.Subtotais != 7 || r.LinhasConta != 8 || r.ContasMapeadas != 8 || r.SemGrupo != 1 || r.GrupoSemLinha != 2 || r.Duplicadas != 1 ||
		r.DepartamentosPorBloco["Comercial"] != 2 || r.DepartamentosPorBloco["Logística"] != 1 || r.DepartamentosPorBloco["Administrativo"] != 2 || !r.Papel {
		t.Errorf("resumo: %+v", r)
	}
}

func TestGerar_ContaRepetidaComLinhasDiferentes(t *testing.T) {
	arq := planilhaFicticia(t, func(x *excelize.File) {
		x.SetCellValue("METRICAS2", "C14", "DEV") // a conta 1 já está em REC
	})
	_, err := Gerar(Opcoes{Arquivo: arq, Saida: filepath.Join(t.TempDir(), "s"), AbaEstrutura: "MENSAL", AbaDicionario: "METRICAS2", ColunaValor: "D"})
	if err == nil || !strings.Contains(err.Error(), "conta repetida com grupos diferentes") {
		t.Fatalf("esperava erro claro, veio %v", err)
	}
}

func TestGerar_ChaveRepetidaEFormulaInvalida(t *testing.T) {
	arq := planilhaFicticia(t, func(x *excelize.File) {
		x.SetCellValue("MENSAL", "A6", "REC") // chave repetida
	})
	_, err := Gerar(Opcoes{Arquivo: arq, Saida: filepath.Join(t.TempDir(), "s"), AbaEstrutura: "MENSAL", AbaDicionario: "METRICAS2", ColunaValor: "D"})
	if err == nil || !strings.Contains(err.Error(), "chave repetida") {
		t.Fatalf("chave repetida: %v", err)
	}
	arq = planilhaFicticia(t, func(x *excelize.File) {
		x.SetCellFormula("MENSAL", "D7", "SUM(D5:D7)") // soma a si mesma
	})
	_, err = Gerar(Opcoes{Arquivo: arq, Saida: filepath.Join(t.TempDir(), "s"), AbaEstrutura: "MENSAL", AbaDicionario: "METRICAS2", ColunaValor: "D"})
	if err == nil || !strings.Contains(err.Error(), "soma a própria linha") {
		t.Fatalf("autossoma: %v", err)
	}
	arq = planilhaFicticia(t, func(x *excelize.File) {
		x.SetCellFormula("MENSAL", "D9", "D7+D99") // fora do DRE
	})
	_, err = Gerar(Opcoes{Arquivo: arq, Saida: filepath.Join(t.TempDir(), "s"), AbaEstrutura: "MENSAL", AbaDicionario: "METRICAS2", ColunaValor: "D"})
	if err == nil || !strings.Contains(err.Error(), "não é linha do DRE") {
		t.Fatalf("referência fora: %v", err)
	}
}

func TestGerar_RecusaPastaDentroDeRepositorio(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Gerar(Opcoes{Arquivo: "nao-importa.xlsx", Saida: filepath.Join(repo, "x"), AbaEstrutura: "MENSAL", AbaDicionario: "METRICAS2", ColunaValor: "D"})
	if err == nil || !strings.Contains(err.Error(), "repositório") {
		t.Fatalf("esperava recusa: %v", err)
	}
}

func TestParseSomaSimples(t *testing.T) {
	casos := []struct {
		f    string
		want []refSoma
		ok   bool
	}{
		{"SUM(D5:D7)", []refSoma{{5, false}, {6, false}, {7, false}}, true},
		{"=sum(D5:D6, D9)", []refSoma{{5, false}, {6, false}, {9, false}}, true},
		{"D15+D16", []refSoma{{15, false}, {16, false}}, true},
		{"D21+D24-D28", []refSoma{{21, false}, {24, false}, {28, true}}, true},
		{"$D$3+$D$4", []refSoma{{3, false}, {4, false}}, true},
		{"IF(D1=2,D3,D4)", nil, false},
		{"SUMIFS(A:A,B:B,C5)", nil, false},
		{"SUM(E5:E7)", nil, false},
		{"D5D6", nil, false},
		{"", nil, false},
	}
	for _, c := range casos {
		got, ok := parseSomaSimples(c.f, "MENSAL", "D")
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: %v %v", c.f, got, ok)
		}
	}
}
