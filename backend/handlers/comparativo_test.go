package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

// Dados fictícios. Mensal e Real em setembro de 2026.
type fxAnalise struct {
	anoC2     any
	visao     string
	d1        any
	mensal    [][]any // cada item: chave, descrição, valor de setembro (nil = vazio)
	real      [][]any // conta, valor, mês, loja
	semMensal bool
	semReal   bool
}

func xlsxAnalise(t testing.TB, o fxAnalise) []byte {
	t.Helper()
	x := excelize.NewFile()
	x.SetSheetName("Sheet1", "Outra")
	if !o.semMensal {
		_, _ = x.NewSheet("MENSAL")
		if o.anoC2 != nil {
			_ = x.SetCellValue("MENSAL", "C2", o.anoC2)
		}
		_ = x.SetCellValue("MENSAL", "C3", o.visao)
		if o.d1 != nil {
			_ = x.SetCellValue("MENSAL", "D1", o.d1)
		}
		_ = x.SetCellValue("MENSAL", "A4", "GRUPO")
		for i, l := range o.mensal {
			r := 5 + i
			if l[0] != "" {
				_ = x.SetCellValue("MENSAL", cel(1, r), l[0])
			}
			_ = x.SetCellValue("MENSAL", cel(3, r), l[1])
			if l[2] != nil {
				_ = x.SetCellValue("MENSAL", cel(12, r), l[2]) // L = setembro
			}
			_ = x.SetCellValue("MENSAL", cel(17, r), 0.5) // percentuais (Q) não são lidos
		}
	}
	if !o.semReal {
		_, _ = x.NewSheet("Real_26")
		_ = x.SetSheetRow("Real_26", "A1", &[]any{"CONTA", "VALOR", "MÊS", "LOJA"})
		for i, l := range o.real {
			_ = x.SetSheetRow("Real_26", cel(1, 2+i), &l)
		}
	}
	var buf bytes.Buffer
	if err := x.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func cel(c, r int) string {
	n, _ := excelize.CoordinatesToCellName(c, r)
	return n
}

func analisePadrao(t testing.TB) []byte {
	return xlsxAnalise(t, fxAnalise{
		anoC2: 2026, visao: "Filial de teste", d1: 1,
		mensal: [][]any{
			{"R1", "Receita", 1000.5},            // sistema 1000: dentro da tolerância
			{"R2", "Despesa", -250.0},            // sistema -300: diverge
			{"", "(=) Resultado Líquido", 700.5}, // subtotal sem chave: casa pela descrição
			{"X9", "Linha só da planilha", 10.0},
			{"", "", nil},
			{"VAZ", "Linha sem valor", nil},
		},
		real: [][]any{
			{"4010001", 600.0, 202609, "01"},
			{4010001, 400.0, 202609.0, 1},
			{"5010001", 100.0, "202609", "1"},
			{"5010001.0", 150.0, 202609, "001"},
			{"7777", 5.0, 202609, 1},
			{"", 9.0, 202609, 1},        // sem conta: ignorada
			{"4010001", 9.0, nil, 1},    // sem mês: ignorada
			{"4010001", 9.0, 202509, 1}, // mês de outro ano: ignorada
			{"4010001", 30.0, 202609, 2},
		},
	})
}

func prepararComparativo(t *testing.T, db *sql.DB, f testdb.Fixture) {
	t.Helper()
	exec := func(q string, a ...any) {
		t.Helper()
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	e := f.EmpresaID
	exec(`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo) VALUES
		($1::uuid,'R1','Receita',1,1,'conta'), ($1::uuid,'R2','Despesa',1,2,'conta'), ($1::uuid,'D3','Terceira',1,4,'conta')`, e)
	exec(`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo, soma_de) VALUES
		($1::uuid,'SL','Resultado liquido',0,3,'subtotal','{R1,R2}')`, e)
	exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES
		($1::uuid,'4%','R1',1), ($1::uuid,'5%','R2',-1), ($1::uuid,'6%','D3',1)`, e)
	ins := func(origem, filial, conta string, valor float64) {
		exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, conta_desc, valor, origem)
			VALUES ($1::uuid,'2026-09-01',$2,$3,'desc '||$3,$4,$5)`, e, filial, conta, valor, origem)
	}
	ins("csv", "1", "4010001", 1000)
	ins("csv", "1", "5010001", 300)
	ins("csv", "1", "6010001", 40) // só no sistema
	ins("csv", "2", "4010001", 30)
}

func uploadAnalise(t *testing.T, db *sql.DB, f testdb.Fixture, perfil, query string, x []byte) *httpResp {
	t.Helper()
	w := uploadCarga(ComparativoCargaHandler(db), f, perfil, "/api/aud/comparativo/carga"+query, "analise.xlsx", x)
	return &httpResp{w.Code, w.Body.String(), w.Header().Get("Content-Type")}
}

func carregarPadrao(t *testing.T, db *sql.DB, f testdb.Fixture) dre.CargaExternaOut {
	t.Helper()
	r := uploadAnalise(t, db, f, "controladoria", "", analisePadrao(t))
	if r.Code != http.StatusOK {
		t.Fatalf("carga: %d %s", r.Code, r.Body)
	}
	var out dre.CargaExternaOut
	if err := json.Unmarshal([]byte(r.Body), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func chamarComp(t *testing.T, db *sql.DB, f testdb.Fixture, perfil string, kv ...string) *httpResp {
	t.Helper()
	v := url.Values{"ano": {"2026"}, "mes": {"9"}}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	w := chamar(ComparativoHandler(db), reqFilial(http.MethodGet, "/api/aud/comparativo?"+v.Encode(), "", f, perfil))
	return &httpResp{w.Code, w.Body.String(), w.Header().Get("Content-Type")}
}

func lerComp(t *testing.T, r *httpResp) dre.Comparativo {
	t.Helper()
	if r.Code != http.StatusOK {
		t.Fatalf("status %d: %s", r.Code, r.Body)
	}
	var c dre.Comparativo
	if err := json.Unmarshal([]byte(r.Body), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func porChave(c dre.Comparativo) map[string]dre.ItemComparativo {
	m := map[string]dre.ItemComparativo{}
	for _, it := range c.Linhas {
		m[it.Chave] = it
	}
	return m
}

func TestComparativo_CargaGravaContasELinhas(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	out := carregarPadrao(t, db, f)
	// contas: (4010001, filial 1), (5010001, 1), (7777, 1), (4010001, 2) = 4; linhas: 3 com valor em setembro
	if out.Contas != 4 || out.Linhas != 4 || out.Meses != 1 || out.Filiais != 2 || out.Ignoradas != 3 || out.Ano != 2026 || out.Filial != "1" {
		t.Errorf("resposta: %+v", out)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.comparativo_externo_contas WHERE empresa_id=$1::uuid AND filial='1' AND competencia='2026-09-01'`, f.EmpresaID); n != 3 {
		t.Errorf("contas da filial 1: %d", n)
	}
	var v float64
	if err := db.QueryRow(`SELECT valor FROM auditoria.comparativo_externo_contas WHERE empresa_id=$1::uuid AND filial='1' AND conta='5010001'`, f.EmpresaID).Scan(&v); err != nil || v != 250 {
		t.Errorf("conta 5010001: %v %v (a 5010001.0 e a 5010001 somam; loja 1, 01 e 001 são a mesma)", v, err)
	}
	var chave string
	if err := db.QueryRow(`SELECT linha_chave FROM auditoria.comparativo_externo_linhas WHERE empresa_id=$1::uuid AND descricao LIKE '%Resultado%'`, f.EmpresaID).Scan(&chave); err != nil || chave != "resultado_liquido" {
		t.Errorf("chave do subtotal: %q %v", chave, err)
	}
	// Nada entrou em lancamentos.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("a planilha não pode entrar em lancamentos: %d", n)
	}
	// Trilha: só contagens.
	var payload string
	if err := db.QueryRow(`SELECT payload::text FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='comparativo_externo' AND acao='carregar'`, f.EmpresaID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	for _, proibido := range []string{"1000", "250", "Receita", "4010001", "valor"} {
		if strings.Contains(payload, proibido) {
			t.Errorf("trilha com dado da planilha (%q): %s", proibido, payload)
		}
	}
	if !strings.Contains(payload, `"contas": 4`) && !strings.Contains(payload, `"contas":4`) {
		t.Errorf("trilha sem contagem: %s", payload)
	}
}

func TestComparativo_ReenvioSubstituiOAno(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	carregarPadrao(t, db, f)
	x := xlsxAnalise(t, fxAnalise{anoC2: 2026, visao: "v", d1: "01",
		mensal: [][]any{{"R1", "Receita", 5.0}},
		real:   [][]any{{"4010001", 7.0, 202609, 1}}})
	if r := uploadAnalise(t, db, f, "analista", "", x); r.Code != http.StatusOK {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.comparativo_externo_contas WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 1 {
		t.Errorf("contas após o reenvio: %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.comparativo_externo_linhas WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 1 {
		t.Errorf("linhas após o reenvio: %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.comparativo_externo_cargas WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 2 {
		t.Errorf("cargas: %d", n)
	}
}

func TestComparativo_MensalSemFilialResolvidaGeraAviso(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	prepararComparativo(t, db, f)
	x := xlsxAnalise(t, fxAnalise{anoC2: 2026, visao: "GRUPO", d1: "#N/D",
		mensal: [][]any{{"R1", "Receita", 5.0}}, real: [][]any{{"4010001", 7.0, 202609, 1}}})
	r := uploadAnalise(t, db, f, "controladoria", "", x)
	if r.Code != http.StatusOK {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	var out dre.CargaExternaOut
	json.Unmarshal([]byte(r.Body), &out)
	if out.Filial != "" || out.Visao != "GRUPO" || len(out.Avisos) == 0 || !strings.Contains(out.Avisos[0], "filial") {
		t.Errorf("avisos: %+v", out)
	}
	// Por linha sem filial resolvida: aviso, nenhuma linha.
	c := lerComp(t, chamarComp(t, db, f, "controladoria", "nivel", "linha"))
	if len(c.Linhas) != 0 || len(c.Avisos) == 0 {
		t.Errorf("comparativo por linha sem filial: %+v", c)
	}
	// Por conta continua funcionando com a filial informada.
	c = lerComp(t, chamarComp(t, db, f, "controladoria", "nivel", "conta", "filial", "1"))
	if c.Resumo.OK+c.Resumo.Diverge+c.Resumo.SoPlanilha+c.Resumo.SoSistema == 0 {
		t.Errorf("por conta vazio")
	}
}

func TestComparativo_CargaErros(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if r := uploadAnalise(t, db, f, "controladoria", "", xlsxAnalise(t, fxAnalise{anoC2: 2026, semMensal: true})); r.Code != http.StatusBadRequest || !strings.Contains(r.Body, "MENSAL") {
		t.Errorf("sem MENSAL: %d %s", r.Code, r.Body)
	}
	if r := uploadAnalise(t, db, f, "controladoria", "", xlsxAnalise(t, fxAnalise{anoC2: 2026, d1: 1, semReal: true})); r.Code != http.StatusBadRequest || !strings.Contains(r.Body, "Real_26") {
		t.Errorf("sem Real: %d %s", r.Code, r.Body)
	}
	if r := uploadAnalise(t, db, f, "controladoria", "", []byte("não é xlsx")); r.Code != http.StatusBadRequest {
		t.Errorf("lixo: %d %s", r.Code, r.Body)
	}
	// ano pedido sem aba
	if r := uploadAnalise(t, db, f, "controladoria", "?ano=2024", analisePadrao(t)); r.Code != http.StatusOK && r.Code != http.StatusBadRequest {
		t.Errorf("ano pedido: %d", r.Code)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.comparativo_externo_cargas WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("erro não pode gravar: %d", n)
	}
}

func TestComparativo_PorConta(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	prepararComparativo(t, db, f)
	carregarPadrao(t, db, f)
	// outra empresa não aparece
	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor, origem)
		VALUES ($1::uuid,'2026-09-01','1','9999',777,'csv')`, outra.EmpresaID); err != nil {
		t.Fatal(err)
	}
	// uma segunda origem para a conta de receita
	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor, origem)
		VALUES ($1::uuid,'2026-09-01','01','4010001',50,'outra')`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}

	c := lerComp(t, chamarComp(t, db, f, "controladoria", "nivel", "conta", "filial", "1", "origem", "csv"))
	m := porChave(c)
	if m["4010001"].Status != "OK" || m["5010001"].Status != "DIVERGE" || m["7777"].Status != "SO_PLANILHA" || m["6010001"].Status != "SO_SISTEMA" {
		t.Fatalf("status: %+v", m)
	}
	if _, ok := m["9999"]; ok {
		t.Error("outra empresa vazou")
	}
	if d := m["5010001"].Diferenca; d != -50 {
		t.Errorf("diferença da 5010001: %v", d)
	}
	r := c.Resumo
	if r.OK != 1 || r.Diverge != 1 || r.SoPlanilha != 1 || r.SoSistema != 1 || r.SomaDiferencas != 95 || !r.TotaisValidos {
		t.Errorf("resumo: %+v", r)
	}
	// ordem: DIVERGE, SO_PLANILHA, SO_SISTEMA, OK
	var ordem []string
	for _, it := range c.Linhas {
		ordem = append(ordem, it.Status)
	}
	if strings.Join(ordem, ",") != "DIVERGE,SO_PLANILHA,SO_SISTEMA,OK" {
		t.Errorf("ordem: %v", ordem)
	}
	if c.Carga == nil || c.Carga.Arquivo != "analise.xlsx" || c.Carga.Usuario == "" || c.Fechada {
		t.Errorf("carga/fechada: %+v %v", c.Carga, c.Fechada)
	}
	// sem filtro de origem a receita soma as duas origens (1050 contra 1000): diverge
	c = lerComp(t, chamarComp(t, db, f, "controladoria", "nivel", "conta", "filial", "01"))
	if it := porChave(c)["4010001"]; it.Status != "DIVERGE" || it.Diferenca != -50 {
		t.Errorf("todas as origens: %+v", it)
	}
	// tolerância maior absorve
	c = lerComp(t, chamarComp(t, db, f, "controladoria", "nivel", "conta", "filial", "1", "tolerancia", "60,00"))
	if it := porChave(c)["4010001"]; it.Status != "OK" {
		t.Errorf("tolerância 60: %+v", it)
	}
	// filtro de status e paginação
	c = lerComp(t, chamarComp(t, db, f, "controladoria", "nivel", "conta", "filial", "1", "origem", "csv", "status", "DIVERGE,SO_PLANILHA", "limite", "1"))
	if c.Total != 2 || len(c.Linhas) != 1 || c.Linhas[0].Status != "DIVERGE" || c.Resumo.OK+c.Resumo.SoSistema == 0 {
		t.Errorf("filtro: total %d, linhas %d, resumo %+v", c.Total, len(c.Linhas), c.Resumo)
	}
	// filial obrigatória no nível conta
	if r := chamarComp(t, db, f, "controladoria", "nivel", "conta"); r.Code != http.StatusBadRequest {
		t.Errorf("sem filial: %d", r.Code)
	}
	if r := chamarComp(t, db, f, "controladoria", "nivel", "xx"); r.Code != http.StatusBadRequest {
		t.Errorf("nivel inválido: %d", r.Code)
	}
	// ano sem carga: aviso
	c = lerComp(t, chamarComp(t, db, f, "controladoria", "nivel", "conta", "filial", "1", "ano", "2025"))
	if len(c.Avisos) == 0 || c.Total != 0 {
		t.Errorf("ano sem carga: %+v", c)
	}
}

func TestComparativo_PorLinha(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	prepararComparativo(t, db, f)
	carregarPadrao(t, db, f)
	c := lerComp(t, chamarComp(t, db, f, "controladoria", "nivel", "linha")) // filial resolvida da carga
	if c.Filial != "1" {
		t.Fatalf("filial: %q", c.Filial)
	}
	m := porChave(c)
	if m["R1"].Status != "OK" || m["R2"].Status != "DIVERGE" || m["resultado_liquido"].Status != "OK" ||
		m["X9"].Status != "SO_PLANILHA" || m["D3"].Status != "SO_SISTEMA" {
		t.Fatalf("status: %+v", m)
	}
	if !m["resultado_liquido"].Subtotal {
		t.Error("subtotal não marcado")
	}
	if s := m["R2"].Sistema; s == nil || *s != -300 {
		t.Errorf("R2 sistema (sinal do Mapa): %v", s)
	}
	if m["R2"].Diferenca != 50 {
		t.Errorf("R2 diferença: %v", m["R2"].Diferenca)
	}
	if c.Resumo.TotaisValidos || c.Resumo.Diverge != 1 || c.Resumo.SoPlanilha != 1 || c.Resumo.SoSistema != 1 || c.Resumo.OK != 2 {
		t.Errorf("resumo: %+v", c.Resumo)
	}
	if _, ok := m["VAZ"]; ok {
		t.Error("linha sem valor não deveria ser gravada")
	}

	// Mês fechado lê do snapshot: mudar o lançamento depois do fechamento não muda a
	// leitura por linha, mas muda o nível conta (que lê lançamentos).
	if _, err := (&dre.Fechamento{DB: db}).Fechar(context.Background(), dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9}); err != nil {
		t.Fatalf("fechar: %v", err)
	}
	if _, err := db.Exec(`UPDATE auditoria.lancamentos SET valor = valor + 500 WHERE empresa_id=$1::uuid AND conta='4010001' AND filial='1'`, f.EmpresaID); err != nil {
		t.Skipf("não foi possível alterar lançamento fechado (guarda do banco): %v", err)
	}
	c = lerComp(t, chamarComp(t, db, f, "controladoria", "nivel", "linha"))
	if !c.Fechada || porChave(c)["R1"].Status != "OK" {
		t.Errorf("mês fechado deve ler do snapshot: fechada=%v %+v", c.Fechada, porChave(c)["R1"])
	}
	c = lerComp(t, chamarComp(t, db, f, "controladoria", "nivel", "conta", "filial", "1"))
	if porChave(c)["4010001"].Status != "DIVERGE" {
		t.Errorf("nível conta lê lançamentos: %+v", porChave(c)["4010001"])
	}
}

func TestComparativo_Permissoes(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	prepararComparativo(t, db, f)
	carregarPadrao(t, db, f)
	// Gestor não lê: 403 registrado.
	for _, p := range []string{"gestor"} {
		if r := chamarComp(t, db, f, p, "nivel", "linha"); r.Code != http.StatusForbidden {
			t.Errorf("%s: %d", p, r.Code)
		}
		w := chamar(ComparativoPeriodosHandler(db), reqFilial(http.MethodGet, "/api/aud/comparativo/periodos", "", f, p))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s periodos: %d", p, w.Code)
		}
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='comparativo_externo' AND acao='negado'`, f.EmpresaID); n != 2 {
		t.Errorf("negados na trilha: %d", n)
	}
	// Carga sem a capacidade: diretoria, auditoria e gestor recebem 403 e nada é gravado de novo.
	antes := contar(t, db, `SELECT COUNT(*) FROM auditoria.comparativo_externo_cargas WHERE empresa_id=$1::uuid`, f.EmpresaID)
	for _, p := range []string{"diretoria", "auditoria", "gestor"} {
		if r := uploadAnalise(t, db, f, p, "", analisePadrao(t)); r.Code != http.StatusForbidden {
			t.Errorf("carga por %s: %d", p, r.Code)
		}
	}
	if contar(t, db, `SELECT COUNT(*) FROM auditoria.comparativo_externo_cargas WHERE empresa_id=$1::uuid`, f.EmpresaID) != antes {
		t.Error("403 gravou")
	}
	// Diretoria lê.
	if r := chamarComp(t, db, f, "diretoria", "nivel", "linha"); r.Code != http.StatusOK {
		t.Errorf("diretoria lê: %d", r.Code)
	}
}

func TestComparativo_OutraEmpresaIsolada(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	prepararComparativo(t, db, f)
	carregarPadrao(t, db, f)
	c := lerComp(t, chamarComp(t, db, outra, "controladoria", "nivel", "conta", "filial", "1"))
	if c.Total != 0 || c.Carga != nil {
		t.Errorf("outra empresa enxergou a planilha: %+v", c)
	}
	var p struct {
		Contas []struct {
			Ano int `json:"ano"`
		} `json:"contas"`
		Cargas []any `json:"cargas"`
	}
	w := chamar(ComparativoPeriodosHandler(db), reqFilial(http.MethodGet, "/api/aud/comparativo/periodos", "", outra, "controladoria"))
	json.Unmarshal(w.Body.Bytes(), &p)
	if len(p.Contas) != 0 || len(p.Cargas) != 0 {
		t.Errorf("periodos da outra empresa: %s", w.Body.String())
	}
	// A carga da outra empresa não apaga a primeira.
	if r := uploadAnalise(t, db, outra, "controladoria", "", analisePadrao(t)); r.Code != http.StatusOK {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.comparativo_externo_contas WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 4 {
		t.Errorf("contas da primeira: %d", n)
	}
}

func TestComparativo_Periodos(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	carregarPadrao(t, db, f)
	w := chamar(ComparativoPeriodosHandler(db), reqFilial(http.MethodGet, "/api/aud/comparativo/periodos", "", f, "controladoria"))
	if w.Code != http.StatusOK {
		t.Fatalf("%d", w.Code)
	}
	var p struct {
		Contas, Linhas []struct {
			Ano     int      `json:"ano"`
			Filiais []string `json:"filiais"`
			Meses   []string `json:"meses"`
		}
		Cargas []struct {
			Arquivo string `json:"arquivo"`
			Ano     int    `json:"ano"`
			Filial  string `json:"filial"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Contas) != 1 || p.Contas[0].Ano != 2026 || len(p.Contas[0].Filiais) != 2 || p.Contas[0].Meses[0] != "2026-09" {
		t.Errorf("contas: %+v", p.Contas)
	}
	if len(p.Linhas) != 1 || len(p.Linhas[0].Filiais) != 1 || p.Linhas[0].Filiais[0] != "1" {
		t.Errorf("linhas: %+v", p.Linhas)
	}
	if len(p.Cargas) != 1 || p.Cargas[0].Arquivo != "analise.xlsx" || p.Cargas[0].Filial != "1" {
		t.Errorf("cargas: %+v", p.Cargas)
	}
}

func TestComparativo_CSVComProtecaoETrilha(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	prepararComparativo(t, db, f)
	x := xlsxAnalise(t, fxAnalise{anoC2: 2026, visao: "v", d1: 1,
		mensal: [][]any{{"=CMD", "=HYPERLINK(\"x\")", -5.0}, {"R1", "Receita", 1000.0}},
		real:   [][]any{{"=4010001", -7.5, 202609, 1}}})
	if r := uploadAnalise(t, db, f, "controladoria", "", x); r.Code != http.StatusOK {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	r := chamarComp(t, db, f, "controladoria", "nivel", "linha", "formato", "csv")
	if r.Code != http.StatusOK || !strings.HasPrefix(r.Tipo, "text/csv") || !strings.HasPrefix(r.Body, "\xef\xbb\xbf") {
		t.Fatalf("csv: %d %q", r.Code, r.Tipo)
	}
	if !strings.Contains(r.Body, "'=CMD") || !strings.Contains(r.Body, "'=HYPERLINK") {
		t.Errorf("texto com = sem proteção:\n%s", r.Body)
	}
	if strings.Contains(r.Body, "'-5") || !strings.Contains(r.Body, ";-5,00;") {
		t.Errorf("número negativo não pode ganhar apóstrofo:\n%s", r.Body)
	}
	r = chamarComp(t, db, f, "controladoria", "nivel", "conta", "filial", "1", "formato", "csv")
	if !strings.Contains(r.Body, "'=4010001") {
		t.Errorf("conta com = sem proteção:\n%s", r.Body)
	}
	// Trilha: visualizar/exportar com parâmetros, sem valores.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='comparativo_externo' AND acao='exportar'
		AND payload->>'nivel' IN ('linha','conta') AND payload ? 'tolerancia'`, f.EmpresaID).Scan(&n); err != nil || n != 2 {
		t.Errorf("trilha exportar: %d %v", n, err)
	}
	chamarComp(t, db, f, "controladoria", "nivel", "linha")
	var payload string
	if err := db.QueryRow(`SELECT payload::text FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='comparativo_externo' AND acao='visualizar'`, f.EmpresaID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, "1000") || strings.Contains(payload, "Receita") {
		t.Errorf("trilha com valor: %s", payload)
	}
}

// Planilha grande: milhares de linhas na aba Real são lidas em fluxo e o resultado
// está certo (a memória não cresce com o arquivo inteiro: a leitura é por linha).
func TestComparativo_CargaGrandeEmFluxo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	const n = 20000
	real := make([][]any, 0, n)
	for i := 0; i < n; i++ {
		real = append(real, []any{4000000 + i%500, 1.0, 202601 + i%12, 1 + i%3})
	}
	x := xlsxAnalise(t, fxAnalise{anoC2: 2026, visao: "v", d1: 1, mensal: [][]any{{"R1", "Receita", 1.0}}, real: real})
	var antes, depois runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&antes)
	r := uploadAnalise(t, db, f, "controladoria", "", x)
	runtime.ReadMemStats(&depois)
	if r.Code != http.StatusOK {
		t.Fatalf("%d %s", r.Code, r.Body)
	}
	var out dre.CargaExternaOut
	json.Unmarshal([]byte(r.Body), &out)
	if out.Ignoradas != 0 || out.Contas == 0 || out.Meses != 12 || out.Filiais != 3 {
		t.Errorf("resposta: %+v", out)
	}
	var soma float64
	if err := db.QueryRow(`SELECT SUM(valor)::float8 FROM auditoria.comparativo_externo_contas WHERE empresa_id=$1::uuid`, f.EmpresaID).Scan(&soma); err != nil || soma != n {
		t.Errorf("soma %v (%v)", soma, err)
	}
	// Limite folgado (o teste existe para pegar uma regressão para GetRows em tudo).
	if mb := (depois.TotalAlloc - antes.TotalAlloc) >> 20; mb > 600 {
		t.Errorf("a carga alocou %d MB", mb)
	}
}

func TestComparativo_LimpezaDeTesteCobreAsTabelas(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := db.Exec(`UPDATE companies SET ambiente='teste' WHERE id=$1::uuid`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	carregarPadrao(t, db, f)
	var js []byte
	if err := db.QueryRow(`SELECT auditoria.limpar_dados_teste($1::uuid)`, f.EmpresaID).Scan(&js); err != nil {
		t.Fatal(err)
	}
	var m map[string]int
	json.Unmarshal(js, &m)
	if m["comparativo_externo_contas"] != 4 || m["comparativo_externo_linhas"] != 4 || m["comparativo_externo_cargas"] != 1 {
		t.Errorf("contagens da limpeza: %s", js)
	}
	for _, tab := range []string{"cargas", "contas", "linhas"} {
		if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.comparativo_externo_`+tab+` WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
			t.Errorf("%s ainda tem %d", tab, n)
		}
	}
}
