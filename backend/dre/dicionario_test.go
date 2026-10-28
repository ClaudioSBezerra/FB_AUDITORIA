package dre_test

import (
	"context"
	"errors"
	"testing"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

func salvarD(t *testing.T, s *dre.Dicionario, f testdb.Fixture, id, cod, grupo string) (dre.Departamento, error) {
	t.Helper()
	return s.SalvarDepartamento(context.Background(), dre.SalvarDepartamentoIn{
		EmpresaID: f.EmpresaID, UserID: f.UserID, ID: id, Codigo: cod, Nome: "Depto " + cod, Grupo: grupo})
}

func TestDepartamentos_CriarEditarExcluirTrilha(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	s := &dre.Dicionario{DB: db}

	d1, err := salvarD(t, s, f, "", "D1", "Logística")
	if err != nil {
		t.Fatal(err)
	}
	// Mesmo código em outra empresa não conflita.
	if _, err := salvarD(t, s, outra, "", "D1", "Comercial"); err != nil {
		t.Fatal(err)
	}
	// Código repetido na empresa.
	var ce *dre.ErrDepartamentoCodigoEmUso
	if _, err := salvarD(t, s, f, "", "D1", "Comercial"); !errors.As(err, &ce) {
		t.Errorf("esperava código em uso: %v", err)
	}
	// Editar.
	d1, err = salvarD(t, s, f, d1.ID, "D1", "Administrativo")
	if err != nil || d1.Grupo != "Administrativo" {
		t.Fatalf("editar: %v %+v", err, d1)
	}
	// Renomear para código existente.
	d2, _ := salvarD(t, s, f, "", "D2", "Comercial")
	if _, err := salvarD(t, s, f, d2.ID, "D1", "Comercial"); !errors.As(err, &ce) {
		t.Errorf("renomear para existente: %v", err)
	}
	// Entrada inválida.
	var ee *dre.ErroEntrada
	for _, c := range [][2]string{{"", "Comercial"}, {"D3", "Outro"}, {"D3", ""}} {
		if _, err := salvarD(t, s, f, "", c[0], c[1]); !errors.As(err, &ee) {
			t.Errorf("%v: esperava ErroEntrada: %v", c, err)
		}
	}
	// Id inexistente ou inválido.
	var nf *dre.ErrDepartamentoNaoEncontrado
	if _, err := salvarD(t, s, f, "00000000-0000-0000-0000-000000000000", "DX", "Comercial"); !errors.As(err, &nf) {
		t.Errorf("editar inexistente: %v", err)
	}
	if err := s.ExcluirDepartamento(ctx, f.EmpresaID, f.UserID, "x"); !errors.As(err, &nf) {
		t.Errorf("excluir id inválido: %v", err)
	}
	// Outra empresa não enxerga nem exclui.
	if err := s.ExcluirDepartamento(ctx, outra.EmpresaID, outra.UserID, d1.ID); !errors.As(err, &nf) {
		t.Errorf("excluir de outra empresa: %v", err)
	}
	l, _ := s.ListarDepartamentos(ctx, f.EmpresaID)
	if len(l) != 2 || l[0].Codigo != "D1" {
		t.Errorf("listar: %+v", l)
	}
	// Excluir.
	if err := s.ExcluirDepartamento(ctx, f.EmpresaID, f.UserID, d1.ID); err != nil {
		t.Fatal(err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='departamento'`, f.EmpresaID); n != 4 {
		t.Errorf("trilha: %d (criar D1, editar, criar D2, excluir)", n)
	}
}

func TestDepartamentos_LimpezaDeTesteApaga(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	s := &dre.Dicionario{DB: db}
	if _, err := salvarD(t, s, f, "", "D1", "Comercial"); err != nil {
		t.Fatal(err)
	}
	var res string
	if err := db.QueryRow(`SELECT auditoria.limpar_dados_teste($1::uuid)::text`, f.EmpresaID).Scan(&res); err != nil {
		t.Fatal(err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.departamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("departamentos restantes: %d (%s)", n, res)
	}
}
