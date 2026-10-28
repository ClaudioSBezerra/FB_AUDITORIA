package dre

import (
	"context"
	"time"
)

// RotuloAno é o rótulo da coluna do ano inteiro na visão por trimestre.
const RotuloAno = "Ano"

// ColunaTrimestre é uma coluna da visão por trimestre (T1 a T4 ou Ano): os
// mapas por linha (subtotais inclusive) e a situação dos meses.
type ColunaTrimestre struct {
	Rotulo        string
	Ini, Fim      time.Time
	Vazio         bool     // trimestre que ainda não começou no ano em curso
	Parcial       bool     // algum mês ainda está aberto
	MesesFechados []string // "AAAA-MM" dos meses lidos do snapshot
	Real, Orc, AA map[string]float64
	Sem           map[string]float64 // contas sem mapa (meses ao vivo)
	// Estrutura exibida da coluna (a congelada, se todos os meses estão fechados
	// com snapshot completo) e a linha-base dos percentuais dela.
	Estrutura []Linha
	Congelada bool
	CodBase   string
}

func mapaVazio() map[string]float64 { return map[string]float64{} }

func arredondarMapa(m map[string]float64) {
	for k, v := range m {
		m[k] = arredondaCentavos(v)
	}
}

// LerTrimestres lê o ano em quatro trimestres e no total. Cada trimestre é a
// soma de três meses lidos por Leitor.LerFiliais (mês fechado do snapshot, mês
// aberto ao vivo): nada é relido por fora da regra única de leitura. Trimestre
// que começa depois de `hoje` vem vazio. A coluna Ano é a soma das quatro, de
// modo que Ano = T1+T2+T3+T4 exatamente. linhas é a estrutura viva (usada para
// os subtotais dos meses ao vivo e como reserva de papel dos percentuais).
func (l Leitor) LerTrimestres(ctx context.Context, empresaID string, filiais []string, ano int, hoje time.Time, linhas []Linha, mapa []MapaItem) ([]ColunaTrimestre, error) {
	anoPer := Periodo{Ini: time.Date(ano, 1, 1, 0, 0, 0, 0, time.UTC), Fim: time.Date(ano, 12, 1, 0, 0, 0, 0, time.UTC)}
	ativosAno, err := l.Ativos(ctx, empresaID, anoPer)
	if err != nil {
		return nil, err
	}
	primeiroMesAtual := time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, time.UTC)
	rotulos := []string{"T1", "T2", "T3", "T4"}
	cols := make([]ColunaTrimestre, 0, 5)
	total := ColunaTrimestre{Rotulo: RotuloAno, Ini: anoPer.Ini, Fim: anoPer.Fim, Real: mapaVazio(), Orc: mapaVazio(), AA: mapaVazio(), Sem: mapaVazio(), MesesFechados: []string{}}
	for i, rot := range rotulos {
		ini := time.Date(ano, time.Month(3*i+1), 1, 0, 0, 0, 0, time.UTC)
		c := ColunaTrimestre{Rotulo: rot, Ini: ini, Fim: ini.AddDate(0, 2, 0), MesesFechados: []string{},
			Real: mapaVazio(), Orc: mapaVazio(), AA: mapaVazio(), Sem: mapaVazio()}
		if ini.After(primeiroMesAtual) {
			c.Vazio = true
			cols = append(cols, c)
			continue
		}
		p := Periodo{Ini: c.Ini, Fim: c.Fim}
		lei, err := l.LerFiliais(ctx, empresaID, filiais, p, linhas, mapa)
		if err != nil {
			return nil, err
		}
		c.Real, c.Orc, c.AA, c.Sem = lei.Real, lei.Orc, lei.AA, lei.Sem
		for _, mes := range meses(p) {
			if a, ok := ativosAno[chaveMes(mes)]; ok && a.OrigemFiliais(filiais) != Vivo {
				c.MesesFechados = append(c.MesesFechados, chaveMes(mes))
			}
		}
		c.Parcial = len(c.MesesFechados) < 3
		if cong, ok, err := l.EstruturaCongelada(ctx, filiais[0], p, ativosAno); err != nil {
			return nil, err
		} else if ok {
			c.Estrutura, c.Congelada = cong, true
		} else {
			c.Estrutura = linhas
		}
		c.CodBase = CodigoBase(c.Estrutura, linhas, c.Congelada)
		arredondarMapa(c.Real)
		arredondarMapa(c.Orc)
		arredondarMapa(c.AA)
		somar(total.Real, c.Real)
		somar(total.Orc, c.Orc)
		somar(total.AA, c.AA)
		somar(total.Sem, c.Sem)
		total.MesesFechados = append(total.MesesFechados, c.MesesFechados...)
		cols = append(cols, c)
	}
	arredondarMapa(total.Real)
	arredondarMapa(total.Orc)
	arredondarMapa(total.AA)
	for _, c := range cols {
		if !c.Vazio && c.Parcial {
			total.Parcial = true
		}
	}
	total.Vazio = true
	for _, c := range cols {
		if !c.Vazio {
			total.Vazio = false
		}
	}
	if total.Vazio {
		total.Parcial = false
	}
	// Estrutura do Ano: a congelada se os doze meses estão no snapshot completo.
	if cong, ok, err := l.EstruturaCongelada(ctx, filiais[0], anoPer, ativosAno); err != nil {
		return nil, err
	} else if ok {
		total.Estrutura, total.Congelada = cong, true
	} else {
		total.Estrutura = linhas
	}
	total.CodBase = CodigoBase(total.Estrutura, linhas, total.Congelada)
	return append(cols, total), nil
}
