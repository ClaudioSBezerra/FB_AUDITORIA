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

func uploadDic(t *testing.T, h http.Handler, f testdb.Fixture, perfil, nome string, conteudo []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("arquivo", nome)
	part.Write(conteudo)
	mw.Close()
	r := reqFilial(http.MethodPost, "/api/aud/dicionario/carga", "", f, perfil)
	r.Body = http.NoBody
	r2 := httptest.NewRequest(http.MethodPost, "/api/aud/dicionario/carga", &buf)
	r2.Header.Set("Content-Type", mw.FormDataContentType())
	return chamar(h, r2.WithContext(r.Context()))
}

func xlsxDic(t *testing.T, linhas [][]any) []byte {
	t.Helper()
	x := excelize.NewFile()
	defer x.Close()
	s := x.GetSheetName(0)
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
	return buf.Bytes()
}

func TestDicionario_Handlers(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	hd := DepartamentosHandler(db)
	carga, lista, conta := DicionarioCargaHandler(db), DicionarioHandler(db), DicionarioContaHandler(db)

	var ids [2]string
	for i, c := range []string{"G1", "D2"} {
		w := chamar(hd, reqFilial(http.MethodPost, "/api/aud/departamentos", `{"codigo":"`+c+`","nome":"Depto","grupo":"Logística"}`, f, "controladoria"))
		var d struct {
			ID string `json:"id"`
		}
		json.Unmarshal(w.Body.Bytes(), &d)
		ids[i] = d.ID
	}

	csv := []byte("conta;descricao;grupo\nC1;Um;G1\nC2;Dois;ZZ\n")
	if w := uploadDic(t, carga, f, "diretoria", "d.csv", csv); w.Code != http.StatusForbidden {
		t.Fatalf("diretoria carga: %d", w.Code)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.conta_dicionario WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("403 gravou: %d", n)
	}
	// Erro: nada gravado, mensagem lista os problemas.
	w := uploadDic(t, carga, f, "controladoria", "d.csv", []byte("conta;grupo\nC1;G1\nC1;G1\nC3;G1;extra\n"))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "repetida") {
		t.Fatalf("erro esperado 400: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.conta_dicionario WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("erro gravou: %d", n)
	}
	// Sem arquivo e cabeçalho inválido.
	r := reqFilial(http.MethodPost, "/api/aud/dicionario/carga", "", f, "controladoria")
	if w = chamar(carga, r); w.Code != http.StatusBadRequest {
		t.Errorf("sem arquivo: %d", w.Code)
	}
	if w = uploadDic(t, carga, f, "controladoria", "d.csv", []byte("a;b\n1;2\n")); w.Code != http.StatusBadRequest {
		t.Errorf("cabeçalho inválido: %d", w.Code)
	}
	// Carga CSV e depois XLSX (reenvio).
	if w = uploadDic(t, carga, f, "controladoria", "d.csv", csv); w.Code != http.StatusOK {
		t.Fatalf("carga: %d %s", w.Code, w.Body.String())
	}
	x := xlsxDic(t, [][]any{{"conta", "descrição", "grupo", "departamento"}, {"C1", "Um novo", "G1", ""}, {"C2", "Dois", "ZZ", "D2"}, {"C3", "Três", "G1", ""}})
	if w = uploadDic(t, carga, f, "controladoria", "d.xlsx", x); w.Code != http.StatusOK {
		t.Fatalf("carga xlsx: %d %s", w.Code, w.Body.String())
	}

	w = chamar(lista, reqFilial(http.MethodGet, "/api/aud/dicionario", "", f, "diretoria"))
	var l struct {
		Total          int `json:"total"`
		PendentesTotal int `json:"pendentes_total"`
		Contas         []struct {
			Conta, Origem      string
			DepartamentoCodigo string `json:"departamento_codigo"`
			Descricao          string
		}
	}
	json.Unmarshal(w.Body.Bytes(), &l)
	if w.Code != http.StatusOK || l.Total != 3 || l.PendentesTotal != 0 || l.Contas[0].Descricao != "Um novo" || l.Contas[1].DepartamentoCodigo != "D2" || l.Contas[1].Origem != "manual" {
		t.Fatalf("lista: %d %s", w.Code, w.Body.String())
	}
	w = chamar(lista, reqFilial(http.MethodGet, "/api/aud/dicionario", "", outra, "controladoria"))
	if strings.Contains(w.Body.String(), "C1") {
		t.Errorf("outra empresa vê o dicionário")
	}

	// Correção por conta.
	body := `{"conta":"C1","departamento_id":"` + ids[1] + `"}`
	if w = chamar(conta, reqFilial(http.MethodPut, "/api/aud/dicionario/conta", body, f, "diretoria")); w.Code != http.StatusForbidden {
		t.Errorf("PUT diretoria: %d", w.Code)
	}
	if w = chamar(conta, reqFilial(http.MethodPut, "/api/aud/dicionario/conta", body, f, "controladoria")); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"manual"`) {
		t.Errorf("PUT: %d %s", w.Code, w.Body.String())
	}
	if w = chamar(conta, reqFilial(http.MethodPut, "/api/aud/dicionario/conta", `{"conta":"NAO","departamento_id":"`+ids[1]+`"}`, f, "controladoria")); w.Code != http.StatusNotFound {
		t.Errorf("PUT conta inexistente: %d", w.Code)
	}
	if w = chamar(conta, reqFilial(http.MethodPut, "/api/aud/dicionario/conta", `lixo`, f, "controladoria")); w.Code != http.StatusBadRequest {
		t.Errorf("PUT lixo: %d", w.Code)
	}
	if w = chamar(conta, reqFilial(http.MethodGet, "/api/aud/dicionario/conta", "", f, "controladoria")); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET conta: %d", w.Code)
	}

	// Exclusão de departamento ligado: 409.
	if w = chamar(hd, reqFilial(http.MethodDelete, "/api/aud/departamentos?id="+ids[1], "", f, "controladoria")); w.Code != http.StatusConflict {
		t.Errorf("DELETE ligado: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dicionario'`, f.EmpresaID); n != 3 {
		t.Errorf("trilha do dicionário: %d (2 cargas + 1 correção)", n)
	}
}

// Departamento inexistente no arquivo do dicionário: uma mensagem agregada com os
// códigos distintos e o número de linhas (história 2.20).
func TestDicionario_DepartamentoInexistenteAgregado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	chamar(DepartamentosHandler(db), reqFilial(http.MethodPost, "/api/aud/departamentos", `{"codigo":"OK","nome":"Depto","grupo":"Comercial"}`, f, "controladoria"))
	csv := []byte("conta;grupo;departamento\nC1;G;AAA\nC2;G;BBB\nC3;G;AAA\nC4;G;OK\n")
	w := uploadDic(t, DicionarioCargaHandler(db), f, "controladoria", "d.csv", csv)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "departamentos inexistentes: AAA, BBB (3 linhas)") {
		t.Fatalf("esperava a mensagem agregada: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.conta_dicionario WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("erro gravou %d", n)
	}
}
