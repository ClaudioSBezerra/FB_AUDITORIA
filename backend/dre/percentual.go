package dre

// PapelFaturamentoLiquido marca a linha cujo valor é a base dos percentuais.
const PapelFaturamentoLiquido = "faturamento_liquido"

// Situação do percentual sobre o Faturamento Líquido.
const (
	PctOK       = "ok"
	PctSemPapel = "sem_papel" // nenhuma linha com o papel: não se calcula
	PctZero     = "zero"      // Faturamento Líquido zero: "não calculável"
)

// CodigoComPapel devolve o código da linha ativa com o papel ('' se não houver).
func CodigoComPapel(linhas []Linha, papel string) string {
	for _, l := range linhas {
		if l.Papel == papel && l.Ativo {
			return l.Codigo
		}
	}
	return ""
}

// Percentual é valor/base*100; nil quando a base é zero.
func Percentual(valor, base float64) *float64 {
	if base == 0 {
		return nil
	}
	x := valor / base * 100
	return &x
}

// PercentuaisSobreFaturamento calcula, para cada linha, o percentual do valor
// sobre o valor da linha-base (codBase). Situação PctSemPapel (codBase vazio),
// PctZero (base zero, percentuais nil) ou PctOK.
func PercentuaisSobreFaturamento(linhas []Linha, valores map[string]float64, codBase string) (map[string]*float64, string) {
	if codBase == "" {
		return nil, PctSemPapel
	}
	base := valores[codBase]
	out := make(map[string]*float64, len(linhas))
	for _, l := range linhas {
		out[l.Codigo] = Percentual(valores[l.Codigo], base)
	}
	if base == 0 {
		return out, PctZero
	}
	return out, PctOK
}

// CodigoBase escolhe a linha-base dos percentuais da estrutura exibida. Se ela
// é a congelada de um fechamento anterior à 3.4 (sem papel gravado), vale o
// papel vivo, desde que a linha exista na estrutura exibida.
func CodigoBase(exibidas, vivas []Linha, congelada bool) string {
	cod := CodigoComPapel(exibidas, PapelFaturamentoLiquido)
	if cod != "" || !congelada {
		return cod
	}
	vivo := CodigoComPapel(vivas, PapelFaturamentoLiquido)
	for _, l := range exibidas {
		if l.Codigo == vivo {
			return vivo
		}
	}
	return ""
}
