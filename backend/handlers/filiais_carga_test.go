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

// História 2.21: carga do Mapa de filiais e dos agrupamentos por arquivo. Dados fictícios.

func uploadArquivoFiliais(t *testing.T, h http.Handler, url string, f testdb.Fixture, perfil, nome string, conteudo []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("arquivo", nome)
	part.Write(conteudo)
	mw.Close()
	r := httptest.NewRequest(http.MethodPost, url, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return chamar(h, r.WithContext(reqFilial(http.MethodPost, url, "", f, perfil).Context()))
}

func TestFiliaisGerenciaisCarga(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	h := FiliaisGerenciaisCargaHandler(db)
	const url = "/api/aud/filiais-gerenciais/carga"
	const qg = `SELECT COUNT(*) FROM auditoria.filiais_gerenciais WHERE empresa_id=$1::uuid`
	const qe = `SELECT COUNT(*) FROM auditoria.filiais_gerenciais_erp WHERE empresa_id=$1::uuid`
	up := func(perfil, nome, csv string) *httptest.ResponseRecorder {
		return uploadArquivoFiliais(t, h, url, f, perfil, nome, []byte(csv))
	}

	base := "filial_gerencial;filial_erp\n11;11\n11;12\n011;13\n20;20\n20;28\n21;21\n"
	if w := up("diretoria", "m.csv", base); w.Code != http.StatusForbidden {
		t.Fatalf("diretoria: %d", w.Code)
	}
	if n := contar(t, db, qg, f.EmpresaID); n != 0 {
		t.Errorf("403 gravou %d", n)
	}
	if w := chamar(h, reqFilial(http.MethodGet, url, "", f, "controladoria")); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
	if w := chamar(h, reqFilial(http.MethodPost, url, "", f, "controladoria")); w.Code != http.StatusBadRequest {
		t.Errorf("sem arquivo: %d", w.Code)
	}

	// Criação: "011" vira 11 (zeros à esquerda); nome padrão quando o arquivo não traz.
	type resp struct {
		Linhas, Criadas, Atualizadas, Inalteradas int
		LigacoesNovas                             int `json:"ligacoes_novas"`
		LigacoesMovidas                           int `json:"ligacoes_movidas"`
		Movidas                                   []struct{ Filial, De, Para string }
	}
	var out resp
	w := up("controladoria", "m.csv", base)
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || out.Criadas != 3 || out.LigacoesNovas != 6 || out.Linhas != 6 {
		t.Fatalf("criação: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, qg+` AND codigo='11' AND nome='Filial 11'`, f.EmpresaID); n != 1 {
		t.Errorf("filial 11 sem nome padrão")
	}
	if n := contar(t, db, qe+` AND filial_erp='13'`, f.EmpresaID); n != 1 {
		t.Errorf("filial 13 não ligada")
	}
	if n := contar(t, db, qg, outra.EmpresaID); n != 0 {
		t.Errorf("outra empresa: %d", n)
	}

	// Reenvio idempotente: nada novo.
	out = resp{}
	w = up("controladoria", "m.csv", base)
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || out.Criadas != 0 || out.LigacoesNovas != 0 || out.LigacoesMovidas != 0 || out.Inalteradas != 6 {
		t.Fatalf("reenvio: %d %s", w.Code, w.Body.String())
	}

	// Aditivo: 40 é nova; 28 muda de 20 para 21 (movida, informada); nome dado atualiza; 12 (não citada) fica.
	out = resp{}
	w = up("controladoria", "m2.csv", "Filial Gerencial;Filial ERP;Nome\n21;28;Distribuição 21\n30;40;\n")
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || out.Criadas != 1 || out.Atualizadas != 1 || out.LigacoesNovas != 1 || out.LigacoesMovidas != 1 ||
		len(out.Movidas) != 1 || out.Movidas[0].Filial != "28" || out.Movidas[0].De != "20" || out.Movidas[0].Para != "21" {
		t.Fatalf("aditivo: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, qe+` AND filial_erp='12'`, f.EmpresaID); n != 1 {
		t.Errorf("filial 12 foi apagada")
	}
	if n := contar(t, db, qg+` AND codigo='21' AND nome='Distribuição 21'`, f.EmpresaID); n != 1 {
		t.Errorf("nome não atualizou")
	}

	// Erros: nada gravado (a linha boa não entra).
	antes, antesE := contar(t, db, qg, f.EmpresaID), contar(t, db, qe, f.EmpresaID)
	ruim := "filial_gerencial;filial_erp\n50;50\n;51\n52;\n53;54\n55;54\nGRUPO;56\n57;" + strings.Repeat("9", 21) + "\n"
	w = up("controladoria", "ruim.csv", ruim)
	corpo := w.Body.String()
	if w.Code != http.StatusBadRequest {
		t.Fatalf("erros: %d %s", w.Code, corpo)
	}
	for _, m := range []string{"filial_gerencial vazia", "filial_erp vazia", "a filial 54 já está na filial gerencial 53 (linha 5)", "palavra reservada", "no máximo 20"} {
		if !strings.Contains(corpo, m) {
			t.Errorf("faltou %q em %s", m, corpo)
		}
	}
	if contar(t, db, qg, f.EmpresaID) != antes || contar(t, db, qe, f.EmpresaID) != antesE {
		t.Errorf("erro gravou")
	}
	// Código de filial gerencial igual a filial do ERP de outra: recusado (regra do cadastro manual).
	w = up("controladoria", "c.csv", "filial_gerencial;filial_erp\n60;61\n61;62\n")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "o código 61 é igual a uma filial do ERP ligada à filial gerencial 60") {
		t.Errorf("código igual a filial: %d %s", w.Code, w.Body.String())
	}
	// Cabeçalho inválido e arquivo sem linhas.
	if w = up("controladoria", "c.csv", "a;b\n1;2\n"); w.Code != http.StatusBadRequest {
		t.Errorf("cabeçalho: %d", w.Code)
	}
	if w = up("controladoria", "v.csv", "filial_gerencial;filial_erp\n"); w.Code != http.StatusBadRequest {
		t.Errorf("sem linhas: %d", w.Code)
	}

	// Trilha: 3 cargas gravadas (as recusadas não gravam).
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='filial_gerencial' AND acao='carga'`, f.EmpresaID).Scan(&n)
	if n != 3 {
		t.Errorf("trilhas de carga: %d, esperado 3", n)
	}
}

func TestAgrupamentosFiliaisCarga(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	h := AgrupamentosFiliaisCargaHandler(db)
	const url = "/api/aud/agrupamentos-filiais/carga"
	const qa = `SELECT COUNT(*) FROM auditoria.agrupamentos_filiais WHERE empresa_id=$1::uuid`
	const qm = `SELECT COUNT(*) FROM auditoria.agrupamentos_filiais_membros WHERE empresa_id=$1::uuid`
	up := func(perfil, csv string) *httptest.ResponseRecorder {
		return uploadArquivoFiliais(t, h, url, f, perfil, "a.csv", []byte(csv))
	}
	// Filiais gerenciais prontas (1, 12, 14 e 11), pela carga do mapa.
	mapa := uploadArquivoFiliais(t, FiliaisGerenciaisCargaHandler(db), "/api/aud/filiais-gerenciais/carga", f, "controladoria", "m.csv",
		[]byte("filial_gerencial;filial_erp\n1;1\n12;12\n14;14\n11;11\n11;13\n"))
	if mapa.Code != http.StatusOK {
		t.Fatalf("mapa: %d %s", mapa.Code, mapa.Body.String())
	}

	base := "agrupamento;nome;filial\nBAL;Balcões;01\nBAL;Balcões;12\nBAL;Balcões;14\nDIS;;11\n"
	if w := up("gestor", base); w.Code != http.StatusForbidden {
		t.Fatalf("gestor: %d", w.Code)
	}
	if w := chamar(h, reqFilial(http.MethodGet, url, "", f, "controladoria")); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
	type resp struct {
		Linhas, Criados, Atualizados, Inalteradas int
		MembrosAdicionados                        int `json:"membros_adicionados"`
	}
	var out resp
	w := up("controladoria", base)
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || out.Criados != 2 || out.MembrosAdicionados != 4 || out.Linhas != 4 {
		t.Fatalf("criação: %d %s", w.Code, w.Body.String())
	}
	// "01" casou com a filial gerencial 1; nome padrão = código quando falta.
	if n := contar(t, db, qm, f.EmpresaID); n != 4 {
		t.Errorf("membros: %d", n)
	}
	if n := contar(t, db, qa+` AND codigo='DIS' AND nome='DIS'`, f.EmpresaID); n != 1 {
		t.Errorf("nome padrão do agrupamento")
	}
	// A leitura do DRE por agrupamento enxerga as filiais do ERP: BAL = 1, 12, 14.
	var erps int
	db.QueryRow(`SELECT COUNT(*) FROM auditoria.agrupamentos_filiais a
		JOIN auditoria.agrupamentos_filiais_membros m ON m.agrupamento_id=a.id
		JOIN auditoria.filiais_gerenciais_erp e ON e.filial_gerencial_id=m.filial_gerencial_id
		WHERE a.empresa_id=$1::uuid AND a.codigo='BAL'`, f.EmpresaID).Scan(&erps)
	if erps != 3 {
		t.Errorf("filiais do ERP do BAL: %d", erps)
	}

	// Reenvio idempotente + aditivo (DIS ganha a 12, nome de DIS atualiza; BAL fica como está).
	out = resp{}
	w = up("controladoria", base)
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || out.Criados != 0 || out.MembrosAdicionados != 0 || out.Inalteradas != 4 {
		t.Fatalf("reenvio: %d %s", w.Code, w.Body.String())
	}
	out = resp{}
	w = up("controladoria", "agrupamento;filial;nome\nDIS;12;Distribuição\n")
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || out.MembrosAdicionados != 1 || out.Atualizados != 1 {
		t.Fatalf("aditivo: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, qm, f.EmpresaID); n != 5 {
		t.Errorf("membros: %d, esperado 5", n)
	}

	// Erros: filial gerencial inexistente (mensagem única), nome conflitante, GRUPO, código de filial gerencial.
	antes := contar(t, db, qm, f.EmpresaID)
	w = up("controladoria", "agrupamento;filial\nNOVO;99\nNOVO;98\nNOVO;99\n")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "filiais gerenciais inexistentes: 99, 98 (2 linha(s))") ||
		!strings.Contains(w.Body.String(), "Mapa de filiais") {
		t.Errorf("membro inexistente: %d %s", w.Code, w.Body.String())
	}
	w = up("controladoria", "agrupamento;nome;filial\nX;Um;1\nX;Dois;12\nGRUPO;G;1\n11;Igual a filial;1\n")
	for _, m := range []string{"nome diferente para o agrupamento X", "palavra reservada"} {
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), m) {
			t.Errorf("faltou %q: %d %s", m, w.Code, w.Body.String())
		}
	}
	w = up("controladoria", "agrupamento;filial\n11;1\n")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "já é o código de uma filial gerencial") {
		t.Errorf("código de filial gerencial: %d %s", w.Code, w.Body.String())
	}
	if contar(t, db, qm, f.EmpresaID) != antes || contar(t, db, qa+` AND codigo IN ('NOVO','X')`, f.EmpresaID) != 0 {
		t.Errorf("erro gravou")
	}
	if w = up("controladoria", "a;b\n1;2\n"); w.Code != http.StatusBadRequest {
		t.Errorf("cabeçalho: %d", w.Code)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='agrupamento_filiais' AND acao='carga'`, f.EmpresaID).Scan(&n)
	if n != 3 {
		t.Errorf("trilhas de carga: %d, esperado 3", n)
	}
}
