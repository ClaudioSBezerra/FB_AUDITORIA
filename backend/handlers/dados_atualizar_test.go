package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

type fonteHandlerFake struct{ lancs []dre.Lancamento }

func (f *fonteHandlerFake) Nome() string { return "erp-fake" }
func (f *fonteHandlerFake) Ler(context.Context, dre.Escopo) ([]dre.Lancamento, error) {
	return f.lancs, nil
}

func chamarAtualizar(h http.Handler, f testdb.Fixture, perfil string, admin bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/aud/dados/atualizar", nil)
	c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: perfil, Areas: []string{AreaEmpresa}, Admin: admin}
	return chamar(h, r.WithContext(context.WithValue(r.Context(), AudContextKey, c)))
}

func TestDadosAtualizar_Handler(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	var corpo dre.Atualizacao

	// Sem fonte registrada: 200 com aviso, não sucesso.
	vazio := dadosAtualizar(db, &dre.Registro{})
	w := chamarAtualizar(vazio, f, "controladoria", false)
	json.Unmarshal(w.Body.Bytes(), &corpo)
	if w.Code != http.StatusOK || corpo.Status != dre.AtualizacaoSemFonte || corpo.Hora.IsZero() {
		t.Fatalf("sem fonte: %d %s", w.Code, w.Body.String())
	}

	// Perfis sem permissão: 403, nada rodou, tentativa na trilha.
	reg := &dre.Registro{}
	t1, _ := time.Parse("2006-01", "2026-09")
	reg.Registrar(dre.ViaIngestor(&dre.Ingestor{DB: db}, &fonteHandlerFake{lancs: []dre.Lancamento{{Linha: 1, Competencia: t1, Filial: "01", Conta: "3.1", Valor: 10}}}))
	h := dadosAtualizar(db, reg)
	for _, p := range []string{"auditoria", "diretoria", "gestor", ""} {
		if w = chamarAtualizar(h, f, p, false); w.Code != http.StatusForbidden {
			t.Errorf("perfil %q: %d", p, w.Code)
		}
	}
	if n := contar(t, db, total(), f.EmpresaID); n != 0 {
		t.Errorf("nada deveria ser gravado: %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='atualizacao_dados' AND acao='negado'`, f.EmpresaID); n != 4 {
		t.Errorf("negados na trilha: %d", n)
	}
	if w = chamar(h, httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(context.WithValue(context.Background(), AudContextKey, &AudContext{EmpresaID: f.EmpresaID, Perfil: "controladoria"}))); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}

	// Controladoria e admin de plataforma atualizam.
	for _, c := range []struct {
		perfil string
		admin  bool
	}{{"controladoria", false}, {"", true}} {
		w = chamarAtualizar(h, f, c.perfil, c.admin)
		corpo = dre.Atualizacao{}
		json.Unmarshal(w.Body.Bytes(), &corpo)
		if w.Code != http.StatusOK || corpo.Status != dre.AtualizacaoOK || len(corpo.Fontes) != 1 || corpo.Fontes[0].Linhas != 1 {
			t.Fatalf("%+v: %d %s", c, w.Code, w.Body.String())
		}
	}

	// Mês fechado: 409 com orientação.
	estruturaPct(t, f, false)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d %s", w.Code, w.Body.String())
	}
	w = chamarAtualizar(h, f, "controladoria", false)
	corpo = dre.Atualizacao{}
	json.Unmarshal(w.Body.Bytes(), &corpo)
	if w.Code != http.StatusConflict || corpo.Status != dre.AtualizacaoRecusada || len(corpo.Fontes) != 1 {
		t.Fatalf("fechado: %d %s", w.Code, w.Body.String())
	}
}

func total() string { return `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid` }
