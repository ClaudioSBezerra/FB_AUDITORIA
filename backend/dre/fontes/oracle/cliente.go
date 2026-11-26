package oracle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	go_ora "github.com/sijms/go-ora/v2"

	"fb_auditoria/dre"
)

// Config é a conexão e os objetos lidos. Vem só de variáveis de ambiente.
type Config struct {
	Host, Servico, Usuario, Senha string
	Porta                         int
	ObjetoConsolidado             string // padrão VW_DRE_CONSOLIDADO
	ObjetoRealizado               string // padrão CONTR_DRE_REALIZ
	ColunaData                    string // padrão DTLANC
	ArquivoContas                 string // vazio = o que acompanha o sistema
}

// ident é um identificador Oracle simples, com schema opcional. Nada além disso
// entra no texto de uma consulta.
var ident = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_$#]{0,127}(\.[A-Za-z][A-Za-z0-9_$#]{0,127})?$`)

var coluna = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_$#]{0,127}$`)

// ConfigDoAmbiente lê JC_ORACLE_*. Sem as obrigatórias devolve ok=false (não
// configurado); identificador inválido devolve erro.
func ConfigDoAmbiente(getenv func(string) string) (Config, bool, error) {
	v := func(k, def string) string {
		if s := strings.TrimSpace(getenv(k)); s != "" {
			return s
		}
		return def
	}
	c := Config{
		Host: v("JC_ORACLE_HOST", ""), Servico: v("JC_ORACLE_SERVICE", ""),
		Usuario: v("JC_ORACLE_USER", ""), Senha: v("JC_ORACLE_PASS", ""),
		ObjetoConsolidado: v("JC_ORACLE_CONSOLIDADO_OBJETO", "VW_DRE_CONSOLIDADO"),
		ObjetoRealizado:   v("JC_ORACLE_REALIZ_OBJETO", "CONTR_DRE_REALIZ"),
		ColunaData:        v("JC_ORACLE_REALIZ_DATA", "DTLANC"),
		ArquivoContas:     v("JC_ORACLE_CONTAS_ARQUIVO", ""),
	}
	if c.Host == "" || c.Servico == "" || c.Usuario == "" || c.Senha == "" {
		return c, false, nil
	}
	porta, err := strconv.Atoi(v("JC_ORACLE_PORT", "1521"))
	if err != nil || porta <= 0 || porta > 65535 {
		return c, false, errors.New("JC_ORACLE_PORT inválida")
	}
	c.Porta = porta
	if !ident.MatchString(c.ObjetoConsolidado) {
		return c, false, errors.New("JC_ORACLE_CONSOLIDADO_OBJETO não é um nome de objeto Oracle")
	}
	if !ident.MatchString(c.ObjetoRealizado) {
		return c, false, errors.New("JC_ORACLE_REALIZ_OBJETO não é um nome de objeto Oracle")
	}
	if !coluna.MatchString(c.ColunaData) {
		return c, false, errors.New("JC_ORACLE_REALIZ_DATA não é um nome de coluna Oracle")
	}
	return c, true, nil
}

// SQLConsolidado e SQLRealizado são as consultas; sempre filtradas por período.
func (c Config) SQLConsolidado() string {
	return "SELECT UF, CODFILIAL, MES_ANO, " + strings.Join(ColunasConsolidado, ", ") +
		" FROM " + c.ObjetoConsolidado + " WHERE MES_ANO = :1"
}

func (c Config) SQLRealizado() string {
	d := c.ColunaData
	return "SELECT CODFILIAL, TO_CHAR(CODCONTA), MAX(CONTA), TRUNC(" + d + "), SUM(VALOR) FROM " + c.ObjetoRealizado +
		" WHERE " + d + " >= TO_DATE(:1,'YYYY-MM-DD') AND " + d + " < ADD_MONTHS(TO_DATE(:2,'YYYY-MM-DD'),1)" +
		" GROUP BY CODFILIAL, CODCONTA, TRUNC(" + d + ")"
}

const (
	prazoPing     = 30 * time.Second
	prazoConsulta = 3 * time.Minute
)

// Cliente lê o Oracle do cliente; cada leitura abre e fecha a conexão.
type Cliente struct{ cfg Config }

// NovoCliente não conecta; a conexão é feita em cada leitura.
func NovoCliente(cfg Config) *Cliente { return &Cliente{cfg} }

func (c *Cliente) consultar(ctx context.Context, q string, args []any, cada func(*sql.Rows) error) error {
	dsn := go_ora.BuildUrl(c.cfg.Host, c.cfg.Porta, c.cfg.Servico, c.cfg.Usuario, c.cfg.Senha,
		map[string]string{"PREFETCH_ROWS": "2000"})
	db, err := sql.Open("oracle", dsn)
	if err != nil {
		return c.falha(err)
	}
	defer db.Close()
	pctx, cancel := context.WithTimeout(ctx, prazoPing)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		return c.falha(err)
	}
	qctx, cancelQ := context.WithTimeout(ctx, prazoConsulta)
	defer cancelQ()
	rows, err := db.QueryContext(qctx, q, args...)
	if err != nil {
		return c.falha(err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := cada(rows); err != nil {
			return c.falha(err)
		}
	}
	if err := rows.Err(); err != nil {
		return c.falha(err)
	}
	return nil
}

// falha registra o erro sem credencial nem endereço e devolve a causa em
// palavras para o usuário.
func (c *Cliente) falha(err error) error {
	txt := err.Error()
	for _, s := range []string{c.cfg.Senha, c.cfg.Host} {
		if s != "" {
			txt = strings.ReplaceAll(txt, s, "***")
		}
	}
	log.Printf("oracle: %s", txt)
	return &dre.ErroFonte{Msg: Causa(err)}
}

// Causa traduz o erro do driver em uma mensagem sem dados da conexão.
func Causa(err error) string {
	s := err.Error()
	var rede net.Error
	switch {
	case strings.Contains(s, "ORA-01017"), strings.Contains(s, "ORA-28000"), strings.Contains(s, "ORA-28001"):
		return "o banco do cliente recusou o usuário ou a senha (confira a credencial; a conta pode estar bloqueada)"
	case strings.Contains(s, "ORA-12514"), strings.Contains(s, "ORA-12505"):
		return "o banco do cliente não conhece o serviço informado (confira JC_ORACLE_SERVICE)"
	case strings.Contains(s, "ORA-00942"), strings.Contains(s, "ORA-00904"):
		return "o objeto ou a coluna não existe no banco do cliente, ou falta permissão de leitura"
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(s, "ORA-01013"):
		return "o banco do cliente demorou demais para responder"
	case errors.As(err, &rede), strings.Contains(s, "connection refused"), strings.Contains(s, "i/o timeout"), strings.Contains(s, "no route"):
		return "não foi possível alcançar o banco do cliente (rede ou liberação do IP do servidor)"
	}
	return "falha ao ler o banco do cliente (veja o log do servidor)"
}

// Consolidado implementa Leitor.
func (c *Cliente) Consolidado(ctx context.Context, comp time.Time) ([]LinhaConsolidado, error) {
	var out []LinhaConsolidado
	err := c.consultar(ctx, c.cfg.SQLConsolidado(), []any{comp.Format("01/2006")}, func(r *sql.Rows) error {
		var uf, filial, mes sql.NullString
		vals := make([]sql.NullFloat64, len(ColunasConsolidado))
		dest := []any{&uf, &filial, &mes}
		for i := range vals {
			dest = append(dest, &vals[i])
		}
		if err := r.Scan(dest...); err != nil {
			return fmt.Errorf("ler linha: %w", err)
		}
		l := LinhaConsolidado{UF: uf.String, Filial: filial.String, MesAno: mes.String, Valores: map[string]float64{}}
		for i, v := range vals {
			if v.Valid {
				l.Valores[ColunasConsolidado[i]] = v.Float64
			}
		}
		out = append(out, l)
		return nil
	})
	return out, err
}

// Realizado implementa Leitor.
func (c *Cliente) Realizado(ctx context.Context, comp time.Time) ([]LinhaRealizado, error) {
	ini := comp.Format("2006-01-02")
	var out []LinhaRealizado
	err := c.consultar(ctx, c.cfg.SQLRealizado(), []any{ini, ini}, func(r *sql.Rows) error {
		var filial, conta, desc sql.NullString
		var data sql.NullTime
		var valor sql.NullFloat64
		if err := r.Scan(&filial, &conta, &desc, &data, &valor); err != nil {
			return fmt.Errorf("ler linha: %w", err)
		}
		if !valor.Valid {
			return nil // valor nulo não é gravado
		}
		out = append(out, LinhaRealizado{Filial: filial.String, Conta: conta.String, Descricao: desc.String, Data: data.Time, Valor: valor.Float64})
		return nil
	})
	return out, err
}
