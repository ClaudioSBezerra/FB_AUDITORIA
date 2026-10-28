package dre

import "testing"

func TestVariacaoSobre(t *testing.T) {
	f := func(x float64) *float64 { return &x }
	if v := VariacaoSobre(100, nil); v.Valor != nil || v.Pct != nil {
		t.Errorf("sem base deve ser nil: %+v", v)
	}
	if v := VariacaoSobre(100, f(0)); v.Valor == nil || *v.Valor != 100 || v.Pct != nil {
		t.Errorf("base zero: valor 100 e pct não calculável: %+v", v)
	}
	if v := VariacaoSobre(-30, f(-60)); *v.Valor != 30 || *v.Pct != 50 {
		t.Errorf("base negativa usa módulo: %+v %v", *v.Valor, *v.Pct)
	}
	if SomaOpcional(nil, nil) != nil || *SomaOpcional(nil, f(2.5)) != 2.5 {
		t.Error("SomaOpcional")
	}
}

func TestContasComparativo_EscopoZeroNegaSemConsultar(t *testing.T) {
	r, err := Leitor{}.ContasComparativo(nil, "e", []string{""}, Periodo{}, nil, nil, EscopoDepto{})
	if err != nil || len(r.Contas) != 0 {
		t.Errorf("%v %+v", err, r)
	}
}
