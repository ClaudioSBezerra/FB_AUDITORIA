package dre

// comparativo_externo.go — comparativo de fechamento (história 2.18): a
// planilha de análise do cliente fica gravada em tabelas próprias (nunca em
// `lancamentos`, para o DRE não somá-la) e é comparada, linha a linha, com o
// que o sistema tem para a mesma competência.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"golang.org/x/text/unicode/norm"

	"fb_auditoria/trilha"
)

// Status de cada linha do comparativo.
const (
	StatusOK         = "OK"
	StatusDiverge    = "DIVERGE"
	StatusSoPlanilha = "SO_PLANILHA"
	StatusSoSistema  = "SO_SISTEMA"
)

// ToleranciaComparativoPadrao é a diferença aceita (R$) entre planilha e sistema.
const ToleranciaComparativoPadrao = 1.00

// ErrComparativo é uma entrada inválida do comparativo (o handler responde 400).
var ErrComparativo = errors.New("comparativo inválido")

// ContaExterna é a soma de uma conta, num mês e numa loja, da aba Real da planilha.
type ContaExterna struct {
	Competencia time.Time
	Filial      string
	Conta       string
	Valor       float64
}

// LinhaExterna é uma linha do DRE da planilha (aba MENSAL) num mês.
type LinhaExterna struct {
	Competencia time.Time
	Chave       string
	Descricao   string
	Valor       float64
}

var (
	reNaoAlnumLinha = regexp.MustCompile(`[^a-z0-9]+`)
	reEspacosLinha  = regexp.MustCompile(`\s+`)
)

// SlugLinha gera a chave de uma linha sem código a partir da descrição: sem
// marcas iniciais como "(=)" e "(-)", sem acento, minúsculas, o resto
// não alfanumérico vira "_". É a mesma regra do gerador da estrutura inicial.
func SlugLinha(desc string) string {
	d := strings.TrimSpace(desc)
	for strings.HasPrefix(d, "(") {
		i := strings.Index(d, ")")
		if i < 0 {
			break
		}
		d = strings.TrimSpace(d[i+1:])
	}
	var b strings.Builder
	for _, r := range norm.NFD.String(d) {
		if r >= 0x300 && r <= 0x36f {
			continue
		}
		b.WriteRune(r)
	}
	s := strings.ToLower(reEspacosLinha.ReplaceAllString(strings.TrimSpace(b.String()), " "))
	s = strings.Trim(reNaoAlnumLinha.ReplaceAllString(s, "_"), "_")
	if s == "" {
		s = "linha"
	}
	return s
}

// ComparativoExterno é o serviço de carga da planilha de análise.
type ComparativoExterno struct{ DB *sql.DB }

// CargaExternaIn é a planilha já lida (o parse fica em dre/fontes/planilha).
type CargaExternaIn struct {
	EmpresaID, UserID, Arquivo string
	Ano                        int
	Visao, Filial              string
	Contas                     []ContaExterna
	Linhas                     []LinhaExterna
	Ignoradas                  int
	Avisos                     []string
}

// CargaExternaOut resume o que foi gravado.
type CargaExternaOut struct {
	CargaID   int64    `json:"carga_id"`
	Ano       int      `json:"ano"`
	Visao     string   `json:"visao"`
	Filial    string   `json:"filial"`
	Contas    int      `json:"contas"`
	Linhas    int      `json:"linhas"`
	Meses     int      `json:"meses"`
	Filiais   int      `json:"filiais"`
	Ignoradas int      `json:"ignoradas"`
	Avisos    []string `json:"avisos"`
}

func fmtCents(v float64) string { return strconv.FormatFloat(cents(v), 'f', 2, 64) }

// Carregar grava a planilha e substitui o escopo do ano: as contas de todas as
// lojas do ano e as linhas do DRE da filial da planilha. Tudo na mesma
// transação, sob lock por empresa, com a trilha (só contagens, nunca valores).
func (s *ComparativoExterno) Carregar(ctx context.Context, in CargaExternaIn) (CargaExternaOut, error) {
	out := CargaExternaOut{Ano: in.Ano, Visao: in.Visao, Filial: in.Filial, Ignoradas: in.Ignoradas, Avisos: append([]string{}, in.Avisos...)}
	if len(in.Contas) == 0 && len(in.Linhas) == 0 {
		return out, &ErroEntrada{Msg: "a planilha não tem contas nem linhas do DRE para gravar"}
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('comparativo-externo:' || $1))`, in.EmpresaID); err != nil {
		return out, err
	}
	ini := time.Date(in.Ano, 1, 1, 0, 0, 0, 0, time.UTC)
	fim := time.Date(in.Ano, 12, 1, 0, 0, 0, 0, time.UTC)
	if err := tx.QueryRowContext(ctx, `INSERT INTO auditoria.comparativo_externo_cargas (empresa_id, user_id, arquivo, ano, visao, filial)
		VALUES ($1::uuid, NULLIF($2,'')::uuid, $3, $4, $5, $6) RETURNING id`,
		in.EmpresaID, in.UserID, in.Arquivo, in.Ano, in.Visao, in.Filial).Scan(&out.CargaID); err != nil {
		return out, err
	}

	meses, filiais := map[string]bool{}, map[string]bool{}
	if len(in.Contas) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM auditoria.comparativo_externo_contas
			WHERE empresa_id = $1::uuid AND competencia BETWEEN $2 AND $3`, in.EmpresaID, ini, fim); err != nil {
			return out, err
		}
		st, err := tx.PrepareContext(ctx, pq.CopyInSchema("auditoria", "comparativo_externo_contas",
			"empresa_id", "carga_id", "competencia", "filial", "conta", "valor"))
		if err != nil {
			return out, err
		}
		for _, c := range in.Contas {
			if _, err := st.ExecContext(ctx, in.EmpresaID, out.CargaID, c.Competencia.Format("2006-01-02"), c.Filial, c.Conta, fmtCents(c.Valor)); err != nil {
				st.Close()
				return out, err
			}
			meses[c.Competencia.Format("2006-01")] = true
			filiais[c.Filial] = true
		}
		if _, err := st.ExecContext(ctx); err != nil {
			st.Close()
			return out, err
		}
		if err := st.Close(); err != nil {
			return out, err
		}
		out.Contas = len(in.Contas)
	}
	if len(in.Linhas) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM auditoria.comparativo_externo_linhas
			WHERE empresa_id = $1::uuid AND competencia BETWEEN $2 AND $3 AND filial = $4`, in.EmpresaID, ini, fim, in.Filial); err != nil {
			return out, err
		}
		st, err := tx.PrepareContext(ctx, pq.CopyInSchema("auditoria", "comparativo_externo_linhas",
			"empresa_id", "carga_id", "competencia", "filial", "linha_chave", "descricao", "valor"))
		if err != nil {
			return out, err
		}
		for _, l := range in.Linhas {
			if _, err := st.ExecContext(ctx, in.EmpresaID, out.CargaID, l.Competencia.Format("2006-01-02"), in.Filial, l.Chave, l.Descricao, fmtCents(l.Valor)); err != nil {
				st.Close()
				return out, err
			}
			meses[l.Competencia.Format("2006-01")] = true
		}
		if _, err := st.ExecContext(ctx); err != nil {
			st.Close()
			return out, err
		}
		if err := st.Close(); err != nil {
			return out, err
		}
		out.Linhas = len(in.Linhas)
		if len(in.Contas) == 0 {
			filiais[in.Filial] = true
		}
	}
	out.Meses, out.Filiais = len(meses), len(filiais)
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "comparativo_externo", strconv.FormatInt(out.CargaID, 10), "carregar", map[string]any{
		"arquivo": in.Arquivo, "ano": in.Ano, "contas": out.Contas, "linhas": out.Linhas,
		"meses": out.Meses, "filiais": out.Filiais, "ignoradas": out.Ignoradas,
		"filial_resolvida": in.Filial != "", "avisos": len(out.Avisos),
	}); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// ParamsComparativo são os parâmetros do comparativo.
type ParamsComparativo struct {
	Ano, Mes   int
	Filial     string // vazio: no nível linha, a filial resolvida da carga
	Nivel      string // "linha" ou "conta"
	Tolerancia float64
	Origem     string   // vazio = todas as origens de lançamentos (só nível conta)
	Status     []string // filtro da lista (o resumo não é filtrado); vazio = todos
	Limite     int
	Offset     int
}

// ItemComparativo é uma linha ou conta do comparativo.
type ItemComparativo struct {
	Chave        string   `json:"chave"`
	Descricao    string   `json:"descricao"`
	Status       string   `json:"status"`
	Planilha     *float64 `json:"planilha"`
	Sistema      *float64 `json:"sistema"`
	Diferenca    float64  `json:"diferenca"` // planilha − sistema
	DiferencaPct *float64 `json:"diferenca_pct,omitempty"`
	Subtotal     bool     `json:"subtotal,omitempty"`
	ordem        int
}

// ResumoComparativo conta os status e soma as diferenças (valor absoluto).
type ResumoComparativo struct {
	OK             int     `json:"ok"`
	Diverge        int     `json:"diverge"`
	SoPlanilha     int     `json:"so_planilha"`
	SoSistema      int     `json:"so_sistema"`
	SomaDiferencas float64 `json:"soma_diferencas"`
	TotalPlanilha  float64 `json:"total_planilha"`
	TotalSistema   float64 `json:"total_sistema"`
	// TotaisValidos: false no nível linha (somar subtotais dobraria a conta).
	TotaisValidos bool `json:"totais_validos"`
}

// CargaInfo diz de qual carga vem o lado da planilha.
type CargaInfo struct {
	ID      int64  `json:"id"`
	Arquivo string `json:"arquivo"`
	Quando  string `json:"quando"`
	Usuario string `json:"usuario"`
}

// Comparativo é o resultado.
type Comparativo struct {
	Competencia string            `json:"competencia"`
	Nivel       string            `json:"nivel"`
	Filial      string            `json:"filial"`
	Origem      string            `json:"origem"`
	Tolerancia  float64           `json:"tolerancia"`
	Fechada     bool              `json:"fechada"`
	Carga       *CargaInfo        `json:"carga"`
	Resumo      ResumoComparativo `json:"resumo"`
	Total       int               `json:"total"` // itens da lista (após o filtro de status)
	Linhas      []ItemComparativo `json:"linhas"`
	Avisos      []string          `json:"avisos"`
	todas       []ItemComparativo
}

// Todas devolve a lista completa (após o filtro de status, sem paginação), para exportação.
func (c *Comparativo) Todas() []ItemComparativo { return c.todas }

type ladoExt struct {
	valor float64
	desc  string
}

func ptr(v float64) *float64 { return &v }

// Comparar compara a planilha gravada com o sistema na competência.
func Comparar(ctx context.Context, q Consultador, empresaID string, p ParamsComparativo) (*Comparativo, error) {
	if p.Nivel == "" {
		p.Nivel = "linha"
	}
	if p.Nivel != "linha" && p.Nivel != "conta" {
		return nil, errors.Join(ErrComparativo, errors.New("nivel deve ser linha ou conta"))
	}
	if p.Ano < 2000 || p.Ano > 2100 || p.Mes < 1 || p.Mes > 12 {
		return nil, errors.Join(ErrComparativo, errors.New("ano e mes inválidos"))
	}
	if p.Tolerancia < 0 || math.IsNaN(p.Tolerancia) || math.IsInf(p.Tolerancia, 0) {
		return nil, errors.Join(ErrComparativo, errors.New("tolerância inválida"))
	}
	comp := time.Date(p.Ano, time.Month(p.Mes), 1, 0, 0, 0, 0, time.UTC)
	filial := NormFilialConferencia(p.Filial)
	out := &Comparativo{
		Competencia: comp.Format("2006-01"), Nivel: p.Nivel, Origem: strings.TrimSpace(p.Origem), Tolerancia: p.Tolerancia,
		Linhas: []ItemComparativo{}, Avisos: []string{},
	}
	if p.Nivel == "conta" && filial == "" {
		return nil, errors.Join(ErrComparativo, errors.New("filial é obrigatória no nível conta"))
	}
	var anoTemCarga bool
	if err := uma(ctx, q, func(r *sql.Rows) error { return r.Scan(&anoTemCarga) },
		`SELECT EXISTS (SELECT 1 FROM auditoria.comparativo_externo_cargas WHERE empresa_id = $1::uuid AND ano = $2)`, empresaID, p.Ano); err != nil {
		return nil, err
	}
	if !anoTemCarga {
		out.Avisos = append(out.Avisos, fmt.Sprintf("Nenhuma planilha carregada para o ano %d.", p.Ano))
	}

	var plan map[string]ladoExt
	var cargaID int64
	var err error
	if p.Nivel == "conta" {
		out.Filial = filial
		plan, cargaID, err = planilhaContas(ctx, q, empresaID, comp, filial)
		if err != nil {
			return nil, err
		}
		if anoTemCarga && len(plan) == 0 {
			out.Avisos = append(out.Avisos, fmt.Sprintf("A planilha carregada não tem contas da filial %s em %s.", filial, out.Competencia))
		}
		err = compararContas(ctx, q, empresaID, comp, filial, out.Origem, plan, out)
	} else {
		if filial == "" {
			filial, err = filialDasLinhas(ctx, q, empresaID, p.Ano)
			if err != nil {
				return nil, err
			}
		}
		out.Filial = filial
		if filial == "" {
			if anoTemCarga {
				out.Avisos = append(out.Avisos, "Nenhuma carga de linhas do DRE com filial resolvida neste ano: salve a planilha com uma filial selecionada na aba MENSAL e envie de novo, ou informe a filial.")
			}
		} else {
			plan, cargaID, err = planilhaLinhas(ctx, q, empresaID, comp, filial)
			if err != nil {
				return nil, err
			}
			if anoTemCarga && len(plan) == 0 {
				out.Avisos = append(out.Avisos, fmt.Sprintf("Nenhuma carga de linhas do DRE para a filial %s em %s.", filial, out.Competencia))
			}
			err = compararLinhas(ctx, q, empresaID, comp, filial, plan, out)
		}
	}
	if err != nil {
		return nil, err
	}
	if cargaID > 0 {
		var ci CargaInfo
		if err := uma(ctx, q, func(r *sql.Rows) error { return r.Scan(&ci.ID, &ci.Arquivo, &ci.Quando, &ci.Usuario) },
			`SELECT c.id, c.arquivo, to_char(c.criado_em AT TIME ZONE 'America/Sao_Paulo', 'YYYY-MM-DD HH24:MI'), COALESCE(u.full_name, '')
			 FROM auditoria.comparativo_externo_cargas c LEFT JOIN users u ON u.id = c.user_id
			 WHERE c.empresa_id = $1::uuid AND c.id = $2`, empresaID, cargaID); err != nil {
			return nil, err
		}
		out.Carga = &ci
	}
	if err := uma(ctx, q, func(r *sql.Rows) error { return r.Scan(&out.Fechada) },
		`SELECT EXISTS (SELECT 1 FROM auditoria.dre_fechamentos WHERE empresa_id = $1::uuid AND competencia = $2 AND reaberto_em IS NULL)`,
		empresaID, comp); err != nil {
		return nil, err
	}
	finalizar(out, p)
	return out, nil
}

// uma executa uma consulta e lê a primeira linha; sem linha, o destino fica como está.
func uma(ctx context.Context, q Consultador, ler func(*sql.Rows) error, query string, args ...any) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		if err := ler(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
