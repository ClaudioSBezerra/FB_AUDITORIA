// Package oracle são as fontes que leem o banco do cliente (histórias 2.3 e
// 2.22): a view consolidada e o realizado por conta gerencial. Só lê, sempre
// filtrando por competência; não conhece o nosso banco. O valor é entregue como
// o ERP o entrega: o sinal vive só no Mapa do DRE (AD-4).
package oracle

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"fb_auditoria/dre"
)

// Origens gravadas em lancamentos.origem.
const (
	OrigemConsolidado = dre.PrefixoOrigemERP + "consolidado"
	OrigemRealizado   = dre.PrefixoOrigemERP + "realizado"
)

// ColunasConsolidado são as colunas de valor da view consolidada, na ordem lida.
var ColunasConsolidado = []string{
	"VLFATBRUTO", "VLTOTAL_DEV", "VLTOTAL_DEVAVUSLO", "VLCMVDEVOL", "VLDEVCMVAVULSAI", "ST", "VLDESCCUSTO",
}

// LinhaConsolidado é uma linha da view consolidada. Coluna nula não está em Valores.
type LinhaConsolidado struct {
	UF, Filial, MesAno string
	Valores            map[string]float64
}

// LinhaRealizado é o realizado somado por filial, conta e dia.
type LinhaRealizado struct {
	Filial, Conta, Descricao string
	Data                     time.Time
	Valor                    float64
}

// Leitor lê o banco do cliente; a implementação real é Cliente. Os erros
// devolvidos são *dre.ErroFonte, sem credencial.
type Leitor interface {
	Consolidado(ctx context.Context, competencia time.Time) ([]LinhaConsolidado, error)
	Realizado(ctx context.Context, competencia time.Time) ([]LinhaRealizado, error)
}

// ContaColuna diz em que conta do Mapa entra uma coluna da view.
type ContaColuna struct {
	Conta     string `json:"conta"`
	Descricao string `json:"descricao"`
}

//go:embed contas_consolidado.json
var contasPadrao []byte

// CarregarContas lê a correspondência coluna → conta do arquivo indicado, ou do
// arquivo que acompanha o sistema quando caminho é vazio.
func CarregarContas(caminho string) (map[string]ContaColuna, error) {
	b := contasPadrao
	if caminho != "" {
		var err error
		if b, err = os.ReadFile(caminho); err != nil {
			return nil, fmt.Errorf("ler a correspondência de contas: %w", err)
		}
	}
	var arq struct {
		Colunas map[string]ContaColuna `json:"colunas"`
	}
	if err := json.Unmarshal(b, &arq); err != nil {
		return nil, fmt.Errorf("correspondência de contas inválida: %w", err)
	}
	conhecidas := map[string]bool{}
	for _, c := range ColunasConsolidado {
		conhecidas[c] = true
		if _, ok := arq.Colunas[c]; !ok {
			return nil, fmt.Errorf("correspondência de contas: falta a coluna %s (use conta vazia para não importar)", c)
		}
	}
	usadas := 0
	for col, cc := range arq.Colunas {
		if !conhecidas[col] {
			return nil, fmt.Errorf("correspondência de contas: coluna desconhecida %s", col)
		}
		cc.Conta = strings.TrimSpace(cc.Conta)
		arq.Colunas[col] = cc
		if cc.Conta != "" {
			usadas++
		}
	}
	if usadas == 0 {
		return nil, fmt.Errorf("correspondência de contas: nenhuma coluna tem conta")
	}
	return arq.Colunas, nil
}

// ParseMesAno converte o MES_ANO da view (MM/AAAA).
func ParseMesAno(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) != 7 {
		return time.Time{}, fmt.Errorf("MES_ANO inválido %q (esperado MM/AAAA)", s)
	}
	t, err := time.Parse("01/2006", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("MES_ANO inválido %q (esperado MM/AAAA)", s)
	}
	return t, nil
}

type fonteConsolidado struct {
	l      Leitor
	comp   time.Time
	contas map[string]ContaColuna
}

// NovaConsolidado é a fonte da view consolidada para uma competência.
func NovaConsolidado(l Leitor, comp time.Time, contas map[string]ContaColuna) dre.FonteERP {
	return &fonteConsolidado{l, comp, contas}
}

func (f *fonteConsolidado) Nome() string        { return OrigemConsolidado }
func (f *fonteConsolidado) CompetenciaInteira() {}
func (f *fonteConsolidado) Arquivo() string     { return "ERP consolidado " + f.comp.Format("01/2006") }

func (f *fonteConsolidado) Ler(ctx context.Context, _ dre.Escopo) ([]dre.Lancamento, error) {
	linhas, err := f.l.Consolidado(ctx, f.comp)
	if err != nil {
		return nil, err
	}
	var out []dre.Lancamento
	var probs []dre.Problema
	for i, l := range linhas {
		comp, err := ParseMesAno(l.MesAno)
		if err != nil {
			probs = append(probs, dre.Problema{Linha: i + 1, Msg: err.Error()})
			continue
		}
		if !comp.Equal(f.comp) {
			probs = append(probs, dre.Problema{Linha: i + 1, Msg: "MES_ANO fora da competência pedida: " + l.MesAno})
			continue
		}
		for _, col := range ColunasConsolidado {
			cc := f.contas[col]
			v, tem := l.Valores[col]
			if cc.Conta == "" || !tem {
				continue // coluna não importada, ou valor nulo
			}
			out = append(out, dre.Lancamento{Linha: i + 1, Competencia: comp, Filial: dre.NormFilialConferencia(l.Filial),
				Conta: cc.Conta, ContaDesc: cc.Descricao, Valor: v, UF: strings.TrimSpace(l.UF), Historico: col})
		}
	}
	if len(probs) > 0 {
		return out, &dre.ErroLinhas{Problemas: probs}
	}
	return out, nil
}

type fonteRealizado struct {
	l    Leitor
	comp time.Time
}

// NovaRealizado é a fonte do realizado por conta gerencial para uma competência.
func NovaRealizado(l Leitor, comp time.Time) dre.FonteERP { return &fonteRealizado{l, comp} }

func (f *fonteRealizado) Nome() string        { return OrigemRealizado }
func (f *fonteRealizado) CompetenciaInteira() {}
func (f *fonteRealizado) Arquivo() string     { return "ERP realizado " + f.comp.Format("01/2006") }

func (f *fonteRealizado) Ler(ctx context.Context, _ dre.Escopo) ([]dre.Lancamento, error) {
	linhas, err := f.l.Realizado(ctx, f.comp)
	if err != nil {
		return nil, err
	}
	out := make([]dre.Lancamento, 0, len(linhas))
	for i, l := range linhas {
		hist := ""
		if !l.Data.IsZero() {
			hist = "data " + l.Data.Format("02/01/2006")
		}
		out = append(out, dre.Lancamento{Linha: i + 1, Competencia: f.comp, Filial: dre.NormFilialConferencia(l.Filial),
			Conta: strings.TrimSpace(l.Conta), ContaDesc: strings.TrimSpace(l.Descricao), Valor: l.Valor, Historico: hist})
	}
	return out, nil
}
