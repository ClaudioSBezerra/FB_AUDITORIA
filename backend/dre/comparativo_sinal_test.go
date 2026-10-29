package dre

import "testing"

func TestMarcarSinalInvertido(t *testing.T) {
	cas := []struct {
		nome      string
		plan, sis *float64
		statusFim string
		obs       string
	}{
		{"sinal trocado", ptr(-857226.09), ptr(857226.09), StatusOK, "sinal invertido"},
		{"dentro da tolerância", ptr(-100.00), ptr(100.50), StatusOK, "sinal invertido"},
		{"valores diferentes", ptr(-100), ptr(250), StatusDiverge, ""},
		{"mesmo sinal", ptr(100), ptr(150), StatusDiverge, ""},
		{"planilha zero", ptr(0), ptr(500), StatusDiverge, ""},
		{"só planilha", ptr(-10), nil, StatusSoPlanilha, ""},
	}
	for _, c := range cas {
		it := novoItem(1.00, "2", "Devoluções", c.plan, c.sis)
		marcarSinalInvertido(&it, 1.00)
		if it.Status != c.statusFim || it.Obs != c.obs {
			t.Errorf("%s: status %q obs %q, esperado %q %q", c.nome, it.Status, it.Obs, c.statusFim, c.obs)
		}
	}
}
