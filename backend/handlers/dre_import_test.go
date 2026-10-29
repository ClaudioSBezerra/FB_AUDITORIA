package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"fb_auditoria/internal/testdb"
)

// Teste de caracterização da importação CSV (DREImportarHandler).

func importar(t *testing.T, f testdb.Fixture, nome, conteudo string) *httptest.ResponseRecorder {
	t.Helper()
	db := testdb.Open(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("arquivo", nome)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(conteudo)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/aud/dre/lancamentos/importar", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return chamar(DREImportarHandler(db), reqComCtx(r, f))
}

const csvPontoEVirgula = "competencia;filial;conta;valor;conta_desc\n2026-09;01;3.1;1.234,56;Venda\n2026-09;02;3.2;100,00;Devolucao\n"
const csvVirgula = "competencia,filial,conta,valor,conta_desc\n2026-09,01,3.1,1234.56,Venda\n2026-09,02,3.2,100.00,Devolucao\n"

func TestImportar_SeparadorEquivalente(t *testing.T) {
	casos := []struct{ nome, csv string }{{"ponto_e_virgula.csv", csvPontoEVirgula}, {"virgula.csv", csvVirgula}}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			db := testdb.Open(t)
			f := testdb.NovaEmpresa(t, db)
			w := importar(t, f, c.nome, c.csv)
			if w.Code != http.StatusOK {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			var resp struct {
				CargaID int64 `json:"carga_id"`
				Linhas  int   `json:"linhas"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Linhas != 2 || resp.CargaID == 0 {
				t.Fatalf("resposta inesperada: %s (%v)", w.Body.String(), err)
			}
			if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND carga_id=$2`, f.EmpresaID, resp.CargaID); n != 2 {
				t.Errorf("lançamentos com carga_id: esperado 2, veio %d", n)
			}
			if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND conta='3.1' AND valor=1234.56 AND filial='01' AND competencia='2026-09-01' AND origem='csv'`, f.EmpresaID); n != 1 {
				t.Errorf("lançamento 3.1 esperado exato, veio %d", n)
			}
			if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND conta='3.2' AND valor=100.00 AND filial='02'`, f.EmpresaID); n != 1 {
				t.Errorf("lançamento 3.2 esperado exato, veio %d", n)
			}
			if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='lancamentos' AND acao='importar' AND entidade_id=$2`, f.EmpresaID, resp.CargaID); n != 1 {
				t.Errorf("trilha lancamentos/importar: esperado 1, veio %d", n)
			}
		})
	}
}

// Competência com fechamento recusa a importação (409) e nada é gravado (FR-11).
func TestImportar_MesFechado_Recusa409(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := db.Exec(`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial, fechado_por)
		VALUES ($1::uuid,'2026-09-01',1,'',$2::uuid)`, f.EmpresaID, f.UserID); err != nil {
		t.Fatal(err)
	}
	w := importar(t, f, "fechado.csv", csvPontoEVirgula)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "2026-09") || !strings.Contains(w.Body.String(), "reabrir") {
		t.Fatalf("esperava 409 com escopo e pedido de reabrir, veio %d: %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("esperava nada gravado, vieram %d", n)
	}
}

func TestImportar_Invalido_Retorna400ComMensagem(t *testing.T) {
	casos := []struct{ nome, csv, msg string }{
		{"filial_vazia", "competencia;filial;conta;valor\n2026-09;;3.1;1,00\n", "linha 2"},
		{"sem_coluna_filial", "competencia;conta;valor\n2026-09;3.1;1,00\n", "coluna obrigatória ausente"},
		{"so_cabecalho", "competencia;filial;conta;valor\n", "nenhuma linha"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			db := testdb.Open(t)
			f := testdb.NovaEmpresa(t, db)
			w := importar(t, f, c.nome+".csv", c.csv)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), c.msg) {
				t.Fatalf("esperava 400 com %q, veio %d: %s", c.msg, w.Code, w.Body.String())
			}
			if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
				t.Errorf("gravou %d linhas", n)
			}
		})
	}
}

func xlsxBytes(t *testing.T, linhas [][]any) string {
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
	return b.String()
}

// Planilha do cliente (CSV e XLSX): origem csv.contas, ajuste ignorado e contado,
// reenvio substitui o escopo.
func TestImportar_Planilha(t *testing.T) {
	csvPlan := "CONTA;DESCRIÇÃO;VL. Realizado;Ajustes;Saldo;Grupo;Mês;Loja\n" +
		"4010001;Venda;1.234,56;5,00;0;G1;202609;01\n4010002;Custo;10,00;0;0;G1;202609;1\n4010003;Outra;3,00;2;0;G1;202609;02\n"
	xl := xlsxBytes(t, [][]any{
		{"CONTA", "DESCRIÇÃO", "VL. Realizado", "Ajustes", "Saldo", "Grupo", "Mês", "Loja"},
		{"4010001", "Venda", 1234.56, 5, 0, "G1", 202609, 1},
		{"4010002", "Custo", 10, 0, 0, "G1", 202609, 1},
		{"4010003", "Outra", 3, 2, 0, "G1", 202609, 2},
	})
	for nome, conteudo := range map[string]string{"planilha.csv": csvPlan, "planilha.xlsx": xl} {
		t.Run(nome, func(t *testing.T) {
			db := testdb.Open(t)
			f := testdb.NovaEmpresa(t, db)
			for i := 0; i < 2; i++ { // reenvio não duplica
				w := importar(t, f, nome, conteudo)
				if w.Code != http.StatusOK {
					t.Fatalf("status %d: %s", w.Code, w.Body.String())
				}
				var resp struct {
					Linhas           int `json:"linhas"`
					AjustesIgnorados int `json:"ajustes_ignorados"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Linhas != 3 || resp.AjustesIgnorados != 2 {
					t.Fatalf("resposta inesperada: %s (%v)", w.Body.String(), err)
				}
			}
			if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND origem='csv.contas' AND competencia='2026-09-01'`, f.EmpresaID); n != 3 {
				t.Errorf("esperava 3 lançamentos csv.contas, vieram %d", n)
			}
			if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND filial='1' AND valor IN (1234.56, 10)`, f.EmpresaID); n != 2 {
				t.Errorf("filial 01 e 1 deviam ser a mesma, veio %d", n)
			}
		})
	}
}

func TestImportar_Planilha_Invalida400(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	w := importar(t, f, "p.csv", "CONTA;VL. Realizado;Mês;Loja\n1;1;2026-09;1\n")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "linha 2") {
		t.Fatalf("esperava 400 com linha 2, veio %d: %s", w.Code, w.Body.String())
	}
	w = importar(t, f, "p.xlsx", "PK\x03\x04truncado")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 para ZIP corrompido, veio %d: %s", w.Code, w.Body.String())
	}
}

// Entrada_Dados (história 2.17): duas tabelas com cabeçalhos diferentes,
// subtotal sem código e linhas de conferência sem Mês e Loja. Dados fictícios.
func entradaDadosCSV(total string) string {
	return "Código;Conta;Vl. Realizado;Ajuste;Saldo;Grupo;Mês;Loja\n" +
		"1;Venda bruta;100,00;;;G;202602;7\n" +
		";Faturamento Líquido;100,00;;;G;202602;7\n" +
		";;;;;;;\n" +
		"CONTA;DESCRIÇÃO;VL. Realizado;Ajuste;Saldo;Grupo;Mês;Loja\n" +
		"4010001;Conta A;10,50;;;G;202602;7\n" +
		"4010002;Conta B;4,50;;;G;202602;7\n" +
		"TOTAL;;" + total + ";;;;;\n" +
		"Calculo para prova real;;1,00;;;;;\n"
}

type respEntrada struct {
	Linhas           int `json:"linhas"`
	LinhasIgnoradas  int `json:"linhas_ignoradas"`
	AjustesIgnorados int `json:"ajustes_ignorados"`
	Conferencia      *struct {
		Esperado  float64 `json:"esperado"`
		Calculado float64 `json:"calculado"`
		OK        bool    `json:"ok"`
	} `json:"conferencia_total"`
}

func TestImportar_EntradaDados(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	for i := 0; i < 2; i++ { // reenvio não duplica
		w := importar(t, f, "entrada.csv", entradaDadosCSV("15,00"))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var r respEntrada
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil || r.Linhas != 3 || r.LinhasIgnoradas != 3 || r.Conferencia == nil || !r.Conferencia.OK || r.Conferencia.Calculado != 15 {
			t.Fatalf("resposta inesperada: %s (%v)", w.Body.String(), err)
		}
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND origem='csv.contas' AND filial='7' AND competencia='2026-02-01'`, f.EmpresaID); n != 3 {
		t.Errorf("esperava 3 lançamentos, vieram %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND conta IN ('CONTA','TOTAL')`, f.EmpresaID); n != 0 {
		t.Errorf("cabeçalho ou TOTAL viraram lançamento: %d", n)
	}
	// TOTAL diverge: aviso, não bloqueia
	w := importar(t, f, "entrada.csv", entradaDadosCSV("99,00"))
	var r respEntrada
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &r) != nil || r.Conferencia == nil || r.Conferencia.OK || r.Conferencia.Esperado != 99 {
		t.Fatalf("esperava 200 com conferência divergente: %d %s", w.Code, w.Body.String())
	}
}

func TestImportar_EntradaDados_XLSX(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	xl := xlsxBytes(t, [][]any{
		{"Código", "Conta", "Vl. Realizado", "Ajuste", "Saldo", "Grupo", "Mês", "Loja"},
		{"1", "Venda bruta", 100, "", "", "G", 202602, 7},
		{"", "Faturamento Líquido", 100, "", "", "G", 202602, 7},
		{},
		{"CONTA", "DESCRIÇÃO", "VL. Realizado", "Ajuste", "Saldo", "Grupo", "Mês", "Loja"},
		{4010001, "Conta A", 10.5, "", "", "G", 202602, 7},
		{"TOTAL", "", 10.5},
	})
	w := importar(t, f, "entrada.xlsx", xl)
	var r respEntrada
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &r) != nil || r.Linhas != 2 || r.LinhasIgnoradas != 2 || r.Conferencia == nil || !r.Conferencia.OK {
		t.Fatalf("resposta inesperada: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND conta='4010001' AND valor=10.5`, f.EmpresaID); n != 1 {
		t.Errorf("conta numérica de célula esperada exata, veio %d", n)
	}
}

func TestImportar_EntradaDados_MesFechado409ELojaVazia400(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	w := importar(t, f, "e.csv", strings.Replace(entradaDadosCSV("15,00"), "4010002;Conta B;4,50;;;G;202602;7", "4010002;Conta B;4,50;;;G;202602;", 1))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Loja vazia") {
		t.Fatalf("esperava 400 com Loja vazia, veio %d: %s", w.Code, w.Body.String())
	}
	if _, err := db.Exec(`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial, fechado_por)
		VALUES ($1::uuid,'2026-02-01',1,'',$2::uuid)`, f.EmpresaID, f.UserID); err != nil {
		t.Fatal(err)
	}
	w = importar(t, f, "e.csv", entradaDadosCSV("15,00"))
	if w.Code != http.StatusConflict {
		t.Fatalf("esperava 409, veio %d: %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("esperava nada gravado, vieram %d", n)
	}
}
