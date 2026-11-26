package dre_test

import (
	"context"
	"testing"
	"time"

	"fb_auditoria/dre"
	"fb_auditoria/dre/fontes/oracle"
	"fb_auditoria/internal/testdb"
)

func TestAgendar_DesligadoNaoRodaLigadoRodaUmaVez(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	erp := &erpFalso{real: map[string][]oracle.LinhaRealizado{"2026-09": {linhaReal("11", "4010001", -10)}}}
	i := novaImportacao(t, f, erp)
	ctx := context.Background()
	salvo := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	depois := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) // dia 5, após 06:30 de São Paulo

	agendar := func(quando time.Time) *dre.Execucao {
		t.Helper()
		ex, err := i.Agendar(ctx, quando)
		if err != nil {
			t.Fatal(err)
		}
		return ex
	}
	if ex := agendar(depois); ex != nil {
		t.Fatalf("sem configuração não roda: %+v", ex)
	}
	a := dre.AgendamentoPadrao()
	a.Ativo = true
	if err := dre.SalvarAgendamento(ctx, db, f.EmpresaID, f.UserID, a, salvo); err != nil {
		t.Fatal(err)
	}
	// o horário de 05/09 já tinha passado quando foi ligado
	if ex := agendar(salvo.Add(time.Hour)); ex != nil {
		t.Fatalf("horário anterior à ativação disparou: %+v", ex)
	}
	ex := agendar(depois)
	if ex == nil {
		t.Fatal("deveria rodar")
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.execucoes WHERE empresa_id=$1::uuid
		AND gatilho='agendador' AND user_id IS NULL AND de='2026-09-01' AND ate='2026-09-01'`, f.EmpresaID); n != 1 {
		t.Errorf("execução registrada: %d", n)
	}
	if ex := agendar(depois.Add(time.Minute)); ex != nil {
		t.Fatalf("rodou duas vezes o mesmo horário: %+v", ex)
	}
	got, err := dre.LerAgendamento(ctx, db, f.EmpresaID, depois)
	if err != nil || got.Ultimo == nil || got.Proxima == nil {
		t.Fatalf("%+v %v", got, err)
	}
	a.Ativo = false
	if err := dre.SalvarAgendamento(ctx, db, f.EmpresaID, f.UserID, a, depois); err != nil {
		t.Fatal(err)
	}
	if ex := agendar(depois.AddDate(0, 2, 0)); ex != nil {
		t.Fatalf("desligado rodou: %+v", ex)
	}
}
