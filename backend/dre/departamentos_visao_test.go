package dre_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

type contador struct {
	db       *sql.DB
	vinculos int
}

func (c *contador) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	if strings.Contains(q, "auditoria.conta_departamento") {
		c.vinculos++
	}
	return c.db.QueryContext(ctx, q, args...)
}

// Resolver o departamento de muitas contas lê os vínculos uma única vez.
func TestContasPorDepartamento_VinculosLidosUmaVez(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	exec := func(q string, a ...any) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO auditoria.departamentos (empresa_id, codigo, nome, grupo) VALUES ($1::uuid,'D1','Depto 1','Comercial')`, f.EmpresaID)
	exec(`INSERT INTO auditoria.conta_departamento (empresa_id, conta_padrao, departamento_id, origem)
		SELECT $1::uuid, '5.%', id, 'grupo' FROM auditoria.departamentos WHERE empresa_id=$1::uuid`, f.EmpresaID)
	mapa := []dre.MapaItem{{ContaPadrao: "5.%", LinhaCodigo: "DA", Sinal: 1}}
	for _, m := range []string{"2026-07-01", "2026-08-01", "2026-09-01"} {
		for i := 0; i < 50; i++ {
			exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,$2,'01',$3,10)`,
				f.EmpresaID, m, "5."+string(rune('a'+i%26))+string(rune('a'+i/26)))
		}
	}
	c := &contador{db: db}
	p := dre.Periodo{Ini: mustData("2026-07-01"), Fim: mustData("2026-09-01")}
	contas, _, err := (dre.Leitor{Q: c}).ContasPorDepartamento(context.Background(), f.EmpresaID, "", p, mapa, map[string]dre.Ativo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(contas) != 50 || contas[0].Departamento != "D1" || contas[0].Valor != 30 {
		t.Errorf("contas: %d %+v", len(contas), contas[0])
	}
	if c.vinculos != 1 {
		t.Errorf("vínculos consultados %d vezes, esperado 1", c.vinculos)
	}
}
