package dre

import (
	"context"
	"sort"
	"time"
)

// meu_departamento.go — "Meu departamento" do Gerente (história 4.8, FR-18/FR-8).
//
// Realizado, Orçado e Ano anterior POR CONTA dentro do escopo do Gerente.
// - Realizado e Ano anterior: mês fechado vem de dre_fechamento_contas (valor,
//   ano_anterior e departamento congelados); mês aberto, ao vivo (lançamentos do
//   período e do mesmo período do ano anterior, mesmo mapa e dicionário).
// - Orçado: não existe por conta (dre_orcado é por linha e filial; ver 2.7).
//   Fica nil, nunca zero e nunca rateado, até haver a fonte por conta.
// - Ano anterior nil quando a conta não teve movimento no ano anterior, ou
//   quando algum mês do período é de um fechamento legado (sem contas): nesse
//   caso a leitura inteira não tem ano anterior confiável (AnoAnteriorIndisponivel).

// ContaComparativo é uma conta com o Realizado e o Ano anterior do período.
type ContaComparativo struct {
	ContaDepto
	AnoAnterior *float64
}

// ComparativoContas é o resultado de ContasComparativo.
type ComparativoContas struct {
	Contas []ContaComparativo
	// SemContas lista os meses fechados em fechamento legado (sem contas).
	SemContas []string
	// AnoAnteriorIndisponivel: há mês legado; nenhuma conta traz ano anterior.
	AnoAnteriorIndisponivel bool
}

type chaveCD struct{ conta, dep string }

type acumCD struct {
	c    ContaDepto
	aa   float64
	temA bool
}

// ContasComparativo lê as contas do período para o recorte de filiais e devolve
// só as do escopo (valor zero nega sem consultar). O casamento de conta por
// departamento acontece em Go (ResolvedorDepartamento / snapshot), nunca em SQL.
func (l Leitor) ContasComparativo(ctx context.Context, empresaID string, filiais []string, p Periodo, mapa []MapaItem, ativos map[string]Ativo, esc EscopoDepto) (ComparativoContas, error) {
	if esc.Vazio() {
		return ComparativoContas{Contas: []ContaComparativo{}}, nil
	}
	acc := map[chaveCD]*acumCD{}
	sem := map[string]bool{}
	var resolvedor *ResolvedorDepartamento
	for _, filial := range filiais {
		if err := l.acumularFilial(ctx, empresaID, filial, p, mapa, ativos, acc, sem, &resolvedor); err != nil {
			return ComparativoContas{}, err
		}
	}
	out := ComparativoContas{Contas: []ContaComparativo{}, AnoAnteriorIndisponivel: len(sem) > 0}
	for m := range sem {
		out.SemContas = append(out.SemContas, m)
	}
	sort.Strings(out.SemContas)
	for _, a := range acc {
		c := ContaComparativo{ContaDepto: a.c}
		c.Valor = arredondaCentavos(c.Valor)
		if a.temA && !out.AnoAnteriorIndisponivel {
			v := arredondaCentavos(a.aa)
			c.AnoAnterior = &v
		}
		if c.Valor == 0 && c.AnoAnterior == nil {
			continue
		}
		if esc.Permite(c.Departamento) {
			out.Contas = append(out.Contas, c)
		}
	}
	sort.Slice(out.Contas, func(i, j int) bool {
		if out.Contas[i].Conta != out.Contas[j].Conta {
			return out.Contas[i].Conta < out.Contas[j].Conta
		}
		return out.Contas[i].Departamento < out.Contas[j].Departamento
	})
	return out, nil
}

func (l Leitor) acumularFilial(ctx context.Context, empresaID, filial string, p Periodo, mapa []MapaItem, ativos map[string]Ativo, acc map[chaveCD]*acumCD, sem map[string]bool, resolvedor **ResolvedorDepartamento) error {
	somar := func(conta, linha, dep string, real, aa float64) {
		k := chaveCD{conta, dep}
		x, ok := acc[k]
		if !ok {
			x = &acumCD{c: ContaDepto{Conta: conta, Linha: linha, Departamento: dep}}
			acc[k] = x
		}
		x.c.Valor += real
		if aa != 0 {
			x.aa += aa
			x.temA = true
		}
	}
	var iniVivo, fimVivo time.Time
	vivoAberto := false
	descarregar := func() error {
		if !vivoAberto {
			return nil
		}
		vivoAberto = false
		if *resolvedor == nil {
			r, err := CarregarResolvedorDepartamento(ctx, l.Q, empresaID)
			if err != nil {
				return err
			}
			*resolvedor = r
		}
		dep := func(conta string) string {
			if d := (*resolvedor).De(conta); d != nil {
				return d.Codigo
			}
			return ""
		}
		per := Periodo{Ini: iniVivo, Fim: fimVivo}
		real, err := RealizadoPorConta(ctx, l.Q, empresaID, filial, per, mapa)
		if err != nil {
			return err
		}
		aa, err := RealizadoPorConta(ctx, l.Q, empresaID, filial, Periodo{Ini: per.Ini.AddDate(-1, 0, 0), Fim: per.Fim.AddDate(-1, 0, 0)}, mapa)
		if err != nil {
			return err
		}
		for conta, v := range real {
			somar(conta, v.Linha, dep(conta), v.Valor, aa[conta].Valor)
		}
		for conta, v := range aa {
			if _, ok := real[conta]; !ok {
				somar(conta, v.Linha, dep(conta), 0, v.Valor)
			}
		}
		return nil
	}
	for _, m := range meses(p) {
		a, fechado := ativos[chaveMes(m)]
		o := Vivo
		if fechado {
			o = a.Origem(filial)
		}
		switch o {
		case Vivo:
			if !vivoAberto {
				iniVivo, vivoAberto = m, true
			}
			fimVivo = m
		case Legado:
			if err := descarregar(); err != nil {
				return err
			}
			sem[chaveMes(m)] = true
		case Snapshot:
			if err := descarregar(); err != nil {
				return err
			}
			rows, err := l.Q.QueryContext(ctx, `SELECT conta, linha_codigo, departamento, realizado, ano_anterior
				FROM auditoria.dre_fechamento_contas WHERE fechamento_id = $1 AND filial = $2 AND (realizado <> 0 OR ano_anterior <> 0)`, a.ID, filial)
			if err != nil {
				return err
			}
			for rows.Next() {
				var conta, linha, dep string
				var real, aa float64
				if err = rows.Scan(&conta, &linha, &dep, &real, &aa); err != nil {
					break
				}
				somar(conta, linha, dep, real, aa)
			}
			if err == nil {
				err = rows.Err()
			}
			rows.Close()
			if err != nil {
				return err
			}
		}
	}
	return descarregar()
}

// Variacao é Realizado − base, em valor e em percentual da base. Os dois são nil
// quando não há base (sem orçado / sem ano anterior); o percentual também é nil
// quando a base é zero ("não calculável").
type Variacao struct {
	Valor *float64
	Pct   *float64
}

// VariacaoSobre calcula a variação do realizado sobre a base (nil = sem base).
func VariacaoSobre(real float64, base *float64) Variacao {
	if base == nil {
		return Variacao{}
	}
	v := arredondaCentavos(real - *base)
	var pct *float64
	if *base != 0 {
		x := v / abs(*base) * 100
		pct = &x
	}
	return Variacao{Valor: &v, Pct: pct}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// SomaOpcional soma os valores presentes; nil se nenhum está presente.
func SomaOpcional(vs ...*float64) *float64 {
	var s float64
	tem := false
	for _, v := range vs {
		if v != nil {
			s += *v
			tem = true
		}
	}
	if !tem {
		return nil
	}
	s = arredondaCentavos(s)
	return &s
}

// ---- Montagem -------------------------------------------------------------------

// MeuContaVisao é uma conta de "Meu departamento".
type MeuContaVisao struct {
	Conta          string   `json:"conta"`
	Descricao      string   `json:"descricao"`
	Linha          string   `json:"linha"`
	Realizado      float64  `json:"realizado"`
	Orcado         *float64 `json:"orcado"`
	AnoAnterior    *float64 `json:"ano_anterior"`
	VarOrcado      *float64 `json:"var_orcado_valor"`
	VarOrcadoPct   *float64 `json:"var_orcado_pct"`
	VarAA          *float64 `json:"var_ano_anterior_valor"`
	VarAAPct       *float64 `json:"var_ano_anterior_pct"`
	PctFaturamento *float64 `json:"pct_faturamento"`
}

// MeuDeptoVisao é um departamento do escopo com contas e totais.
type MeuDeptoVisao struct {
	Codigo string          `json:"codigo"`
	Nome   string          `json:"nome"`
	Grupo  string          `json:"grupo"`
	Totais MeuContaVisao   `json:"totais"`
	Contas []MeuContaVisao `json:"contas"`
}

// MeuDepartamentoVisao é o resultado de MontarMeuDepartamento.
type MeuDepartamentoVisao struct {
	Departamentos []MeuDeptoVisao `json:"departamentos"`
	Total         MeuContaVisao   `json:"total"`
}

func preencher(v *MeuContaVisao, base float64) {
	vo := VariacaoSobre(v.Realizado, v.Orcado)
	v.VarOrcado, v.VarOrcadoPct = vo.Valor, vo.Pct
	va := VariacaoSobre(v.Realizado, v.AnoAnterior)
	v.VarAA, v.VarAAPct = va.Valor, va.Pct
	v.PctFaturamento = Percentual(v.Realizado, base)
}

// MontarMeuDepartamento agrupa as contas do escopo por departamento. base é o
// Faturamento Líquido do recorte, usada só para o percentual: nunca é devolvida.
// Departamento que saiu do cadastro não aparece (o Gerente nunca vê "Sem departamento").
func MontarMeuDepartamento(contas []ContaComparativo, deps []Departamento, desc *DescritorConta, base float64) MeuDepartamentoVisao {
	porCodigo := map[string]Departamento{}
	for _, d := range deps {
		porCodigo[d.Codigo] = d
	}
	por := map[string][]ContaComparativo{}
	for _, c := range contas {
		if _, ok := porCodigo[c.Departamento]; ok && c.Departamento != "" {
			por[c.Departamento] = append(por[c.Departamento], c)
		}
	}
	codigos := make([]string, 0, len(por))
	for cod := range por {
		codigos = append(codigos, cod)
	}
	sort.Strings(codigos)
	res := MeuDepartamentoVisao{Departamentos: []MeuDeptoVisao{}}
	var totReal float64
	var aas []*float64
	for _, cod := range codigos {
		d := porCodigo[cod]
		dv := MeuDeptoVisao{Codigo: cod, Nome: d.Nome, Grupo: d.Grupo, Contas: []MeuContaVisao{}}
		var real float64
		var daa []*float64
		for _, c := range por[cod] {
			cv := MeuContaVisao{Conta: c.Conta, Descricao: desc.De(c.Conta), Linha: c.Linha, Realizado: c.Valor, AnoAnterior: c.AnoAnterior}
			preencher(&cv, base)
			dv.Contas = append(dv.Contas, cv)
			real += c.Valor
			daa = append(daa, c.AnoAnterior)
		}
		dv.Totais = MeuContaVisao{Conta: cod, Descricao: d.Nome, Realizado: arredondaCentavos(real), AnoAnterior: SomaOpcional(daa...)}
		preencher(&dv.Totais, base)
		res.Departamentos = append(res.Departamentos, dv)
		totReal += real
		aas = append(aas, dv.Totais.AnoAnterior)
	}
	res.Total = MeuContaVisao{Descricao: "Total do escopo", Realizado: arredondaCentavos(totReal), AnoAnterior: SomaOpcional(aas...)}
	preencher(&res.Total, base)
	return res
}
