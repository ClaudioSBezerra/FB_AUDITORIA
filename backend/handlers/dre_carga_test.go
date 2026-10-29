package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

func uploadCarga(h http.Handler, f testdb.Fixture, perfil, url, nome string, conteudo []byte) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("arquivo", nome)
	part.Write(conteudo)
	mw.Close()
	r := httptest.NewRequest(http.MethodPost, url, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: perfil, Areas: []string{AreaEmpresa}}
	return chamar(h, r.WithContext(context.WithValue(r.Context(), AudContextKey, c)))
}

const estruturaOK = "codigo;descricao;nivel;ordem;tipo;soma_de;papel\n" +
	"R1;Receita;1;10;conta;;\n" +
	"R2;Devolucao;1;20;conta;;\n" +
	"SL;Subtotal liquido;0;30;subtotal;R1|R2;faturamento_liquido\n" +
	"D1;Despesa;1;40;conta;;\n" +
	"RES;Resultado;0;50;subtotal;SL,D1;\n"

func linhasAtivas(t *testing.T, f testdb.Fixture) int {
	return contar(t, testdb.Open(t), `SELECT COUNT(*) FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND ativo`, f.EmpresaID)
}

func TestCargaEstrutura_Fluxo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	h := DREEstruturaCargaHandler(db)
	const url = "/api/aud/dre/estrutura/carga"

	// Perfis sem a capacidade: 403 e nada gravado.
	for _, p := range []string{"diretoria", "auditoria", "gestor"} {
		if w := uploadCarga(h, f, p, url, "e.csv", []byte(estruturaOK)); w.Code != http.StatusForbidden {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	if linhasAtivas(t, f) != 0 {
		t.Fatal("403 gravou")
	}
	// Estrutura válida (CSV): 5 linhas, 2 subtotais, papel gravado.
	w := uploadCarga(h, f, "controladoria", url, "e.csv", []byte(estruturaOK))
	if w.Code != http.StatusOK {
		t.Fatalf("carga: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Linhas, Subtotais, Novas, Atualizadas, Desativadas int
		MesesFechados                                      string `json:"meses_fechados"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Linhas != 5 || out.Subtotais != 2 || out.Novas != 5 || out.Atualizadas != 0 || out.MesesFechados == "" {
		t.Errorf("resposta: %s", w.Body.String())
	}
	var soma, papel string
	if err := db.QueryRow(`SELECT array_to_string(soma_de,','), COALESCE(papel,'') FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND codigo='SL'`, f.EmpresaID).Scan(&soma, &papel); err != nil || soma != "R1,R2" || papel != "faturamento_liquido" {
		t.Errorf("SL: %v %q %q", err, soma, papel)
	}
	// Reenvio (XLSX), analista também pode: atualiza sem duplicar.
	x := xlsxDic(t, [][]any{{"Código", "Descrição", "Nível", "Ordem", "Tipo", "Soma de", "Papel"},
		{"R1", "Receita nova", 1, 10, "Conta", "", ""}, {"R2", "Devolucao", 1, 20, "conta", "", ""},
		{"SL", "Subtotal liquido", 0, 30, "subtotal", "R1,R2", "faturamento_liquido"}, {"D1", "Despesa", 1, 40, "conta", "", ""},
		{"RES", "Resultado", 0, 50, "subtotal", "SL|D1", ""}})
	if w = uploadCarga(h, f, "analista", url, "e.xlsx", x); w.Code != http.StatusOK {
		t.Fatalf("reenvio: %d %s", w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Novas != 0 || out.Atualizadas != 5 || linhasAtivas(t, f) != 5 {
		t.Errorf("reenvio: %+v ativas=%d", out, linhasAtivas(t, f))
	}
	// Outra empresa isolada.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid`, outra.EmpresaID); n != 0 {
		t.Errorf("outra empresa: %d", n)
	}

	// Erros bloqueantes: nada muda.
	casos := map[string]string{
		"subtotal inexistente": "codigo;descricao;tipo;soma_de\nA;A;conta;\nS;S;subtotal;A|ZZ\n",
		"ciclo":                "codigo;descricao;tipo;soma_de\nS1;S1;subtotal;S2\nS2;S2;subtotal;S1\n",
		"autorreferencia":      "codigo;descricao;tipo;soma_de\nS1;S1;subtotal;S1\n",
		"dois papeis":          "codigo;descricao;tipo;soma_de;papel\nA;A;conta;;faturamento_liquido\nB;B;conta;;faturamento_liquido\n",
		"codigo repetido":      "codigo;descricao;tipo\nA;A;conta\nA;B;conta\n",
		"tipo invalido":        "codigo;descricao;tipo\nA;A;grupo\n",
		"subtotal sem soma":    "codigo;descricao;tipo;soma_de\nA;A;subtotal;\n",
		"ordem nao inteira":    "codigo;descricao;tipo;ordem\nA;A;conta;1,5x\n",
		"papel desconhecido":   "codigo;descricao;tipo;papel\nA;A;conta;margem\n",
	}
	for nome, csv := range casos {
		w := uploadCarga(h, f, "controladoria", url, "e.csv", []byte(csv))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", nome, w.Code, w.Body.String())
		}
		if linhasAtivas(t, f) != 5 {
			t.Fatalf("%s gravou", nome)
		}
	}
	if w = uploadCarga(h, f, "controladoria", url, "e.csv", []byte("a;b\n1;2\n")); w.Code != http.StatusBadRequest {
		t.Errorf("cabeçalho inválido: %d", w.Code)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND codigo='ZZ'`, f.EmpresaID); n != 0 {
		t.Errorf("erro gravou")
	}
}

func TestCargaEstrutura_LinhaQueSaiComContaNoMapa(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	he, hm := DREEstruturaCargaHandler(db), DREMapaCargaHandler(db)
	if w := uploadCarga(he, f, "controladoria", "/e", "e.csv", []byte(estruturaOK)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := uploadCarga(hm, f, "controladoria", "/m", "m.csv", []byte("conta_padrao;linha_codigo;sinal\n3.1;R1;1\n4.%;D1;-1\n")); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	// Arquivo sem D1: 409 listando a linha, nada muda.
	sem := "codigo;descricao;tipo;soma_de\nR1;Receita;conta;\nR2;Dev;conta;\nSL;Liq;subtotal;R1|R2\n"
	w := uploadCarga(he, f, "controladoria", "/e", "e.csv", []byte(sem))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "D1 (1 conta(s))") {
		t.Fatalf("esperado 409: %d %s", w.Code, w.Body.String())
	}
	if linhasAtivas(t, f) != 5 {
		t.Error("409 gravou")
	}
	// D1 passando a subtotal também é recusado.
	w = uploadCarga(he, f, "controladoria", "/e", "e.csv", []byte("codigo;descricao;tipo;soma_de\nR1;R;conta;\nD1;D;subtotal;R1\n"))
	if w.Code != http.StatusConflict {
		t.Errorf("conta virando subtotal: %d", w.Code)
	}
	// Sem as contas no Mapa (substituído por outro Mapa só com R1), a linha pode sair; fica inativa, não apagada.
	if w := uploadCarga(hm, f, "controladoria", "/m", "m.csv", []byte("conta_padrao;linha_codigo\n3.1;R1\n")); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = uploadCarga(he, f, "controladoria", "/e", "e.csv", []byte(sem)); w.Code != 200 {
		t.Fatalf("saída liberada: %d %s", w.Code, w.Body.String())
	}
	if linhasAtivas(t, f) != 3 || contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid`, f.EmpresaID) != 5 {
		t.Errorf("ativas=%d", linhasAtivas(t, f))
	}
	if !strings.Contains(w.Body.String(), `"desativadas":2`) {
		t.Errorf("resposta: %s", w.Body.String())
	}
	// Sem papel no arquivo: o papel de SL (marcado antes) fica.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND papel='faturamento_liquido' AND codigo='SL' AND ativo`, f.EmpresaID); n != 1 {
		t.Errorf("papel não preservado")
	}
	// Reenvio com a linha de volta a reativa.
	if w = uploadCarga(he, f, "controladoria", "/e", "e.csv", []byte(estruturaOK)); w.Code != 200 || linhasAtivas(t, f) != 5 {
		t.Errorf("reativação: %d %d", w.Code, linhasAtivas(t, f))
	}
}

func TestCargaMapa_Fluxo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	hm := DREMapaCargaHandler(db)
	const url = "/api/aud/dre/mapa/carga"
	if w := uploadCarga(DREEstruturaCargaHandler(db), f, "controladoria", "/e", "e.csv", []byte(estruturaOK)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if _, err := db.Exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'9.9','R1',1),($1::uuid,'3.1','R2',-1)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) SELECT $1::uuid,'3.1','X',1`, outra.EmpresaID); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{"diretoria", "auditoria", "gestor"} {
		if w := uploadCarga(hm, f, p, url, "m.csv", []byte("conta_padrao;linha_codigo\n1;R1\n")); w.Code != http.StatusForbidden {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	// Erros: linha inexistente, subtotal como destino, sinal inválido, conta repetida.
	for nome, csv := range map[string]string{
		"linha inexistente":  "conta_padrao;linha_codigo\n1;NAO\n",
		"linha subtotal":     "conta_padrao;linha_codigo\n1;SL\n",
		"sinal invalido":     "conta_padrao;linha_codigo;sinal\n1;R1;2\n",
		"conta repetida":     "conta_padrao;linha_codigo\n1;R1\n1;R2\n",
		"percentual no meio": "conta_padrao;linha_codigo\n1%2;R1\n",
	} {
		w := uploadCarga(hm, f, "controladoria", url, "m.csv", []byte(csv))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", nome, w.Code, w.Body.String())
		}
		if contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_mapa WHERE empresa_id=$1::uuid`, f.EmpresaID) != 2 {
			t.Fatalf("%s mexeu no mapa", nome)
		}
	}
	// Substitui: 9.9 sai, 3.1 muda, 1.% entra (prefixo), 5 entra.
	w := uploadCarga(hm, f, "analista", url, "m.csv", []byte("conta_padrao;linha_codigo;sinal\n3.1;R1;1\n1.%;D1;-1\n5;R2;\n"))
	if w.Code != http.StatusOK {
		t.Fatalf("carga: %d %s", w.Code, w.Body.String())
	}
	var out struct{ Linhas, Inseridas, Atualizadas, Removidas int }
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Linhas != 3 || out.Inseridas != 2 || out.Atualizadas != 1 || out.Removidas != 1 || !strings.Contains(w.Body.String(), "fechados") {
		t.Errorf("contagens: %s", w.Body.String())
	}
	var l string
	var s int
	db.QueryRow(`SELECT linha_codigo, sinal FROM auditoria.dre_mapa WHERE empresa_id=$1::uuid AND conta_padrao='1.%'`, f.EmpresaID).Scan(&l, &s)
	if l != "D1" || s != -1 {
		t.Errorf("prefixo %%: %s %d", l, s)
	}
	db.QueryRow(`SELECT sinal FROM auditoria.dre_mapa WHERE empresa_id=$1::uuid AND conta_padrao='5'`, f.EmpresaID).Scan(&s)
	if s != 1 {
		t.Errorf("sinal vazio: %d", s)
	}
	if contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_mapa WHERE empresa_id=$1::uuid`, outra.EmpresaID) != 1 {
		t.Error("outra empresa afetada")
	}
	// Mais específico vence (regra do Mapa existente).
	mapa, _ := carregarMapa(db, f.EmpresaID)
	if m, ok := resolverTeste(mapa, "1.2.3"); !ok || m != "D1" {
		t.Errorf("resolução: %v %v", m, ok)
	}
	// Trilha: contagens e códigos, nunca valores; 1 entrada por carga com sucesso.
	var payload string
	if err := db.QueryRow(`SELECT payload::text FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dre_mapa' AND acao='carregar' ORDER BY id DESC LIMIT 1`, f.EmpresaID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	for _, quer := range []string{`"removidas": 1`, `"inseridas": 2`, "meses_fechados", "contas_removidas"} {
		if !strings.Contains(payload, quer) {
			t.Errorf("trilha sem %s: %s", quer, payload)
		}
	}
	if strings.Contains(payload, "valor") {
		t.Errorf("trilha com valor: %s", payload)
	}
	if contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade IN ('dre_mapa','dre_estrutura') AND acao='carregar'`, f.EmpresaID) != 2 {
		t.Error("trilha: esperava 1 da estrutura e 1 do Mapa (erros não gravam)")
	}
}

func TestSemMapa_SomaECapacidade(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	if _, err := db.Exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'3.%','RB',1)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	for _, l := range [][]any{{"3.1", "mapeada", 1000.00, "2026-09-01"}, {"9.9", "Sem um", 50.25, "2026-09-01"}, {"9.9", "Sem um", -10.00, "2026-09-01"},
		{"8.1", "Sem dois", -300.00, "2026-09-01"}, {"7.7", "Outro mes", 5.00, "2026-10-01"}} {
		if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, conta_desc, valor) VALUES ($1::uuid,$4,$2,$3,$5)`,
			f.EmpresaID, l[0], l[1], l[3], l[2]); err != nil {
			t.Fatal(err)
		}
	}
	db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, valor) VALUES ($1::uuid,'2026-09-01','6.6',99)`, outra.EmpresaID)
	h := DREMapaSemMapaHandler(db)
	const url = "/api/aud/dre/mapa/sem-mapa?ano=2026&mes=9"
	for _, p := range []string{"gestor"} {
		if w := chamar(h, reqFilial(http.MethodGet, url, "", f, p)); w.Code != http.StatusForbidden {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	w := chamar(h, reqFilial(http.MethodGet, url, "", f, "diretoria"))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var r struct {
		Total int
		Soma  float64
		Itens []struct {
			Conta, Descricao string
			Valor            float64
		}
	}
	json.Unmarshal(w.Body.Bytes(), &r)
	if r.Total != 2 || r.Soma != -259.75 || r.Itens[0].Conta != "8.1" || r.Itens[1].Conta != "9.9" || r.Itens[1].Valor != 40.25 || r.Itens[1].Descricao != "Sem um" {
		t.Errorf("sem mapa: %s", w.Body.String())
	}
	// Limite e CSV.
	w = chamar(h, reqFilial(http.MethodGet, url+"&limite=1", "", f, "auditoria"))
	if !strings.Contains(w.Body.String(), `"truncado":true`) || !strings.Contains(w.Body.String(), `"total":2`) {
		t.Errorf("limite: %s", w.Body.String())
	}
	w = chamar(h, reqFilial(http.MethodGet, url+"&formato=csv", "", f, "controladoria"))
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") || !strings.Contains(w.Body.String(), "conta;descricao;valor") || !strings.Contains(w.Body.String(), "9.9;Sem um;40,25") {
		t.Errorf("csv: %s", w.Body.String())
	}
	// Mês sem lançamento sem mapa; outro mês; período inválido.
	if w = chamar(h, reqFilial(http.MethodGet, "/x?ano=2026&mes=8", "", f, "controladoria")); !strings.Contains(w.Body.String(), `"total":0`) {
		t.Errorf("agosto: %s", w.Body.String())
	}
	if w = chamar(h, reqFilial(http.MethodGet, "/x?ano=2026&mes=13", "", f, "controladoria")); w.Code != 400 {
		t.Errorf("inválido: %d", w.Code)
	}
	// Acumulado soma outubro.
	w = chamar(h, reqFilial(http.MethodGet, "/x?ano=2026&mes=10&visao=acumulada", "", f, "controladoria"))
	if !strings.Contains(w.Body.String(), `"total":3`) {
		t.Errorf("acumulado: %s", w.Body.String())
	}
	// Com as contas no Mapa a lista esvazia.
	if _, err := db.Exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'9.9','RB',1),($1::uuid,'8.1','RB',1)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if w = chamar(h, reqFilial(http.MethodGet, url, "", f, "controladoria")); !strings.Contains(w.Body.String(), `"total":0`) {
		t.Errorf("com mapa: %s", w.Body.String())
	}
}

func resolverTeste(mapa []dreMapaItem, conta string) (string, bool) {
	m, ok := dre.ResolverConta(mapa, conta)
	return m.LinhaCodigo, ok
}
