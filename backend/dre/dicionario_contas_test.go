package dre_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

func carga(f testdb.Fixture, linhas ...dre.ContaDic) dre.CargaDicionarioIn {
	for i := range linhas {
		linhas[i].Linha = i + 2
	}
	return dre.CargaDicionarioIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Arquivo: "d.csv", Linhas: linhas}
}

func origemDe(t *testing.T, s *dre.Dicionario, f testdb.Fixture, conta string) (string, string) {
	t.Helper()
	l, err := s.ListarContas(context.Background(), f.EmpresaID, "", 1, 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range l.Contas {
		if c.Conta == conta {
			return c.DepartamentoCodigo, c.Origem
		}
	}
	t.Fatalf("conta %s fora da lista", conta)
	return "", ""
}

func TestDicionario_CargaDerivaPorGrupoEReenvio(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	s := &dre.Dicionario{DB: db}
	dA, _ := salvarD(t, s, f, "", "G1", "Logística")
	dB, _ := salvarD(t, s, f, "", "D9", "Comercial")

	// Mapa existente: não pode ser tocado.
	if _, err := db.Exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'C1','L1',1)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}

	out, err := s.Carregar(ctx, carga(f,
		dre.ContaDic{Conta: "C1", Descricao: "um", Grupo: "G1"},
		dre.ContaDic{Conta: "C2", Descricao: "dois", Grupo: "G1", Departamento: "D9"},
		dre.ContaDic{Conta: "C3", Descricao: "tres", Grupo: "ZZ"},
	))
	if err != nil {
		t.Fatal(err)
	}
	if out.Novas != 3 || out.VinculosGrupo != 1 || out.VinculosInformado != 1 || out.SemDepartamento != 1 {
		t.Errorf("resultado: %+v", out)
	}
	if d, o := origemDe(t, s, f, "C1"); d != "G1" || o != "grupo" {
		t.Errorf("C1: %s %s", d, o)
	}
	if d, o := origemDe(t, s, f, "C2"); d != "D9" || o != "manual" {
		t.Errorf("C2: %s %s", d, o)
	}
	l, _ := s.ListarContas(ctx, f.EmpresaID, "", 1, 50)
	if l.PendentesTotal != 1 || l.Pendentes[0].Conta != "C3" {
		t.Errorf("pendentes: %+v", l.Pendentes)
	}

	// Correção manual de C1 para D9; reenvio derivando NÃO sobrescreve.
	if _, err := s.CorrigirDepartamento(ctx, f.EmpresaID, f.UserID, "C1", dB.ID); err != nil {
		t.Fatal(err)
	}
	out, err = s.Carregar(ctx, carga(f,
		dre.ContaDic{Conta: "C1", Descricao: "um novo", Grupo: "G1"},
		dre.ContaDic{Conta: "C3", Descricao: "tres", Grupo: "G1"}, // agora deriva
	))
	if err != nil {
		t.Fatal(err)
	}
	if out.Atualizadas != 2 || out.MantidosManual != 1 || out.VinculosGrupo != 1 {
		t.Errorf("reenvio: %+v", out)
	}
	if d, o := origemDe(t, s, f, "C1"); d != "D9" || o != "manual" {
		t.Errorf("manual sobrescrito: %s %s", d, o)
	}
	if d, o := origemDe(t, s, f, "C3"); d != "G1" || o != "grupo" {
		t.Errorf("C3: %s %s", d, o)
	}
	// Departamento informado no arquivo sobrescreve o manual.
	if _, err := s.Carregar(ctx, carga(f, dre.ContaDic{Conta: "C1", Grupo: "G1", Departamento: "G1"})); err != nil {
		t.Fatal(err)
	}
	if d, _ := origemDe(t, s, f, "C1"); d != "G1" {
		t.Errorf("informado no arquivo deveria valer: %s", d)
	}
	// Grupo some do departamento: vínculo derivado antigo é removido, conta fica pendente.
	if _, err := s.CorrigirDepartamento(ctx, f.EmpresaID, f.UserID, "C3", ""); err != nil { // volta a derivar (G1)
		t.Fatal(err)
	}
	out, err = s.Carregar(ctx, carga(f, dre.ContaDic{Conta: "C3", Grupo: "ZZ"}))
	if err != nil || out.VinculosRemovidos != 1 || out.SemDepartamento != 1 {
		t.Errorf("remoção: %v %+v", err, out)
	}

	// dre_mapa intacto.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_mapa WHERE empresa_id=$1::uuid AND conta_padrao='C1' AND linha_codigo='L1'`, f.EmpresaID); n != 1 {
		t.Errorf("dre_mapa alterado: %d", n)
	}
	_ = dA
	// Trilha: 4 cargas + 2 correções... conferir só que existe por carga e por correção.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dicionario' AND acao='carregar'`, f.EmpresaID); n != 4 {
		t.Errorf("trilha de carga: %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dicionario' AND acao='corrigir_departamento'`, f.EmpresaID); n != 2 {
		t.Errorf("trilha de correção: %d", n)
	}
}

func TestDicionario_ErrosNaoGravamNada(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	s := &dre.Dicionario{DB: db}
	salvarD(t, s, f, "", "G1", "Logística")

	in := carga(f,
		dre.ContaDic{Conta: "C1", Grupo: "G1"},
		dre.ContaDic{Conta: "C1", Grupo: "G1"},
		dre.ContaDic{Conta: "C2", Grupo: "G1", Departamento: "NAO"},
	)
	in.Problemas = []dre.Problema{{Linha: 1, Msg: "da leitura"}}
	_, err := s.Carregar(ctx, in)
	var el *dre.ErroLinhas
	if !errors.As(err, &el) || len(el.Problemas) != 3 {
		t.Fatalf("esperava 3 problemas: %v", err)
	}
	if el.Problemas[0].Linha != 1 || !strings.Contains(err.Error(), "repetida") || !strings.Contains(err.Error(), "inexistente") {
		t.Errorf("mensagem: %v", err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.conta_dicionario WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("gravou com erro: %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dicionario'`, f.EmpresaID); n != 0 {
		t.Errorf("trilha com erro: %d", n)
	}
	// Mais de 20 problemas: lista 20 e informa o total.
	var muitas []dre.ContaDic
	for i := 0; i < 25; i++ {
		muitas = append(muitas, dre.ContaDic{Conta: "X", Grupo: "G1"})
	}
	_, err = s.Carregar(ctx, carga(f, muitas...))
	if !errors.As(err, &el) || len(el.Problemas) != 24 || !strings.Contains(err.Error(), "24 linha(s)") || !strings.Contains(err.Error(), "e mais 4") {
		t.Errorf("limite de 20: %v", err)
	}
	// Arquivo sem contas.
	var ee *dre.ErroEntrada
	if _, err := s.Carregar(ctx, carga(f)); !errors.As(err, &ee) {
		t.Errorf("sem contas: %v", err)
	}
}

func TestDicionario_DepartamentoDePrefixoEExclusao409(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	s := &dre.Dicionario{DB: db}
	dA, _ := salvarD(t, s, f, "", "A", "Comercial")
	dB, _ := salvarD(t, s, f, "", "B", "Logística")
	if _, err := s.Carregar(ctx, carga(f,
		dre.ContaDic{Conta: "4%", Grupo: "A"},
		dre.ContaDic{Conta: "402%", Grupo: "B"},
		dre.ContaDic{Conta: "402.001", Grupo: "A"},
		dre.ContaDic{Conta: "9", Grupo: "NADA"},
	)); err != nil {
		t.Fatal(err)
	}
	casos := map[string]string{"401.5": "A", "402.9": "B", "402.001": "A", "4": "A", "9": "", "5": ""}
	for conta, esp := range casos {
		d, err := dre.DepartamentoDe(ctx, db, f.EmpresaID, conta)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if d != nil {
			got = d.Codigo
		}
		if got != esp {
			t.Errorf("DepartamentoDe(%s) = %q, esperado %q", conta, got, esp)
		}
	}
	// Mesma resolução do Mapa.
	mapa := []dre.MapaItem{{ContaPadrao: "4%"}, {ContaPadrao: "402%"}, {ContaPadrao: "402.001"}}
	if m, ok := dre.ResolverConta(mapa, "402.9"); !ok || m.ContaPadrao != "402%" {
		t.Errorf("ResolverConta: %+v", m)
	}
	// Outra empresa não vê os vínculos.
	if d, _ := dre.DepartamentoDe(ctx, db, outra.EmpresaID, "401.5"); d != nil {
		t.Errorf("vazou entre empresas: %+v", d)
	}
	l, _ := s.ListarContas(ctx, f.EmpresaID, "402", 1, 50)
	if l.Total != 2 {
		t.Errorf("busca: %d", l.Total)
	}

	// Exclusão: 409 com conta ligada; livre depois de desligar.
	var eu *dre.ErrDepartamentoEmUso
	if err := s.ExcluirDepartamento(ctx, f.EmpresaID, f.UserID, dA.ID); !errors.As(err, &eu) || eu.Contas != 2 {
		t.Errorf("esperava 409 com 2 contas: %v", err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.departamentos WHERE empresa_id=$1::uuid AND id=$2::uuid`, f.EmpresaID, dA.ID); n != 1 {
		t.Error("departamento em uso foi excluído")
	}
	if _, err := db.Exec(`DELETE FROM auditoria.conta_departamento WHERE empresa_id=$1::uuid AND departamento_id=$2::uuid`, f.EmpresaID, dB.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ExcluirDepartamento(ctx, f.EmpresaID, f.UserID, dB.ID); err != nil {
		t.Errorf("excluir livre: %v", err)
	}
}

func TestDicionario_CorrigirErrosELimpeza(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	s := &dre.Dicionario{DB: db}
	dA, _ := salvarD(t, s, f, "", "A", "Comercial")
	dO, _ := salvarD(t, s, outra, "", "A", "Comercial")
	if _, err := s.Carregar(ctx, carga(f, dre.ContaDic{Conta: "C1", Grupo: "NADA"})); err != nil {
		t.Fatal(err)
	}
	var cn *dre.ErrContaNaoEncontrada
	if _, err := s.CorrigirDepartamento(ctx, f.EmpresaID, f.UserID, "XX", dA.ID); !errors.As(err, &cn) {
		t.Errorf("conta inexistente: %v", err)
	}
	var dn *dre.ErrDepartamentoNaoEncontrado
	if _, err := s.CorrigirDepartamento(ctx, f.EmpresaID, f.UserID, "C1", dO.ID); !errors.As(err, &dn) {
		t.Errorf("departamento de outra empresa: %v", err)
	}
	if _, err := s.CorrigirDepartamento(ctx, f.EmpresaID, f.UserID, "C1", "x"); !errors.As(err, &dn) {
		t.Errorf("id inválido: %v", err)
	}
	c, err := s.CorrigirDepartamento(ctx, f.EmpresaID, f.UserID, "C1", dA.ID)
	if err != nil || c.DepartamentoCodigo != "A" || c.Origem != "manual" {
		t.Fatalf("corrigir: %v %+v", err, c)
	}
	// Sem grupo derivável, "" remove o vínculo.
	c, err = s.CorrigirDepartamento(ctx, f.EmpresaID, f.UserID, "C1", "")
	if err != nil || c.DepartamentoCodigo != "" {
		t.Errorf("desfazer: %v %+v", err, c)
	}
	// Limpeza de teste apaga as tabelas novas.
	if _, err := s.CorrigirDepartamento(ctx, f.EmpresaID, f.UserID, "C1", dA.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT auditoria.limpar_dados_teste($1::uuid)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	for _, tab := range []string{"conta_dicionario", "conta_departamento", "departamentos"} {
		if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.`+tab+` WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
			t.Errorf("%s: %d linhas após a limpeza", tab, n)
		}
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.departamentos WHERE empresa_id=$1::uuid`, outra.EmpresaID); n != 1 {
		t.Errorf("limpeza afetou outra empresa: %d", n)
	}
}
