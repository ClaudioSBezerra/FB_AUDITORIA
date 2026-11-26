package oracle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fb_auditoria/dre"
)

type falso struct{ cons []LinhaConsolidado }

func (f falso) Consolidado(context.Context, time.Time) ([]LinhaConsolidado, error) {
	return f.cons, nil
}
func (f falso) Realizado(context.Context, time.Time) ([]LinhaRealizado, error) { return nil, nil }

func TestParseMesAno(t *testing.T) {
	if m, err := ParseMesAno("05/2026"); err != nil || m.Format("2006-01") != "2026-05" {
		t.Fatal(m, err)
	}
	for _, s := range []string{"5/2026", "2026-05", "13/2026", ""} {
		if _, err := ParseMesAno(s); err == nil {
			t.Errorf("%q deveria ser recusado", s)
		}
	}
}

func TestConsolidado_ViraContasERejeitaMesInvalido(t *testing.T) {
	contas, err := CarregarContas("")
	if err != nil {
		t.Fatal(err)
	}
	comp := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	todas := map[string]float64{}
	for i, c := range ColunasConsolidado {
		todas[c] = float64(i + 1)
	}
	l, err := NovaConsolidado(falso{[]LinhaConsolidado{{UF: "XX", Filial: "01", MesAno: "05/2026", Valores: todas}}}, comp, contas).
		Ler(context.Background(), dre.Escopo{})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, x := range l {
		got = append(got, x.Conta)
	}
	if strings.Join(got, ",") != "1,2,2,ST,DESC CUSTO" || l[0].Filial != "1" || l[0].UF != "XX" || l[0].Valor != 1 {
		t.Fatalf("%v %+v", got, l[0])
	}
	_, err = NovaConsolidado(falso{[]LinhaConsolidado{{Filial: "1", MesAno: "2026-05", Valores: todas}}}, comp, contas).
		Ler(context.Background(), dre.Escopo{})
	var el *dre.ErroLinhas
	if !errors.As(err, &el) {
		t.Fatalf("%v", err)
	}
}

func TestCarregarContas_ArquivoInvalido(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(p, []byte(`{"colunas":{"VLFATBRUTO":{"conta":"1"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CarregarContas(p); err == nil {
		t.Error("faltando colunas deveria falhar")
	}
}

func TestConfigESQL(t *testing.T) {
	env := map[string]string{"JC_ORACLE_HOST": "h", "JC_ORACLE_SERVICE": "s", "JC_ORACLE_USER": "u", "JC_ORACLE_PASS": "p"}
	get := func(k string) string { return env[k] }
	c, ok, err := ConfigDoAmbiente(get)
	if !ok || err != nil || c.Porta != 1521 || c.ObjetoRealizado != "CONTR_DRE_REALIZ" || c.ColunaData != "DTLANC" {
		t.Fatalf("%+v %v %v", c, ok, err)
	}
	if !strings.Contains(c.SQLConsolidado(), "WHERE MES_ANO = :1") || !strings.Contains(c.SQLRealizado(), "WHERE DTLANC >= TO_DATE(:1") {
		t.Error("consulta sem filtro de período")
	}
	env["JC_ORACLE_REALIZ_DATA"] = "DTPAG"
	env["JC_ORACLE_REALIZ_OBJETO"] = "IACONTR.OUTRA_VIEW"
	if c, ok, _ = ConfigDoAmbiente(get); !ok || !strings.Contains(c.SQLRealizado(), "FROM IACONTR.OUTRA_VIEW WHERE DTPAG >=") {
		t.Fatalf("%s", c.SQLRealizado())
	}
	for k, v := range map[string]string{"JC_ORACLE_REALIZ_OBJETO": "X; DROP TABLE Y", "JC_ORACLE_REALIZ_DATA": "1=1 OR D"} {
		antes := env[k]
		env[k] = v
		if _, ok, err := ConfigDoAmbiente(get); ok || err == nil {
			t.Errorf("%s=%q deveria ser recusado", k, v)
		}
		env[k] = antes
	}
	delete(env, "JC_ORACLE_PASS")
	if _, ok, err := ConfigDoAmbiente(get); ok || err != nil {
		t.Error("sem senha é não configurado, não erro")
	}
}

func TestCausaNaoLevaDadosDaConexao(t *testing.T) {
	for _, e := range []string{"ORA-01017: invalid username/password", "dial tcp 10.0.0.1:1521: i/o timeout", "ORA-00942: table", "algo oracle://u:senha@host"} {
		if m := Causa(errors.New(e)); strings.Contains(m, "10.0.0.1") || strings.Contains(m, "senha@") {
			t.Errorf("vazou: %s", m)
		}
	}
}

type leitorExclusao struct{ Leitor }

func (leitorExclusao) Realizado(context.Context, time.Time) ([]LinhaRealizado, error) {
	return []LinhaRealizado{{Filial: "1", Conta: "100", Valor: -1}, {Filial: "1", Conta: "900", Valor: -2}}, nil
}

func TestRealizadoIgnoraContasExcluidas(t *testing.T) {
	l, err := NovaRealizado(leitorExclusao{}, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), "900").Ler(context.Background(), dre.Escopo{})
	if err != nil || len(l) != 1 || l[0].Conta != "100" {
		t.Fatalf("%+v %v", l, err)
	}
}
