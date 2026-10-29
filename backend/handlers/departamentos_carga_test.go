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

// História 2.20: carga de departamentos por arquivo. Dados fictícios.

func uploadDeptos(t *testing.T, h http.Handler, f testdb.Fixture, perfil, nome string, conteudo []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("arquivo", nome)
	part.Write(conteudo)
	mw.Close()
	const url = "/api/aud/departamentos/carga"
	r := httptest.NewRequest(http.MethodPost, url, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return chamar(h, r.WithContext(reqFilial(http.MethodPost, url, "", f, perfil).Context()))
}

func TestDepartamentosCarga(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	h := DepartamentosCargaHandler(db)
	const q = `SELECT COUNT(*) FROM auditoria.departamentos WHERE empresa_id=$1::uuid`

	csv := []byte("codigo;nome;grupo\nVEN;Vendas;Comercial\nLOG;Logistica;logistica\nADM;Administracao;ADMINISTRATIVO\n")
	// Permissão: só quem edita a estrutura; nada gravado.
	if w := uploadDeptos(t, h, f, "diretoria", "d.csv", csv); w.Code != http.StatusForbidden {
		t.Fatalf("diretoria: %d", w.Code)
	}
	if n := contar(t, db, q, f.EmpresaID); n != 0 {
		t.Errorf("403 gravou %d", n)
	}
	if w := chamar(h, reqFilial(http.MethodGet, "/api/aud/departamentos/carga", "", f, "controladoria")); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
	if w := chamar(h, reqFilial(http.MethodPost, "/api/aud/departamentos/carga", "", f, "controladoria")); w.Code != http.StatusBadRequest {
		t.Errorf("sem arquivo: %d", w.Code)
	}

	// Criação (CSV), grupo sem acento e em qualquer caixa aceito e gravado canônico.
	w := uploadDeptos(t, h, f, "controladoria", "d.csv", csv)
	var out struct{ Linhas, Criados, Atualizados, Inalterados int }
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || out.Criados != 3 || out.Atualizados != 0 || out.Inalterados != 0 || out.Linhas != 3 {
		t.Fatalf("criação: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, q+` AND codigo='LOG' AND grupo='Logística'`, f.EmpresaID); n != 1 {
		t.Errorf("grupo logistica não ficou canônico")
	}

	// Reenvio (XLSX): VEN igual, LOG muda nome e grupo, NOVO é criado; ADM ausente NÃO é apagado.
	x := xlsxDic(t, [][]any{{"Código", "Nome", "Grupo"}, {"VEN", "Vendas", "Comercial"}, {"LOG", "Logistica nova", "Administrativo"}, {"NOVO", "Novo depto", "Comercial"}})
	w = uploadDeptos(t, h, f, "controladoria", "d.xlsx", x)
	out = struct{ Linhas, Criados, Atualizados, Inalterados int }{}
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || out.Criados != 1 || out.Atualizados != 1 || out.Inalterados != 1 {
		t.Fatalf("reenvio: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, q, f.EmpresaID); n != 4 {
		t.Errorf("departamentos: %d, esperado 4 (ADM fica)", n)
	}
	if n := contar(t, db, q+` AND codigo='LOG' AND nome='Logistica nova' AND grupo='Administrativo'`, f.EmpresaID); n != 1 {
		t.Errorf("LOG não foi atualizado")
	}
	// Outra empresa não vê nem é alterada.
	if n := contar(t, db, q, outra.EmpresaID); n != 0 {
		t.Errorf("outra empresa: %d", n)
	}

	// Erros: nada gravado, tudo ou nada (a linha boa não entra).
	antes := contar(t, db, q, f.EmpresaID)
	ruim := "codigo;nome;grupo\nBOM;Bom;Comercial\n;Sem codigo;Comercial\nREP;Um;Comercial\nREP;Dois;Comercial\nGRP;Grupo ruim;Financeiro\nNOM;;Comercial\n" +
		"BIG" + strings.Repeat("x", 40) + ";Codigo grande;Comercial\nNMG;" + strings.Repeat("n", 121) + ";Comercial\n"
	w = uploadDeptos(t, h, f, "controladoria", "ruim.csv", []byte(ruim))
	corpo := w.Body.String()
	if w.Code != http.StatusBadRequest {
		t.Fatalf("erros: %d %s", w.Code, corpo)
	}
	for _, m := range []string{"codigo vazio", "codigo repetido REP", "grupo deve ser", "nome vazio", "no máximo 40", "no máximo 120"} {
		if !strings.Contains(corpo, m) {
			t.Errorf("faltou %q em %s", m, corpo)
		}
	}
	if n := contar(t, db, q, f.EmpresaID); n != antes {
		t.Errorf("erro gravou: %d → %d", antes, n)
	}
	if n := contar(t, db, q+` AND codigo='BOM'`, f.EmpresaID); n != 0 {
		t.Errorf("linha boa entrou apesar do erro")
	}
	// Mais de 20 erros: lista 20 e o total.
	var b strings.Builder
	b.WriteString("codigo;nome;grupo\n")
	for i := 0; i < 25; i++ {
		b.WriteString("X;Nome;Invalido\n")
	}
	w = uploadDeptos(t, h, f, "controladoria", "muitos.csv", []byte(b.String()))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "25 linha(s) com problema") || !strings.Contains(w.Body.String(), "e mais 5") {
		t.Errorf("limite de 20: %d %s", w.Code, w.Body.String())
	}
	// Cabeçalho inválido e arquivo vazio.
	if w = uploadDeptos(t, h, f, "controladoria", "c.csv", []byte("a;b\n1;2\n")); w.Code != http.StatusBadRequest {
		t.Errorf("cabeçalho: %d", w.Code)
	}
	if w = uploadDeptos(t, h, f, "controladoria", "v.csv", []byte("codigo;nome;grupo\n")); w.Code != http.StatusBadRequest {
		t.Errorf("sem linhas: %d", w.Code)
	}

	// Trilha: 2 cargas, só contagens e códigos (nenhum nome).
	rows, err := db.Query(`SELECT payload::text FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='departamento' AND acao='carga' ORDER BY criado_em`, f.EmpresaID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var d string
		rows.Scan(&d)
		n++
		for _, proibido := range []string{"Vendas", "Logistica", "Novo depto", "Administracao"} {
			if strings.Contains(d, proibido) {
				t.Errorf("trilha com nome: %s", d)
			}
		}
		if !strings.Contains(d, "codigos_criados") {
			t.Errorf("trilha sem códigos: %s", d)
		}
	}
	if n != 2 {
		t.Errorf("trilhas de carga: %d, esperado 2 (as recusadas não gravam)", n)
	}
}
