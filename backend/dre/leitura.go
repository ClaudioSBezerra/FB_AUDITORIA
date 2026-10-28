package dre

import (
	"context"
	"time"
)

// OrcadoPorLinha soma o orçado (por linha 'conta') do período.
func OrcadoPorLinha(ctx context.Context, q Consultador, empresaID, filial string, p Periodo) (map[string]float64, error) {
	rows, err := q.QueryContext(ctx, `SELECT linha_codigo, SUM(valor) FROM auditoria.dre_orcado
		WHERE empresa_id = $1::uuid AND competencia BETWEEN $2 AND $3 AND ($4 = '' OR filial = $4)
		GROUP BY linha_codigo`, empresaID, p.Ini, p.Fim, filial)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var c string
		var v float64
		if err := rows.Scan(&c, &v); err != nil {
			return nil, err
		}
		out[c] = v
	}
	return out, rows.Err()
}

// Origem de um mês na leitura.
type Origem int

const (
	// Vivo: competência sem fechamento ativo; lido de lançamentos, mapa e estrutura.
	Vivo Origem = iota
	// Snapshot: fechamento ativo completo (linhas com orçado e ano anterior, por filial, e contas).
	Snapshot
	// Legado: fechamento ativo anterior à 1.8, só com linhas de realizado.
	Legado
)

// Ativo é o fechamento ativo de uma competência.
type Ativo struct {
	ID          int64
	Versao      int
	Competencia time.Time
	Filial      string // do fechamento (legado por filial); '' nos novos
	Completo    bool   // gravado com o snapshot completo (1.8)
}

// Origem diz de onde se lê o mês para a filial pedida.
func (a Ativo) Origem(filial string) Origem {
	if a.Completo {
		return Snapshot
	}
	if a.Filial == filial {
		return Legado
	}
	return Vivo
}

func chaveMes(t time.Time) string { return t.Format("2006-01") }

func primeiroDia(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// Leitor decide, mês a mês, se a leitura vem do snapshot do fechamento ativo
// ou ao vivo. É a regra única de leitura do DRE (AD-17).
type Leitor struct{ Q Consultador }

// Ativos devolve os fechamentos ativos das competências do período, por "AAAA-MM".
func (l Leitor) Ativos(ctx context.Context, empresaID string, p Periodo) (map[string]Ativo, error) {
	rows, err := l.Q.QueryContext(ctx, `SELECT f.id, f.competencia, f.versao, f.filial,
			EXISTS (SELECT 1 FROM auditoria.dre_fechamento_linhas x WHERE x.fechamento_id = f.id AND x.orcado IS NOT NULL)
		FROM auditoria.dre_fechamentos f
		WHERE f.empresa_id = $1::uuid AND f.competencia BETWEEN $2 AND $3 AND f.reaberto_em IS NULL`,
		empresaID, primeiroDia(p.Ini), primeiroDia(p.Fim))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Ativo{}
	for rows.Next() {
		var a Ativo
		if err := rows.Scan(&a.ID, &a.Competencia, &a.Versao, &a.Filial, &a.Completo); err != nil {
			return nil, err
		}
		a.Competencia = primeiroDia(a.Competencia)
		out[chaveMes(a.Competencia)] = a
	}
	return out, rows.Err()
}

// Leitura é o resultado da leitura do período: realizado, orçado e ano anterior por
// linha (subtotais inclusive) e as contas sem mapa (só de meses ao vivo).
type Leitura struct {
	Real, Orc, AA, Sem map[string]float64
	Ativos             map[string]Ativo
}

func somar(dst, src map[string]float64) {
	for k, v := range src {
		dst[k] += v
	}
}

func meses(p Periodo) []time.Time {
	var out []time.Time
	for m := primeiroDia(p.Ini); !m.After(p.Fim); m = m.AddDate(0, 1, 0) {
		out = append(out, m)
	}
	return out
}

// Ler lê o período mês a mês: mês fechado vem só do snapshot do fechamento
// ativo (nem lançamentos, nem mapa, nem estrutura); mês aberto ou reaberto,
// ao vivo. O acumulado soma linha a linha, subtotal com subtotal.
func (l Leitor) Ler(ctx context.Context, empresaID, filial string, p Periodo, linhas []Linha, mapa []MapaItem) (Leitura, error) {
	ativos, err := l.Ativos(ctx, empresaID, p)
	if err != nil {
		return Leitura{}, err
	}
	r := Leitura{Real: map[string]float64{}, Orc: map[string]float64{}, AA: map[string]float64{}, Sem: map[string]float64{}, Ativos: ativos}

	// Meses ao vivo consecutivos viram uma consulta só.
	var iniVivo, fimVivo time.Time
	vivoAberto := false
	descarregar := func() error {
		if !vivoAberto {
			return nil
		}
		vivoAberto = false
		return l.vivo(ctx, empresaID, filial, Periodo{Ini: iniVivo, Fim: fimVivo}, linhas, mapa, true, &r)
	}
	for _, m := range meses(p) {
		a, fechado := ativos[chaveMes(m)]
		o := Vivo
		if fechado {
			o = a.Origem(filial)
		}
		if o == Vivo {
			if !vivoAberto {
				iniVivo, vivoAberto = m, true
			}
			fimVivo = m
			continue
		}
		if err := descarregar(); err != nil {
			return Leitura{}, err
		}
		if o == Snapshot {
			if err := l.snapshot(ctx, a, filial, &r); err != nil {
				return Leitura{}, err
			}
			continue
		}
		// Legado: realizado do fechamento; orçado e ano anterior ao vivo, como antes.
		rows, err := l.Q.QueryContext(ctx, `SELECT linha_codigo, valor FROM auditoria.dre_fechamento_linhas WHERE fechamento_id = $1`, a.ID)
		if err != nil {
			return Leitura{}, err
		}
		for rows.Next() {
			var cod string
			var v float64
			if err := rows.Scan(&cod, &v); err != nil {
				rows.Close()
				return Leitura{}, err
			}
			r.Real[cod] += v
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return Leitura{}, err
		}
		if err := l.vivo(ctx, empresaID, filial, Periodo{Ini: m, Fim: m}, linhas, mapa, false, &r); err != nil {
			return Leitura{}, err
		}
	}
	if err := descarregar(); err != nil {
		return Leitura{}, err
	}
	return r, nil
}

// vivo soma ao resultado o intervalo lido ao vivo (realizado só se comReal).
func (l Leitor) vivo(ctx context.Context, empresaID, filial string, p Periodo, linhas []Linha, mapa []MapaItem, comReal bool, r *Leitura) error {
	if comReal {
		real, sem, err := Realizado(ctx, l.Q, empresaID, filial, p, mapa)
		if err != nil {
			return err
		}
		AplicarSubtotais(linhas, real)
		somar(r.Real, real)
		somar(r.Sem, sem)
	}
	aa, _, err := Realizado(ctx, l.Q, empresaID, filial, Periodo{Ini: p.Ini.AddDate(-1, 0, 0), Fim: p.Fim.AddDate(-1, 0, 0)}, mapa)
	if err != nil {
		return err
	}
	AplicarSubtotais(linhas, aa)
	somar(r.AA, aa)
	orc, err := OrcadoPorLinha(ctx, l.Q, empresaID, filial, p)
	if err != nil {
		return err
	}
	AplicarSubtotais(linhas, orc)
	somar(r.Orc, orc)
	return nil
}

func (l Leitor) snapshot(ctx context.Context, a Ativo, filial string, r *Leitura) error {
	rows, err := l.Q.QueryContext(ctx, `SELECT linha_codigo, valor, COALESCE(orcado,0), COALESCE(ano_anterior,0)
		FROM auditoria.dre_fechamento_linhas WHERE fechamento_id = $1 AND filial = $2`, a.ID, filial)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cod string
		var v, o, aa float64
		if err := rows.Scan(&cod, &v, &o, &aa); err != nil {
			return err
		}
		r.Real[cod] += v
		r.Orc[cod] += o
		r.AA[cod] += aa
	}
	return rows.Err()
}

// ContaCongelada é o realizado de uma conta num mês fechado.
type ContaCongelada struct {
	Competencia time.Time
	Conta       string
	Linha       string
	Valor       float64
}

// ContasCongeladas devolve as contas do snapshot dos meses fechados do período
// cuja linha esteja em alvo, e os meses fechados sem contas (fechamento legado).
func (l Leitor) ContasCongeladas(ctx context.Context, filial string, ativos map[string]Ativo, alvo map[string]bool) (contas []ContaCongelada, semContas []string, err error) {
	for chave, a := range ativos {
		switch a.Origem(filial) {
		case Legado:
			semContas = append(semContas, chave)
		case Snapshot:
			rows, e := l.Q.QueryContext(ctx, `SELECT conta, linha_codigo, realizado FROM auditoria.dre_fechamento_contas
				WHERE fechamento_id = $1 AND filial = $2 AND realizado <> 0`, a.ID, filial)
			if e != nil {
				return nil, nil, e
			}
			for rows.Next() {
				var c ContaCongelada
				if e := rows.Scan(&c.Conta, &c.Linha, &c.Valor); e != nil {
					rows.Close()
					return nil, nil, e
				}
				if alvo[c.Linha] {
					c.Competencia = a.Competencia
					contas = append(contas, c)
				}
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return nil, nil, e
			}
		}
	}
	return contas, semContas, nil
}

// EstruturaCongelada devolve a estrutura (código, descrição, nível, ordem, tipo)
// gravada no fechamento quando TODOS os meses do período são lidos do snapshot;
// usa o fechamento do último mês. Em período misto (ou legado) devolve false e
// a estrutura viva continua valendo.
func (l Leitor) EstruturaCongelada(ctx context.Context, filial string, p Periodo, ativos map[string]Ativo) ([]Linha, bool, error) {
	ms := meses(p)
	for _, m := range ms {
		a, ok := ativos[chaveMes(m)]
		if !ok || a.Origem(filial) != Snapshot {
			return nil, false, nil
		}
	}
	a := ativos[chaveMes(ms[len(ms)-1])]
	rows, err := l.Q.QueryContext(ctx, `SELECT linha_codigo, descricao, nivel, ordem, COALESCE(tipo,'conta'), COALESCE(papel,'')
		FROM auditoria.dre_fechamento_linhas WHERE fechamento_id = $1 AND filial = '' ORDER BY ordem`, a.ID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []Linha
	for rows.Next() {
		var x Linha
		if err := rows.Scan(&x.Codigo, &x.Descricao, &x.Nivel, &x.Ordem, &x.Tipo, &x.Papel); err != nil {
			return nil, false, err
		}
		x.Ativo = true
		out = append(out, x)
	}
	return out, len(out) > 0, rows.Err()
}
