package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// Hoje a importação em competência com fechamento é aceita (muda na 1.6:
// passa a ser recusada com 409).
func TestImportar_MesFechado_Aceita_MudaNa1_6(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := db.Exec(`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial, fechado_por)
		VALUES ($1::uuid,'2026-09-01',1,'',$2::uuid)`, f.EmpresaID, f.UserID); err != nil {
		t.Fatal(err)
	}
	w := importar(t, f, "fechado.csv", csvPontoEVirgula)
	if w.Code != http.StatusOK {
		t.Fatalf("esperava aceitar em mês fechado (hoje), veio %d: %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 2 {
		t.Errorf("esperava 2 lançamentos gravados, vieram %d", n)
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
