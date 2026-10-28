package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fb_auditoria/internal/testdb"
)

// História 4.7: escopo por departamento que nega por padrão. Cenário fictício de
// cenarioDep: D1 (3.1, 4.1), D2 (4.% por prefixo: 4.2), 3.2 e 5.1 sem departamento.

func gestorCom(t *testing.T, db *sql.DB, f testdb.Fixture, codigos ...string) string {
	t.Helper()
	gid := usuarioNaEmpresa(t, db, f, "gestor")
	for _, c := range codigos {
		if _, err := db.Exec(`INSERT INTO auditoria.usuario_departamentos (user_id, departamento_id, empresa_id)
			SELECT $1::uuid, id, empresa_id FROM auditoria.departamentos WHERE empresa_id=$2::uuid AND codigo=$3`, gid, f.EmpresaID, c); err != nil {
			t.Fatal(err)
		}
	}
	return gid
}

func reqDe(metodo, url string, empresaID, userID, perfil string) *http.Request {
	r := httptest.NewRequest(metodo, url, nil)
	c := &AudContext{UserID: userID, EmpresaID: empresaID, Perfil: perfil, Areas: []string{AreaEmpresa}}
	return r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
}

func visaoDoGestor(t *testing.T, db *sql.DB, empresaID, gid, url string) (int, visaoDep, string) {
	t.Helper()
	w := chamar(DREDepartamentosHandler(db), reqDe(http.MethodGet, url, empresaID, gid, "gestor"))
	var v visaoDep
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, v, w.Body.String()
}

func codigosDaVisao(v visaoDep) string {
	var cs []string
	for _, g := range v.Grupos {
		for _, d := range g.Departamentos {
			cs = append(cs, d.Codigo)
		}
	}
	return strings.Join(cs, ",")
}

const urlDepSet = "/x?ano=2026&mes=9&visao=mensal"

func TestEscopo_GestorComAeBVeSoAeB(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	gid := gestorCom(t, db, f, "D1", "D2")
	code, v, corpo := visaoDoGestor(t, db, f.EmpresaID, gid, urlDepSet)
	if code != http.StatusOK || codigosDaVisao(v) != "D1,D2" {
		t.Fatalf("A e B: %d %s", code, codigosDaVisao(v))
	}
	if v.SemDepartamento != nil {
		t.Errorf("Sem departamento apareceu para o Gerente: %+v", v.SemDepartamento)
	}
	// Só A: B some.
	g1 := gestorCom(t, db, f, "D1")
	_, v1, _ := visaoDoGestor(t, db, f.EmpresaID, g1, urlDepSet)
	if codigosDaVisao(v1) != "D1" {
		t.Errorf("só D1: %s", codigosDaVisao(v1))
	}
	// Sem a base do grupo: nem faturamento líquido, nem percentuais.
	var m map[string]any
	json.Unmarshal([]byte(corpo), &m)
	if m["pct_status"] != "restrito" || m["faturamento_liquido"].(float64) != 0 || m["faturamento_liquido_linha"] != "" {
		t.Errorf("base do grupo vazou: %v %v %v", m["pct_status"], m["faturamento_liquido"], m["faturamento_liquido_linha"])
	}
	for _, g := range v.Grupos {
		for _, d := range g.Departamentos {
			if d.Pct != nil {
				t.Errorf("percentual de %s: %v", d.Codigo, *d.Pct)
			}
			for _, c := range d.Contas {
				if c.Pct != nil {
					t.Errorf("percentual da conta %s", c.Conta)
				}
			}
		}
	}
	// Totais são os do cenário: D1 = 1300, D2 = -30.
	if tot, _, ok := achar(v, "Comercial", "D1"); !ok || tot != 1300 {
		t.Errorf("D1 = %v", tot)
	}
}

func TestEscopo_GestorSemDepartamentoRecebeVazio(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	gid := gestorCom(t, db, f)
	code, v, _ := visaoDoGestor(t, db, f.EmpresaID, gid, urlDepSet)
	if code != http.StatusOK || codigosDaVisao(v) != "" || v.SemDepartamento != nil {
		t.Fatalf("vazio esperado: %d %+v", code, v)
	}
	// Contas de uma linha: vazio.
	w := chamar(DREContasHandler(db), reqDe(http.MethodGet, "/x?ano=2026&mes=9&linha=LB", f.EmpresaID, gid, "gestor"))
	var o struct{ Contas []any }
	json.Unmarshal(w.Body.Bytes(), &o)
	if w.Code != http.StatusOK || len(o.Contas) != 0 {
		t.Errorf("contas: %d %s", w.Code, w.Body.String())
	}
	// Lançamentos de uma conta: 403.
	w = chamar(DREContaLancamentosHandler(db), reqDe(http.MethodGet, urlDet, f.EmpresaID, gid, "gestor"))
	if w.Code != http.StatusForbidden {
		t.Errorf("lançamentos sem departamento: %d", w.Code)
	}
}

func TestEscopo_DepartamentoAlheioDireto403(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	g1 := gestorCom(t, db, f, "D1")
	if code, v, _ := visaoDoGestor(t, db, f.EmpresaID, g1, urlDepSet+"&departamento=D1"); code != 200 || codigosDaVisao(v) != "D1" {
		t.Errorf("departamento próprio: %d %s", code, codigosDaVisao(v))
	}
	for _, d := range []string{"D2", "D%25", "D", "", "XX"} {
		if d == "" {
			continue
		}
		if code, _, _ := visaoDoGestor(t, db, f.EmpresaID, g1, urlDepSet+"&departamento="+d); code != http.StatusForbidden {
			t.Errorf("departamento %q: %d", d, code)
		}
	}
	// Gerente sem departamento pedindo um: 403 também.
	g0 := gestorCom(t, db, f)
	if code, _, _ := visaoDoGestor(t, db, f.EmpresaID, g0, urlDepSet+"&departamento=D1"); code != http.StatusForbidden {
		t.Errorf("sem departamento: %d", code)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dre_departamentos' AND acao='negado'`, f.EmpresaID); n != 5 {
		t.Errorf("negativas registradas = %d", n)
	}
}

func TestEscopo_ContasELancamentosDaConta(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	exec(t, `INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'4.15','CMV',1)`, f.EmpresaID)
	lancPct(t, f, "2026-09-01", "01", "4.15", 7) // 4.15 casa com o prefixo 4.% (D2), NÃO com o 4.1 exato (D1)
	g1 := gestorCom(t, db, f, "D1")
	g2 := gestorCom(t, db, f, "D2")

	contasDa := func(gid, linha string) map[string]float64 {
		w := chamar(DREContasHandler(db), reqDe(http.MethodGet, "/x?ano=2026&mes=9&linha="+linha, f.EmpresaID, gid, "gestor"))
		if w.Code != http.StatusOK {
			t.Fatalf("contas %s: %d", linha, w.Code)
		}
		var o struct {
			Contas []struct {
				Conta string
				Valor float64
			}
		}
		json.Unmarshal(w.Body.Bytes(), &o)
		m := map[string]float64{}
		for _, c := range o.Contas {
			m[c.Conta] = c.Valor
		}
		return m
	}
	// Subtotal LB alcança todas as contas das linhas; o Gerente só recebe as do escopo.
	if m := contasDa(g1, "LB"); len(m) != 2 || m["3.1"] != 1200 || m["4.1"] != 100 {
		t.Errorf("D1: %v", m)
	}
	if m := contasDa(g2, "LB"); len(m) != 2 || m["4.2"] != -30 || m["4.15"] != 7 {
		t.Errorf("D2: %v", m)
	}
	// DED (3.2) e CMV de 5.1 não têm departamento: nenhum Gerente as vê.
	if m := contasDa(g1, "DED"); len(m) != 0 {
		t.Errorf("DED vazou: %v", m)
	}
	// Controladoria vê todas (inclusive as sem departamento).
	wc := chamar(DREContasHandler(db), reqDe(http.MethodGet, "/x?ano=2026&mes=9&linha=LB", f.EmpresaID, f.UserID, "controladoria"))
	var oc struct{ Contas []struct{ Conta string } }
	json.Unmarshal(wc.Body.Bytes(), &oc)
	if len(oc.Contas) != 6 {
		t.Errorf("controladoria: %d contas", len(oc.Contas))
	}

	lanc := func(gid, conta string) int {
		return chamar(DREContaLancamentosHandler(db), reqDe(http.MethodGet, "/x?ano=2026&mes=9&visao=mensal&conta="+conta, f.EmpresaID, gid, "gestor")).Code
	}
	casos := []struct {
		gid, conta string
		want       int
	}{
		{g1, "3.1", 200}, {g1, "4.1", 200}, {g1, "4.2", 403}, {g1, "3.2", 403}, {g1, "5.1", 403},
		{g1, "4.15", 403}, // o prefixo 4.% é de D2: o 4.1 exato de D1 não vaza o 4.15
		{g2, "4.2", 200}, {g2, "4.15", 200}, {g2, "4.1", 403}, {g2, "3.1", 403},
		{g1, "4.%25", 403}, {g1, "%25", 403}, {g1, "3.%25", 403}, {g1, "inexistente", 403},
	}
	for _, c := range casos {
		if got := lanc(c.gid, c.conta); got != c.want {
			t.Errorf("conta %s: %d, esperado %d", c.conta, got, c.want)
		}
	}
	// Controladoria abre a conta sem departamento.
	if w := chamar(DREContaLancamentosHandler(db), reqDe(http.MethodGet, "/x?ano=2026&mes=9&visao=mensal&conta=3.2", f.EmpresaID, f.UserID, "controladoria")); w.Code != 200 {
		t.Errorf("controladoria 3.2: %d", w.Code)
	}
}

// Departamento cujo código parece padrão não casa por prefixo nem curinga.
func TestEscopo_CodigoComCuringaNaoVaza(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	exec(t, `INSERT INTO auditoria.departamentos (empresa_id, codigo, nome, grupo) VALUES ($1::uuid,'D%','Depto curinga','Comercial')`, f.EmpresaID)
	g := gestorCom(t, db, f, "D%")
	code, v, _ := visaoDoGestor(t, db, f.EmpresaID, g, urlDepSet)
	if code != 200 || codigosDaVisao(v) != "" {
		t.Errorf("D%% vazou: %d %s", code, codigosDaVisao(v))
	}
	w := chamar(DREContaLancamentosHandler(db), reqDe(http.MethodGet, urlDet, f.EmpresaID, g, "gestor"))
	if w.Code != http.StatusForbidden {
		t.Errorf("conta de D1 por D%%: %d", w.Code)
	}
}

func TestEscopo_MesFechadoUsaOSnapshot(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d %s", w.Code, w.Body.String())
	}
	// Depois do fechamento o dicionário muda: 4.1 passa a ser de D2. O mês fechado não.
	exec(t, `UPDATE auditoria.conta_departamento SET departamento_id=(SELECT id FROM auditoria.departamentos WHERE empresa_id=$1::uuid AND codigo='D2')
		WHERE empresa_id=$1::uuid AND conta_padrao='4.1'`, f.EmpresaID)
	g1 := gestorCom(t, db, f, "D1")
	g2 := gestorCom(t, db, f, "D2")
	_, v1, _ := visaoDoGestor(t, db, f.EmpresaID, g1, urlDepSet)
	if tot, n, ok := achar(v1, "Comercial", "D1"); !ok || tot != 1300 || n != 2 || codigosDaVisao(v1) != "D1" {
		t.Errorf("D1 fechado: %v %v %v %s", tot, n, ok, codigosDaVisao(v1))
	}
	_, v2, _ := visaoDoGestor(t, db, f.EmpresaID, g2, urlDepSet)
	if tot, n, ok := achar(v2, "Logística", "D2"); !ok || tot != -30 || n != 1 || codigosDaVisao(v2) != "D2" {
		t.Errorf("D2 fechado: %v %v %v %s", tot, n, ok, codigosDaVisao(v2))
	}
	if v1.SemDepartamento != nil || v2.SemDepartamento != nil {
		t.Error("Sem departamento no mês fechado")
	}
	// Contas e lançamentos também seguem o snapshot.
	w := chamar(DREContaLancamentosHandler(db), reqDe(http.MethodGet, "/x?ano=2026&mes=9&visao=mensal&conta=4.1", f.EmpresaID, g1, "gestor"))
	if w.Code != 200 {
		t.Errorf("4.1 por D1 (snapshot): %d", w.Code)
	}
	w = chamar(DREContaLancamentosHandler(db), reqDe(http.MethodGet, "/x?ano=2026&mes=9&visao=mensal&conta=4.1", f.EmpresaID, g2, "gestor"))
	if w.Code != http.StatusForbidden {
		t.Errorf("4.1 por D2 (snapshot): %d", w.Code)
	}
	// Mês aberto (outubro) já usa o dicionário novo: 4.1 é de D2.
	lancPct(t, f, "2026-10-01", "01", "4.1", 5)
	_, v3, _ := visaoDoGestor(t, db, f.EmpresaID, g1, "/x?ano=2026&mes=10&visao=mensal")
	if codigosDaVisao(v3) != "" {
		t.Errorf("D1 em outubro aberto: %s", codigosDaVisao(v3))
	}
}

func TestEscopo_DREDoGrupoNegaAoGestor(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	g := gestorCom(t, db, f, "D1", "D2")
	rotas := map[string]http.HandlerFunc{
		"/x?ano=2026&mes=9&visao=mensal":          DREHandler(db),
		"/x?ano=2026":                             DRETrimestresHandler(db),
		"/x?ano=2026&mes=9&visao=mensal&linha=RB": DRELancamentosHandler(db),
	}
	for url, h := range rotas {
		w := chamar(h, reqDe(http.MethodGet, url, f.EmpresaID, g, "gestor"))
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: %d", url, w.Code)
		}
		if strings.Contains(w.Body.String(), "linhas") || strings.Contains(w.Body.String(), "lancamentos") {
			t.Errorf("%s: corpo com dado: %s", url, w.Body.String())
		}
		// os perfis sem restrição continuam vendo
		for _, p := range []string{"controladoria", "analista", "diretoria", "auditoria"} {
			if w := chamar(h, reqDe(http.MethodGet, url, f.EmpresaID, f.UserID, p)); w.Code != http.StatusOK {
				t.Errorf("%s por %s: %d", url, p, w.Code)
			}
		}
	}
}

func TestEscopo_PerfisSemRestricaoVemTudo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	for _, p := range []string{"controladoria", "analista", "diretoria", "auditoria"} {
		w := chamar(DREDepartamentosHandler(db), reqDe(http.MethodGet, urlDepSet, f.EmpresaID, f.UserID, p))
		var v visaoDep
		json.Unmarshal(w.Body.Bytes(), &v)
		if w.Code != 200 || codigosDaVisao(v) != "D1,D2" || v.SemDepartamento == nil || v.PctStatus != "ok" {
			t.Errorf("%s: %d %s sem=%v pct=%s", p, w.Code, codigosDaVisao(v), v.SemDepartamento, v.PctStatus)
		}
	}
	// admin de plataforma, mesmo sem perfil
	r := reqDe(http.MethodGet, urlDepSet, f.EmpresaID, f.UserID, "")
	r.Context().Value(AudContextKey).(*AudContext).Admin = true
	if w := chamar(DREDepartamentosHandler(db), r); w.Code != 200 {
		t.Errorf("admin: %d", w.Code)
	}
}

func TestEscopo_OutraEmpresaNaoAmplia(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	cenarioDep(t, outra) // mesmos códigos D1/D2 na outra empresa
	g := gestorCom(t, db, f, "D1", "D2")
	// O vínculo é da empresa f: na empresa ativa "outra" o Gerente não tem departamento.
	code, v, _ := visaoDoGestor(t, db, outra.EmpresaID, g, urlDepSet)
	if code != 200 || codigosDaVisao(v) != "" {
		t.Errorf("outra empresa: %d %s", code, codigosDaVisao(v))
	}
	if w := chamar(DREContaLancamentosHandler(db), reqDe(http.MethodGet, urlDet, outra.EmpresaID, g, "gestor")); w.Code != http.StatusForbidden {
		t.Errorf("conta da outra empresa: %d", w.Code)
	}
	// E na empresa certa vê só a dela (dados da outra não entram nos totais).
	_, v1, _ := visaoDoGestor(t, db, f.EmpresaID, g, urlDepSet)
	if tot, _, _ := achar(v1, "Comercial", "D1"); tot != 1300 {
		t.Errorf("D1 = %v", tot)
	}
}

func TestEscopo_PerfilSemLeituraNaoTemEscopo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	esc, err := escopoDe(context.Background(), db, &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: "inexistente"})
	if err != nil || !esc.Vazio() {
		t.Errorf("perfil desconhecido: %+v %v", esc, err)
	}
	esc, _ = escopoDe(context.Background(), db, nil)
	if !esc.Vazio() {
		t.Error("contexto nulo deveria negar")
	}
	esc, _ = escopoDe(context.Background(), db, &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: "controladoria"})
	if !esc.Irrestrito {
		t.Error("controladoria deveria ser irrestrita")
	}
}
