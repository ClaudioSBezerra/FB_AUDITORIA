package dre

import (
	"context"
	"sort"
	"time"
)

// SemDepartamento é o rótulo do balde das contas sem departamento.
const SemDepartamento = "Sem departamento"

// ContaDepto é o realizado de uma conta, já com o sinal do Mapa, e o código do
// departamento ('' = sem departamento).
type ContaDepto struct {
	Conta        string
	Linha        string
	Departamento string
	Valor        float64
}

// ContasPorDepartamento lê as contas do período mês a mês: mês fechado vem do
// snapshot (valor e departamento gravados no Fechar, sem consultar lançamentos,
// mapa nem dicionário); mês aberto ou reaberto, ao vivo, com o departamento
// resolvido em lote. Fechamento legado (sem contas) vem em semContas.
func (l Leitor) ContasPorDepartamento(ctx context.Context, empresaID, filial string, p Periodo, mapa []MapaItem, ativos map[string]Ativo) (contas []ContaDepto, semContas []string, err error) {
	type chave struct{ conta, dep string }
	acc := map[chave]*ContaDepto{}
	somar := func(c ContaDepto) {
		k := chave{c.Conta, c.Departamento}
		if x, ok := acc[k]; ok {
			x.Valor += c.Valor
			return
		}
		acc[k] = &c
	}

	var resolvedor *ResolvedorDepartamento
	var iniVivo, fimVivo time.Time
	vivoAberto := false
	descarregar := func() error {
		if !vivoAberto {
			return nil
		}
		vivoAberto = false
		if resolvedor == nil {
			if resolvedor, err = CarregarResolvedorDepartamento(ctx, l.Q, empresaID); err != nil {
				return err
			}
		}
		por, e := RealizadoPorConta(ctx, l.Q, empresaID, filial, Periodo{Ini: iniVivo, Fim: fimVivo}, mapa)
		if e != nil {
			return e
		}
		for conta, v := range por {
			dep := ""
			if d := resolvedor.De(conta); d != nil {
				dep = d.Codigo
			}
			somar(ContaDepto{Conta: conta, Linha: v.Linha, Departamento: dep, Valor: v.Valor})
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
			semContas = append(semContas, chaveMes(m))
		case Snapshot:
			if err := descarregar(); err != nil {
				return nil, nil, err
			}
			rows, e := l.Q.QueryContext(ctx, `SELECT conta, linha_codigo, departamento, realizado
				FROM auditoria.dre_fechamento_contas WHERE fechamento_id = $1 AND filial = $2 AND realizado <> 0`, a.ID, filial)
			if e != nil {
				return nil, nil, e
			}
			for rows.Next() {
				var c ContaDepto
				if e = rows.Scan(&c.Conta, &c.Linha, &c.Departamento, &c.Valor); e != nil {
					break
				}
				somar(c)
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			if e != nil {
				return nil, nil, e
			}
		}
	}
	if err := descarregar(); err != nil {
		return nil, nil, err
	}
	for _, c := range acc {
		if c.Valor != 0 {
			contas = append(contas, *c)
		}
	}
	sort.Strings(semContas)
	return contas, semContas, nil
}

// ContaVisao é uma conta na visão por departamento.
type ContaVisao struct {
	Conta     string   `json:"conta"`
	Descricao string   `json:"descricao"`
	Linha     string   `json:"linha"`
	Valor     float64  `json:"valor"`
	Pct       *float64 `json:"pct_faturamento"`
}

// DeptoVisao é um departamento com suas contas, total e percentual.
type DeptoVisao struct {
	Codigo string       `json:"codigo"`
	Nome   string       `json:"nome"`
	Total  float64      `json:"total"`
	Pct    *float64     `json:"pct_faturamento"`
	Contas []ContaVisao `json:"contas"`
}

// GrupoVisao agrupa departamentos (Comercial, Logística, Administrativo).
type GrupoVisao struct {
	Grupo         string       `json:"grupo"`
	Total         float64      `json:"total"`
	Pct           *float64     `json:"pct_faturamento"`
	Departamentos []DeptoVisao `json:"departamentos"`
}

// VisaoDepartamentos é o resultado de MontarVisao.
type VisaoDepartamentos struct {
	Grupos          []GrupoVisao `json:"grupos"`
	SemDepartamento *DeptoVisao  `json:"sem_departamento"`
}

// MontarVisao agrupa as contas por departamento e grupo. base é o Faturamento
// Líquido do recorte (percentual nil se zero). Conta cujo departamento não está
// mais no cadastro cai em "Sem departamento". Sem departamento só entra com
// incluirSem (perfis sem restrição). Totais somam valores de 2 casas, arredondados.
func MontarVisao(contas []ContaDepto, deps []Departamento, desc *DescritorConta, base float64, incluirSem bool) VisaoDepartamentos {
	porCodigo := map[string]Departamento{}
	for _, d := range deps {
		porCodigo[d.Codigo] = d
	}
	montar := func(cs []ContaDepto, d Departamento) DeptoVisao {
		out := DeptoVisao{Codigo: d.Codigo, Nome: d.Nome, Contas: []ContaVisao{}}
		sort.Slice(cs, func(i, j int) bool { return cs[i].Conta < cs[j].Conta })
		for _, c := range cs {
			v := arredondaCentavos(c.Valor)
			out.Contas = append(out.Contas, ContaVisao{Conta: c.Conta, Descricao: desc.De(c.Conta), Linha: c.Linha, Valor: v, Pct: Percentual(v, base)})
			out.Total += v
		}
		out.Total = arredondaCentavos(out.Total)
		out.Pct = Percentual(out.Total, base)
		return out
	}
	porDep := map[string][]ContaDepto{}
	var sem []ContaDepto
	for _, c := range contas {
		if _, ok := porCodigo[c.Departamento]; ok && c.Departamento != "" {
			porDep[c.Departamento] = append(porDep[c.Departamento], c)
		} else {
			sem = append(sem, c)
		}
	}
	res := VisaoDepartamentos{Grupos: []GrupoVisao{}}
	for _, g := range GruposDepartamento {
		gv := GrupoVisao{Grupo: g, Departamentos: []DeptoVisao{}}
		var codigos []string
		for _, d := range deps {
			if d.Grupo == g && len(porDep[d.Codigo]) > 0 {
				codigos = append(codigos, d.Codigo)
			}
		}
		sort.Strings(codigos)
		for _, cod := range codigos {
			dv := montar(porDep[cod], porCodigo[cod])
			gv.Departamentos = append(gv.Departamentos, dv)
			gv.Total += dv.Total
		}
		gv.Total = arredondaCentavos(gv.Total)
		gv.Pct = Percentual(gv.Total, base)
		res.Grupos = append(res.Grupos, gv)
	}
	if incluirSem && len(sem) > 0 {
		dv := montar(sem, Departamento{Nome: SemDepartamento})
		res.SemDepartamento = &dv
	}
	return res
}

// Departamentos lista o cadastro de departamentos da empresa.
func (l Leitor) Departamentos(ctx context.Context, empresaID string) ([]Departamento, error) {
	return listarDepartamentos(ctx, l.Q, empresaID, "")
}
