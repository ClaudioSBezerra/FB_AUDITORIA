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

func limpar(t *testing.T, db *sql.DB, f testdb.Fixture, admin bool, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/aud/ambiente/limpar", strings.NewReader(corpo))
	c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: "controladoria", Admin: admin}
	r = r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
	return chamar(AmbienteLimparHandler(db), r)
}

func semearDominio(t *testing.T, db *sql.DB, f testdb.Fixture) {
	t.Helper()
	stmts := []string{
		`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, ordem) VALUES ($1::uuid,'RB','Receita',1)`,
		`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo) VALUES ($1::uuid,'3.1','RB')`,
		`INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, valor) VALUES ($1::uuid,'2026-09-01','3.1',10)`,
		`INSERT INTO auditoria.dre_orcado (empresa_id, competencia, linha_codigo, valor) VALUES ($1::uuid,'2026-09-01','RB',10)`,
		`INSERT INTO auditoria.cargas (empresa_id, arquivo, linhas) VALUES ($1::uuid,'x.csv',1)`,
		`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao) VALUES ($1::uuid,'2026-09-01',1)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s, f.EmpresaID); err != nil {
			t.Fatal(err)
		}
	}
	writeAuditLog(db, f.EmpresaID, f.UserID, "teste", "1", "semente", nil)
}

func dominio(t *testing.T, db *sql.DB, f testdb.Fixture) (lanc, orc, fech, cargas, estr, mapa, audit int) {
	c := func(tab string) int {
		return contar(t, db, `SELECT count(*) FROM auditoria.`+tab+` WHERE empresa_id=$1::uuid`, f.EmpresaID)
	}
	return c("lancamentos"), c("dre_orcado"), c("dre_fechamentos"), c("cargas"), c("dre_linhas"), c("dre_mapa"), c("audit_log")
}

func setAdmin(t *testing.T, db *sql.DB, f testdb.Fixture) {
	t.Helper()
	if _, err := db.Exec(`UPDATE users SET role='admin' WHERE id=$1::uuid`, f.UserID); err != nil {
		t.Fatal(err)
	}
}

func TestAmbiente_PadraoTeste(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	var amb string
	if err := db.QueryRow(`SELECT ambiente FROM companies WHERE id=$1::uuid`, f.EmpresaID).Scan(&amb); err != nil || amb != "teste" {
		t.Fatalf("ambiente=%q err=%v", amb, err)
	}
}

func TestAmbiente_ApagaEmTeste(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	setAdmin(t, db, f)
	semearDominio(t, db, f)
	w := limpar(t, db, f, true, `{"senha":"senha-de-teste"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	lanc, orc, fech, cargas, estr, mapa, audit := dominio(t, db, f)
	if lanc+orc+fech+cargas != 0 {
		t.Errorf("dominio nao apagado: %d %d %d %d", lanc, orc, fech, cargas)
	}
	if estr != 1 || mapa != 1 {
		t.Errorf("estrutura/mapa deveriam ficar: %d %d", estr, mapa)
	}
	if audit != 2 { // semente + limpeza
		t.Errorf("audit_log esperado 2, veio %d", audit)
	}
	if contar(t, db, `SELECT count(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND acao='limpar_dados_teste'`, f.EmpresaID) != 1 {
		t.Error("trilha da limpeza ausente")
	}
	if contar(t, db, `SELECT count(*) FROM users WHERE id=$1::uuid`, f.UserID) != 1 {
		t.Error("usuario deveria ficar")
	}
}

func contagemTabelas(t *testing.T, db *sql.DB, f testdb.Fixture) [4]int {
	t.Helper()
	l, o, fe, c, _, _, _ := dominio(t, db, f)
	return [4]int{l, o, fe, c}
}

func TestAmbiente_IsolamentoEntreEmpresas(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	g := testdb.NovaEmpresa(t, db)
	setAdmin(t, db, f)
	semearDominio(t, db, f)
	semearDominio(t, db, g)
	antes := contagemTabelas(t, db, g)
	if w := limpar(t, db, f, true, `{"senha":"senha-de-teste"}`); w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if depois := contagemTabelas(t, db, g); depois != antes {
		t.Errorf("segunda empresa mudou: antes %v depois %v", antes, depois)
	}
	if antes == [4]int{} {
		t.Error("semente da segunda empresa vazia")
	}
}

func TestAmbiente_CascataLinhasFechamento(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	setAdmin(t, db, f)
	semearDominio(t, db, f)
	if _, err := db.Exec(`INSERT INTO auditoria.dre_fechamento_linhas (fechamento_id, linha_codigo, descricao, nivel, ordem, valor)
		SELECT id,'RB','Receita',1,1,10 FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	var fid int64
	if err := db.QueryRow(`SELECT id FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid`, f.EmpresaID).Scan(&fid); err != nil {
		t.Fatal(err)
	}
	q := `SELECT count(*) FROM auditoria.dre_fechamento_linhas WHERE fechamento_id=$1`
	if contar(t, db, q, fid) != 1 {
		t.Fatal("semente das linhas do fechamento ausente")
	}
	if w := limpar(t, db, f, true, `{"senha":"senha-de-teste"}`); w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, q, fid); n != 0 {
		t.Errorf("linhas do fechamento restantes: %d", n)
	}
}

// Toda tabela de domínio (com empresa_id) deve ser apagada por limpar_dados_teste,
// salvo as que se mantêm de propósito.
func TestAmbiente_LimpezaCobreTodaTabelaDeDominio(t *testing.T) {
	db := testdb.Open(t)
	mantidas := map[string]bool{"dre_linhas": true, "dre_mapa": true, "audit_log": true}
	cascata := map[string]bool{"dre_fechamento_linhas": true}
	rows, err := db.Query(`SELECT DISTINCT table_name FROM information_schema.columns
		WHERE table_schema='auditoria' AND column_name='empresa_id' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var def string
	if err := db.QueryRow(`SELECT pg_get_functiondef('auditoria.limpar_dados_teste(uuid)'::regprocedure)`).Scan(&def); err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var tab string
		if err := rows.Scan(&tab); err != nil {
			t.Fatal(err)
		}
		if mantidas[tab] || cascata[tab] {
			continue
		}
		if !strings.Contains(def, "DELETE FROM auditoria."+tab+" ") {
			t.Errorf("tabela %s tem empresa_id e nao e apagada por limpar_dados_teste", tab)
		}
	}
}

func TestAmbiente_SenhaErradaOuAusente(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	setAdmin(t, db, f)
	semearDominio(t, db, f)
	for _, corpo := range []string{`{"senha":"errada"}`, `{}`, ``} {
		if w := limpar(t, db, f, true, corpo); w.Code != http.StatusForbidden {
			t.Errorf("corpo %q: status %d", corpo, w.Code)
		}
	}
	if lanc, _, _, _, _, _, _ := dominio(t, db, f); lanc != 1 {
		t.Error("nada deveria ser apagado")
	}
}

func TestAmbiente_NaoAdmin403(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearDominio(t, db, f)
	if w := limpar(t, db, f, false, `{"senha":"senha-de-teste"}`); w.Code != http.StatusForbidden {
		t.Fatalf("status %d", w.Code)
	}
	if lanc, _, _, _, _, _, _ := dominio(t, db, f); lanc != 1 {
		t.Error("nada deveria ser apagado")
	}
}

func TestAmbiente_Producao409EFuncaoRecusa(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	setAdmin(t, db, f)
	semearDominio(t, db, f)
	if _, err := db.Exec(`UPDATE companies SET ambiente='producao' WHERE id=$1::uuid`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if w := limpar(t, db, f, true, `{"senha":"senha-de-teste"}`); w.Code != http.StatusConflict {
		t.Fatalf("status %d", w.Code)
	}
	if _, err := db.Exec(`SELECT auditoria.limpar_dados_teste($1::uuid)`, f.EmpresaID); err == nil {
		t.Error("funcao deveria recusar em producao")
	}
	if lanc, _, _, _, _, _, _ := dominio(t, db, f); lanc != 1 {
		t.Error("nada deveria ser apagado")
	}
	if _, err := db.Exec(`DELETE FROM companies WHERE id=$1::uuid`, f.EmpresaID); err == nil {
		t.Error("delete de empresa em producao deveria ser recusado")
	}
}

func TestAmbiente_VoltarATesteRecusado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := db.Exec(`UPDATE companies SET ambiente='producao' WHERE id=$1::uuid`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE companies SET ambiente='teste' WHERE id=$1::uuid`, f.EmpresaID); err == nil {
		t.Error("voltar a teste deveria ser recusado")
	}
}

func TestAmbiente_ImutavelSalvoEmTeste(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	q := `SELECT auditoria.imutavel_salvo_em_teste($1::uuid)`
	var ok bool
	if err := db.QueryRow(q, f.EmpresaID).Scan(&ok); err != nil || ok {
		t.Fatalf("sem sessao marcada deveria ser falso: %v %v", ok, err)
	}
	tx, _ := db.Begin()
	defer tx.Rollback()
	if _, err := tx.Exec(`SELECT set_config('auditoria.limpando_teste', $1, true)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(q, f.EmpresaID).Scan(&ok); err != nil || !ok {
		t.Fatalf("em teste com sessao marcada deveria ser verdadeiro: %v %v", ok, err)
	}
	if _, err := tx.Exec(`UPDATE companies SET ambiente='producao' WHERE id=$1::uuid`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(q, f.EmpresaID).Scan(&ok); err != nil || ok {
		t.Fatalf("em producao deveria ser falso: %v %v", ok, err)
	}
}

func virar(t *testing.T, db *sql.DB, f testdb.Fixture, admin bool, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/aud/ambiente/virar", strings.NewReader(corpo))
	c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: "controladoria", Admin: admin}
	r = r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
	return chamar(AmbienteVirarHandler(db), r)
}

func ambienteDe(t *testing.T, db *sql.DB, f testdb.Fixture) string {
	t.Helper()
	var amb string
	if err := db.QueryRow(`SELECT ambiente FROM companies WHERE id=$1::uuid`, f.EmpresaID).Scan(&amb); err != nil {
		t.Fatal(err)
	}
	return amb
}

const corpoVirarOK = `{"senha":"senha-de-teste","confirmacao":"VIRAR PARA PRODUCAO"}`

func TestAmbienteVirar_VaiParaProducaoComAviso(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	setAdmin(t, db, f)
	semearDominio(t, db, f) // lancamento com origem 'csv', conta 3.1
	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, valor, origem) VALUES
		($1::uuid,'2026-09-01','3.2',1,'csv.v2'),($1::uuid,'2026-09-01','3.3',1,'winthor'),($1::uuid,'2026-09-01','3.1',1,'csv')`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	w := virar(t, db, f, true, corpoVirarOK)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if ambienteDe(t, db, f) != "producao" {
		t.Error("deveria estar em producao")
	}
	var resp struct {
		Ambiente string   `json:"ambiente"`
		Contas   []string `json:"contas_de_arquivo"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Ambiente != "producao" || len(resp.Contas) != 2 || resp.Contas[0] != "3.1" || resp.Contas[1] != "3.2" {
		t.Errorf("resposta inesperada: %+v", resp)
	}
	if contar(t, db, `SELECT count(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND acao='virar_producao' AND user_id=$2::uuid`, f.EmpresaID, f.UserID) != 1 {
		t.Error("trilha da virada ausente")
	}
}

func TestAmbienteVirar_SenhaOuFraseErrada403(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	setAdmin(t, db, f)
	for _, corpo := range []string{
		`{"senha":"errada","confirmacao":"VIRAR PARA PRODUCAO"}`,
		`{"confirmacao":"VIRAR PARA PRODUCAO"}`,
		``,
		`{"senha":"senha-de-teste","confirmacao":"virar para producao"}`,
		`{"senha":"senha-de-teste"}`,
	} {
		if w := virar(t, db, f, true, corpo); w.Code != http.StatusForbidden {
			t.Errorf("corpo %q: status %d", corpo, w.Code)
		}
	}
	if ambienteDe(t, db, f) != "teste" {
		t.Error("nada deveria mudar")
	}
}

func TestAmbienteVirar_NaoAdmin403(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if w := virar(t, db, f, false, corpoVirarOK); w.Code != http.StatusForbidden {
		t.Fatalf("status %d", w.Code)
	}
	if ambienteDe(t, db, f) != "teste" {
		t.Error("nada deveria mudar")
	}
}

func TestAmbienteVirar_JaEmProducao409SemNovoRegistro(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	setAdmin(t, db, f)
	if w := virar(t, db, f, true, corpoVirarOK); w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if w := virar(t, db, f, true, corpoVirarOK); w.Code != http.StatusConflict {
		t.Fatalf("status %d", w.Code)
	}
	if contar(t, db, `SELECT count(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND acao='virar_producao'`, f.EmpresaID) != 1 {
		t.Error("deveria haver um só registro")
	}
}

func TestAudMe_InformaAmbiente(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	me := func() string {
		r := httptest.NewRequest(http.MethodGet, "/api/aud/me", nil)
		c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: "controladoria"}
		r = r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
		w := chamar(AudMeHandler(db), r)
		if w.Code != http.StatusOK {
			t.Fatalf("status %d", w.Code)
		}
		var m map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		a, ok := m["ambiente"].(string)
		if !ok {
			t.Fatalf("campo ambiente ausente ou de tipo errado: %v", m["ambiente"])
		}
		return a
	}
	if a := me(); a != "teste" {
		t.Errorf("ambiente=%q", a)
	}
	if _, err := db.Exec(`UPDATE companies SET ambiente='producao' WHERE id=$1::uuid`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if a := me(); a != "producao" {
		t.Errorf("ambiente=%q", a)
	}
}

func TestAmbienteVirar_FalhaNaTrilhaDesfazTudo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	setAdmin(t, db, f)
	for _, q := range []string{
		`CREATE OR REPLACE FUNCTION auditoria.tg_teste_falha_virar() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
			IF NEW.acao = 'virar_producao' THEN RAISE EXCEPTION 'falha simulada'; END IF; RETURN NEW; END $$`,
		`CREATE TRIGGER tg_teste_falha_virar BEFORE INSERT ON auditoria.audit_log FOR EACH ROW EXECUTE FUNCTION auditoria.tg_teste_falha_virar()`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec(`DROP TRIGGER IF EXISTS tg_teste_falha_virar ON auditoria.audit_log`)
		db.Exec(`DROP FUNCTION IF EXISTS auditoria.tg_teste_falha_virar()`)
	})
	if w := virar(t, db, f, true, corpoVirarOK); w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", w.Code)
	}
	if ambienteDe(t, db, f) != "teste" {
		t.Error("ambiente deveria continuar teste")
	}
}
