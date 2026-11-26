package dre

// importacao_erp.go — orquestra a importação do banco do cliente (histórias 2.4,
// 2.5 e 2.22; AD-15): mês a mês, uma transação por fonte, com o registro em
// auditoria.execucoes e na trilha. A exclusão mútua usa um advisory lock numa
// conexão dedicada. Este arquivo não conhece o Oracle: recebe as fontes prontas.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"fb_auditoria/trilha"
)

// MaxMesesImportacao limita o período de um pedido.
const MaxMesesImportacao = 12

// Situação de uma execução.
const (
	ExecucaoExecutando   = "executando"
	ExecucaoConcluida    = "concluida"
	ExecucaoComRessalvas = "concluida_com_ressalvas"
	ExecucaoFalha        = "falha"
)

// Situação de um item (competência × origem).
const (
	ItemAplicado  = "aplicada"
	ItemFechado   = "pulada_mes_fechado"
	ItemSemDados  = "sem_dados"
	ItemRejeitado = "rejeitada"
	ItemFalha     = "falha"
)

// ItemImportacao é o que aconteceu com uma origem numa competência.
type ItemImportacao struct {
	Competencia string `json:"competencia"`
	Origem      string `json:"origem"`
	Status      string `json:"status"`
	Linhas      int    `json:"linhas"`
	Mensagem    string `json:"mensagem,omitempty"`
}

// Execucao é o registro de uma importação.
type Execucao struct {
	ID       int64            `json:"id"`
	Gatilho  string           `json:"gatilho"`
	De       string           `json:"de"`
	Ate      string           `json:"ate"`
	Inicio   time.Time        `json:"inicio"`
	Fim      *time.Time       `json:"fim,omitempty"`
	Status   string           `json:"status"`
	Mensagem string           `json:"mensagem"`
	Itens    []ItemImportacao `json:"itens"`
}

// ErrImportacaoEmAndamento: já há uma importação rodando para a empresa.
var ErrImportacaoEmAndamento = errors.New("já há uma importação em andamento")

// ImportacaoERP importa do banco do cliente para uma única empresa.
type ImportacaoERP struct {
	DB        *sql.DB
	Ingestor  *Ingestor
	EmpresaID string                       // empresa dona do ERP (JC_EMPRESA_ID)
	Fontes    func(comp time.Time) []Fonte // fontes de uma competência
	// Observacoes aparecem na tela (ex.: o que o ERP não traz).
	Observacoes []string
}

// PedidoImportacao é uma importação aceita, com o lock tomado.
type PedidoImportacao struct {
	ID    int64
	e     EscopoAtualizacao
	meses []time.Time
	conn  *sql.Conn
}

// MesesDoPeriodo valida e expande o período AAAA-MM.
func MesesDoPeriodo(de, ate string, hoje time.Time) ([]time.Time, error) {
	d, err1 := time.Parse("2006-01", strings.TrimSpace(de))
	a, err2 := time.Parse("2006-01", strings.TrimSpace(ate))
	if err1 != nil || err2 != nil {
		return nil, &ErroEntrada{Msg: "período inválido: informe de e ate como AAAA-MM"}
	}
	if a.Before(d) {
		return nil, &ErroEntrada{Msg: "período inválido: o fim é anterior ao início"}
	}
	if a.After(time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, time.UTC)) {
		return nil, &ErroEntrada{Msg: "período inválido: competência no futuro"}
	}
	var out []time.Time
	for c := d; !c.After(a); c = c.AddDate(0, 1, 0) {
		if len(out) == MaxMesesImportacao {
			return nil, &ErroEntrada{Msg: fmt.Sprintf("período grande demais: no máximo %d meses por vez", MaxMesesImportacao)}
		}
		out = append(out, c)
	}
	return out, nil
}

// Iniciar toma o lock, registra a execução e o pedido na trilha. Quem chama deve
// em seguida chamar Executar (que solta o lock).
func (i *ImportacaoERP) Iniciar(ctx context.Context, e EscopoAtualizacao, gatilho string, meses []time.Time) (*PedidoImportacao, error) {
	if len(meses) == 0 {
		return nil, &ErroEntrada{Msg: "período vazio"}
	}
	conn, err := i.DB.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var ok bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, "importacao_erp:"+e.EmpresaID).Scan(&ok); err != nil {
		conn.Close()
		return nil, err
	}
	if !ok {
		conn.Close()
		return nil, ErrImportacaoEmAndamento
	}
	p := &PedidoImportacao{e: e, meses: meses, conn: conn}
	if err := i.registrarInicio(ctx, p, gatilho); err != nil {
		soltar(conn, e.EmpresaID)
		return nil, err
	}
	return p, nil
}

func (i *ImportacaoERP) registrarInicio(ctx context.Context, p *PedidoImportacao, gatilho string) error {
	tx, err := i.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Com o lock na mão, execução ainda "executando" é de um processo que caiu.
	if _, err := tx.ExecContext(ctx, `UPDATE auditoria.execucoes SET status=$2, fim=now(), mensagem='interrompida (o servidor reiniciou)'
		WHERE empresa_id=$1::uuid AND status=$3`, p.e.EmpresaID, ExecucaoFalha, ExecucaoExecutando); err != nil {
		return err
	}
	de, ate := p.meses[0], p.meses[len(p.meses)-1]
	if err := tx.QueryRowContext(ctx, `INSERT INTO auditoria.execucoes (empresa_id, user_id, gatilho, de, ate)
		VALUES ($1::uuid, NULLIF($2,'')::uuid, $3, $4, $5) RETURNING id`, p.e.EmpresaID, p.e.UserID, gatilho, de, ate).Scan(&p.ID); err != nil {
		return err
	}
	if err := trilha.Gravar(tx, p.e.EmpresaID, p.e.UserID, "importacao_erp", fmt.Sprint(p.ID), "solicitar",
		map[string]any{"gatilho": gatilho, "de": de.Format("2006-01"), "ate": ate.Format("2006-01")}); err != nil {
		return err
	}
	return tx.Commit()
}

// soltar libera o lock na mesma conexão; se falhar, a conexão é descartada.
func soltar(conn *sql.Conn, empresaID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtext($1))`, "importacao_erp:"+empresaID); err != nil {
		log.Printf("importação erp: soltar lock: %v", err)
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	conn.Close()
}

// Executar roda o pedido e grava o resultado. Mês fechado é pulado; uma carga
// recusada não impede as demais; falha de leitura do ERP interrompe o resto.
func (i *ImportacaoERP) Executar(ctx context.Context, p *PedidoImportacao) (ex Execucao, err error) {
	defer soltar(p.conn, p.e.EmpresaID)
	ex = Execucao{ID: p.ID, Itens: []ItemImportacao{}}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("importação erp %d: pânico: %v", p.ID, r)
			ex.Status, ex.Mensagem = ExecucaoFalha, "falha interna na importação"
			err = i.concluir(p, ex)
		}
	}()

	falhou := ""
laco:
	for _, comp := range p.meses {
		for _, f := range i.Fontes(comp) {
			it := ItemImportacao{Competencia: comp.Format("2006-01"), Status: ItemAplicado}
			res, e := i.Ingestor.Importar(ctx, p.e.EmpresaID, p.e.UserID, f)
			it.Origem, it.Linhas = f.Nome(), res.Linhas
			if e != nil {
				it.Status, it.Mensagem = classificarFalha(p.ID, e)
			}
			ex.Itens = append(ex.Itens, it)
			if it.Status == ItemFalha {
				falhou = it.Mensagem
				break laco
			}
		}
	}
	ex.Status, ex.Mensagem = resumirExecucao(ex.Itens, falhou)
	return ex, i.concluir(p, ex)
}

func classificarFalha(id int64, err error) (string, string) {
	var fechado *ErrPeriodoFechado
	var entrada *ErroEntrada
	var linhas *ErroLinhas
	var dona *ErrContaDeOutraFonte
	var outra *ErrDadosDeOutraOrigem
	var fonte *ErroFonte
	switch {
	case errors.As(err, &fechado):
		return ItemFechado, fechado.Error()
	case errors.As(err, &entrada):
		return ItemSemDados, "o banco do cliente não devolveu linhas; os dados anteriores foram mantidos"
	case errors.As(err, &linhas), errors.As(err, &dona), errors.As(err, &outra):
		return ItemRejeitado, err.Error()
	case errors.As(err, &fonte):
		return ItemFalha, fonte.Msg
	}
	log.Printf("importação erp %d: %v", id, err)
	return ItemFalha, "falha interna na importação"
}

func resumirExecucao(itens []ItemImportacao, falhou string) (string, string) {
	n := map[string]int{}
	linhas := 0
	for _, it := range itens {
		n[it.Status]++
		linhas += it.Linhas
	}
	msg := fmt.Sprintf("%d aplicada(s) (%d linhas), %d pulada(s) por mês fechado, %d rejeitada(s), %d sem dados",
		n[ItemAplicado], linhas, n[ItemFechado], n[ItemRejeitado], n[ItemSemDados])
	switch {
	case falhou != "":
		return ExecucaoFalha, falhou + ". " + msg
	case n[ItemAplicado] == len(itens):
		return ExecucaoConcluida, msg
	}
	return ExecucaoComRessalvas, msg
}

// concluir grava o resultado mesmo que o contexto do pedido tenha sido cancelado.
func (i *ImportacaoERP) concluir(p *PedidoImportacao, ex Execucao) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	j, err := json.Marshal(ex.Itens)
	if err != nil {
		return err
	}
	tx, err := i.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE auditoria.execucoes SET fim=now(), status=$2, mensagem=$3, resumo=$4 WHERE id=$1`,
		p.ID, ex.Status, ex.Mensagem, j); err != nil {
		return err
	}
	if err := trilha.Gravar(tx, p.e.EmpresaID, p.e.UserID, "importacao_erp", fmt.Sprint(p.ID), "concluir",
		map[string]any{"status": ex.Status, "itens": ex.Itens}); err != nil {
		return err
	}
	return tx.Commit()
}

// Execucoes lista as últimas importações da empresa.
func (i *ImportacaoERP) Execucoes(ctx context.Context, empresaID string, limite int) ([]Execucao, error) {
	return ListarExecucoes(ctx, i.DB, empresaID, limite)
}

// ListarExecucoes lê auditoria.execucoes, da mais recente para a mais antiga.
func ListarExecucoes(ctx context.Context, q Consultador, empresaID string, limite int) ([]Execucao, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, gatilho, to_char(de,'YYYY-MM'), to_char(ate,'YYYY-MM'), inicio, fim, status, mensagem, resumo
		FROM auditoria.execucoes WHERE empresa_id=$1::uuid ORDER BY id DESC LIMIT $2`, empresaID, limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Execucao{}
	for rows.Next() {
		var e Execucao
		var fim sql.NullTime
		var j []byte
		if err := rows.Scan(&e.ID, &e.Gatilho, &e.De, &e.Ate, &e.Inicio, &fim, &e.Status, &e.Mensagem, &j); err != nil {
			return nil, err
		}
		if fim.Valid {
			e.Fim = &fim.Time
		}
		if err := json.Unmarshal(j, &e.Itens); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Nome e Atualizar fazem da importação uma Atualizavel (botão "Atualizar
// dados", história 2.16): relê o mês corrente e o anterior.
func (i *ImportacaoERP) Nome() string { return "banco do cliente (Oracle)" }

func (i *ImportacaoERP) Atualizar(ctx context.Context, e EscopoAtualizacao) (Resultado, error) {
	if e.EmpresaID != i.EmpresaID {
		return Resultado{}, &ErroEntrada{Msg: "o banco do cliente está configurado para outra empresa"}
	}
	h := i.Ingestor.agora()
	atual := time.Date(h.Year(), h.Month(), 1, 0, 0, 0, 0, time.UTC)
	p, err := i.Iniciar(ctx, e, "atualizar", []time.Time{atual.AddDate(0, -1, 0), atual})
	if errors.Is(err, ErrImportacaoEmAndamento) {
		return Resultado{}, &ErroEntrada{Msg: err.Error()}
	}
	if err != nil {
		return Resultado{}, err
	}
	ex, err := i.Executar(context.WithoutCancel(ctx), p)
	if err != nil {
		return Resultado{}, err
	}
	var res Resultado
	var fechadas []EscopoFechado
	var probs []string
	for _, it := range ex.Itens {
		switch it.Status {
		case ItemAplicado:
			res.Linhas += it.Linhas
		case ItemFechado:
			fechadas = append(fechadas, EscopoFechado{Competencia: it.Competencia})
		case ItemRejeitado, ItemFalha:
			probs = append(probs, it.Competencia+" "+it.Origem+": "+it.Mensagem)
		}
	}
	switch {
	case len(probs) > 0:
		return Resultado{}, &ErroEntrada{Msg: ex.Mensagem + "\n" + strings.Join(probs, "\n")}
	case res.Linhas > 0:
		return res, nil
	case len(fechadas) > 0:
		return Resultado{}, &ErrPeriodoFechado{Escopos: fechadas}
	}
	return Resultado{}, &ErroEntrada{Msg: "o banco do cliente não devolveu linhas"}
}
