package planilha

import "testing"

func TestEhLinhaDeTotal(t *testing.T) {
	sim := []string{"Total Geral", "(vazio)", "TOTAL", " Resultado Resumo ",
		"Calculo para prova real Margem (saldo Bruto menos Outros Fatur)",
		"Calculo para prova real Despesa", "Diferença (tem que ser zero)"}
	for _, c := range sim {
		if !ehLinhaDeTotal(c) {
			t.Errorf("%q deveria ser linha de total", c)
		}
	}
	nao := []string{"1", "2", "3", "3020008", "FAT_TRANS_", "DESC CUSTO", "ST", "2500001"}
	for _, c := range nao {
		if ehLinhaDeTotal(c) {
			t.Errorf("%q não é linha de total", c)
		}
	}
}
