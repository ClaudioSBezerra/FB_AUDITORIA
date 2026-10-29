package dre

// comparativo_externo_ler.go — os dois lados do comparativo (planilha gravada e
// sistema), a classificação e a ordenação.

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"strings"
	"time"
)

const normFilialSQL = `CASE WHEN filial ~ '^[0-9]+$' THEN COALESCE(NULLIF(LTRIM(filial, '0'), ''), '0') ELSE filial END`

func planilhaContas(ctx context.Context, q Consultador, empresaID string, comp time.Time, filial string) (map[string]ladoExt, int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT conta, valor::float8, carga_id FROM auditoria.comparativo_externo_contas
		WHERE empresa_id = $1::uuid AND competencia = $2 AND filial = $3`, empresaID, comp, filial)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := map[string]ladoExt{}
	var maxCarga int64
	for rows.Next() {
		var conta string
		var v float64
		var c int64
		if err := rows.Scan(&conta, &v, &c); err != nil {
			return nil, 0, err
		}
		out[conta] = ladoExt{valor: v}
		if c > maxCarga {
			maxCarga = c
		}
	}
	return out, maxCarga, rows.Err()
}

func planilhaLinhas(ctx context.Context, q Consultador, empresaID string, comp time.Time, filial string) (map[string]ladoExt, int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT linha_chave, descricao, valor::float8, carga_id FROM auditoria.comparativo_externo_linhas
		WHERE empresa_id = $1::uuid AND competencia = $2 AND filial = $3`, empresaID, comp, filial)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := map[string]ladoExt{}
	var maxCarga int64
	for rows.Next() {
		var chave, desc string
		var v float64
		var c int64
		if err := rows.Scan(&chave, &desc, &v, &c); err != nil {
			return nil, 0, err
		}
		out[chave] = ladoExt{valor: v, desc: desc}
		if c > maxCarga {
			maxCarga = c
		}
	}
	return out, maxCarga, rows.Err()
}

// filialDasLinhas devolve a filial resolvida da carga de linhas mais recente do ano.
func filialDasLinhas(ctx context.Context, q Consultador, empresaID string, ano int) (string, error) {
	var f string
	err := uma(ctx, q, func(r *sql.Rows) error { return r.Scan(&f) },
		`SELECT filial FROM auditoria.comparativo_externo_linhas
		 WHERE empresa_id = $1::uuid AND filial <> '' AND competencia BETWEEN $2 AND $3
		 ORDER BY carga_id DESC, filial LIMIT 1`, empresaID, time.Date(ano, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(ano, 12, 1, 0, 0, 0, 0, time.UTC))
	return f, err
}

func difPct(plan, dif float64) *float64 {
	if plan == 0 {
		return nil
	}
	x := dif / math.Abs(plan) * 100
	return &x
}

// classificar compara os dois lados (nil = não existe). Um lado só, de valor zero,
// conta como OK: os dois lados valem zero.
func classificar(tol float64, plan, sis *float64) (string, float64) {
	var p, s float64
	if plan != nil {
		p = cents(*plan)
	}
	if sis != nil {
		s = cents(*sis)
	}
	dif := cents(p - s)
	switch {
	case plan != nil && sis != nil:
		if math.Abs(dif) > tol+1e-9 {
			return StatusDiverge, dif
		}
		return StatusOK, dif
	case dif == 0:
		return StatusOK, 0
	case plan != nil:
		return StatusSoPlanilha, dif
	default:
		return StatusSoSistema, dif
	}
}

func novoItem(tol float64, chave, desc string, plan, sis *float64) ItemComparativo {
	st, dif := classificar(tol, plan, sis)
	it := ItemComparativo{Chave: chave, Descricao: desc, Status: st, Diferenca: dif}
	if plan != nil {
		it.Planilha = ptr(cents(*plan))
		it.DiferencaPct = difPct(*it.Planilha, dif)
	}
	if sis != nil {
		it.Sistema = ptr(cents(*sis))
	}
	return it
}

// marcarSinalInvertido trata como OK a conta que diverge só pelo sinal: o
// sistema guarda o valor bruto e a planilha já traz o sinal aplicado (devoluções,
// por exemplo). O item segue visível, com a observação.
func marcarSinalInvertido(it *ItemComparativo, tol float64) {
	if it.Status != StatusDiverge || it.Planilha == nil || it.Sistema == nil || *it.Planilha == 0 {
		return
	}
	if math.Abs(cents(*it.Planilha+*it.Sistema)) > tol+1e-9 {
		return
	}
	it.Status, it.Obs = StatusOK, "sinal invertido"
}

func compararContas(ctx context.Context, q Consultador, empresaID string, comp time.Time, filial, origem string, plan map[string]ladoExt, out *Comparativo) error {
	rows, err := q.QueryContext(ctx, `SELECT conta, SUM(valor)::float8, COALESCE(MAX(NULLIF(conta_desc, '')), '')
		FROM auditoria.lancamentos
		WHERE empresa_id = $1::uuid AND competencia = $2 AND `+normFilialSQL+` = $3 AND ($4 = '' OR origem = $4)
		GROUP BY conta`, empresaID, comp, filial, origem)
	if err != nil {
		return err
	}
	sis := map[string]ladoExt{}
	for rows.Next() {
		var conta, desc string
		var v float64
		if err := rows.Scan(&conta, &v, &desc); err != nil {
			rows.Close()
			return err
		}
		sis[conta] = ladoExt{valor: v, desc: desc}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	dic, err := CarregarDescritorConta(ctx, q, empresaID)
	if err != nil {
		return err
	}
	chaves := map[string]bool{}
	for c := range plan {
		chaves[c] = true
	}
	for c := range sis {
		chaves[c] = true
	}
	for conta := range chaves {
		var pp, ss *float64
		desc := dic.De(conta)
		if l, ok := plan[conta]; ok {
			pp = ptr(l.valor)
		}
		if l, ok := sis[conta]; ok {
			ss = ptr(l.valor)
			if desc == "" {
				desc = l.desc
			}
		}
		out.todas = append(out.todas, novoItem(out.Tolerancia, conta, desc, pp, ss))
	}
	out.Resumo.TotaisValidos = true
	return nil
}

func compararLinhas(ctx context.Context, q Consultador, empresaID string, comp time.Time, filial string, plan map[string]ladoExt, out *Comparativo) error {
	linhas, err := CarregarLinhas(ctx, q, empresaID)
	if err != nil {
		return err
	}
	mapa, err := CarregarMapa(ctx, q, empresaID)
	if err != nil {
		return err
	}
	per := Periodo{Ini: comp, Fim: comp}
	leitor := Leitor{Q: q}
	lei, err := leitor.Ler(ctx, empresaID, filial, per, linhas, mapa)
	if err != nil {
		return err
	}
	if cong, ok, err := leitor.EstruturaCongelada(ctx, filial, per, lei.Ativos); err != nil {
		return err
	} else if ok {
		linhas = cong
	}
	if len(linhas) == 0 {
		out.Avisos = append(out.Avisos, "O sistema não tem estrutura do DRE carregada: as linhas da planilha aparecem como só planilha.")
	}

	// Índices da estrutura do sistema: código, slug do código e slug da descrição.
	porCodigo := map[string]int{}
	porSlugCod := map[string]int{}
	porSlugDesc := map[string]int{}
	for i, l := range linhas {
		porCodigo[strings.ToLower(l.Codigo)] = i
		if _, ok := porSlugCod[SlugLinha(l.Codigo)]; !ok {
			porSlugCod[SlugLinha(l.Codigo)] = i
		}
		if _, ok := porSlugDesc[SlugLinha(l.Descricao)]; !ok {
			porSlugDesc[SlugLinha(l.Descricao)] = i
		}
	}
	usadas := map[int]bool{}
	achar := func(chave, desc string) (int, bool) {
		tent := []struct {
			m map[string]int
			k string
		}{
			{porCodigo, strings.ToLower(chave)},
			{porSlugCod, SlugLinha(chave)},
			{porSlugDesc, SlugLinha(chave)},
			{porSlugDesc, SlugLinha(desc)},
		}
		for _, t := range tent {
			if i, ok := t.m[t.k]; ok && !usadas[i] {
				return i, true
			}
		}
		return 0, false
	}

	// Ordem estável: a da chave, para o casamento não depender da ordem do mapa.
	chaves := make([]string, 0, len(plan))
	for c := range plan {
		chaves = append(chaves, c)
	}
	sort.Strings(chaves)
	for _, chave := range chaves {
		l := plan[chave]
		pp := ptr(l.valor)
		i, ok := achar(chave, l.desc)
		desc := l.desc
		if !ok {
			if desc == "" {
				desc = chave
			}
			out.todas = append(out.todas, novoItem(out.Tolerancia, chave, desc, pp, nil))
			continue
		}
		usadas[i] = true
		sl := linhas[i]
		if desc == "" {
			desc = sl.Descricao
		}
		it := novoItem(out.Tolerancia, chave, desc, pp, ptr(lei.Real[sl.Codigo]))
		it.Subtotal, it.ordem = sl.Tipo == "subtotal", sl.Ordem
		out.todas = append(out.todas, it)
	}
	for i, sl := range linhas {
		if usadas[i] {
			continue
		}
		v := cents(lei.Real[sl.Codigo])
		if sl.Tipo != "subtotal" && v == 0 {
			continue
		}
		it := novoItem(out.Tolerancia, sl.Codigo, sl.Descricao, nil, ptr(v))
		it.Subtotal, it.ordem = sl.Tipo == "subtotal", sl.Ordem
		out.todas = append(out.todas, it)
	}
	return nil
}

func ordemStatus(s string) int {
	switch s {
	case StatusDiverge:
		return 0
	case StatusSoPlanilha:
		return 1
	case StatusSoSistema:
		return 2
	}
	return 3
}

func valorAbs(it ItemComparativo) float64 { return math.Abs(it.Diferenca) }

// finalizar calcula o resumo (sobre tudo), ordena, filtra por status e pagina.
func finalizar(out *Comparativo, p ParamsComparativo) {
	r := &out.Resumo
	for _, it := range out.todas {
		switch it.Status {
		case StatusOK:
			r.OK++
		case StatusDiverge:
			r.Diverge++
		case StatusSoPlanilha:
			r.SoPlanilha++
		case StatusSoSistema:
			r.SoSistema++
		}
		r.SomaDiferencas += math.Abs(it.Diferenca)
		if r.TotaisValidos {
			if it.Planilha != nil {
				r.TotalPlanilha += *it.Planilha
			}
			if it.Sistema != nil {
				r.TotalSistema += *it.Sistema
			}
		}
	}
	r.SomaDiferencas, r.TotalPlanilha, r.TotalSistema = cents(r.SomaDiferencas), cents(r.TotalPlanilha), cents(r.TotalSistema)

	sort.SliceStable(out.todas, func(i, j int) bool {
		x, y := out.todas[i], out.todas[j]
		if ox, oy := ordemStatus(x.Status), ordemStatus(y.Status); ox != oy {
			return ox < oy
		}
		if x.Status != StatusOK {
			if ax, ay := valorAbs(x), valorAbs(y); ax != ay {
				return ax > ay
			}
		} else if x.ordem != y.ordem {
			return x.ordem < y.ordem
		}
		return x.Chave < y.Chave
	})
	if len(p.Status) > 0 {
		want := map[string]bool{}
		for _, s := range p.Status {
			want[strings.ToUpper(strings.TrimSpace(s))] = true
		}
		f := out.todas[:0:0]
		for _, it := range out.todas {
			if want[it.Status] {
				f = append(f, it)
			}
		}
		out.todas = f
	}
	out.Total = len(out.todas)
	lim := p.Limite
	if lim <= 0 {
		lim = 500
	}
	out.Linhas = pagina(out.todas, p.Offset, lim, out.Linhas)
}
