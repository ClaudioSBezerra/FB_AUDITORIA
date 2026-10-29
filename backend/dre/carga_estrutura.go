package dre

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"fb_auditoria/trilha"

	"github.com/lib/pq"
)

// História 3.2: carga da estrutura do DRE e do Mapa por arquivo. Tudo ou nada,
// em uma transação com advisory lock por empresa, com a trilha no mesmo commit.
// Mês fechado não muda: lê do snapshot (1.8).

// maxCodigosTrilha limita os códigos listados no payload da trilha.
const maxCodigosTrilha = 200

// LinhaEstrutura é uma linha do arquivo de estrutura.
type LinhaEstrutura struct {
	Linha     int // linha do arquivo, para as mensagens
	Codigo    string
	Descricao string
	Nivel     int
	Ordem     int
	Tipo      string // conta | subtotal
	SomaDe    []string
	Papel     string
}

// ItemMapaArquivo é uma linha do arquivo de Mapa.
type ItemMapaArquivo struct {
	Linha       int
	ContaPadrao string
	LinhaCodigo string
	Sinal       int
}

// CargaEstruturaIn é a entrada de CarregarEstrutura. Problemas traz o que a
// leitura do arquivo já recusou; nada é gravado se houver qualquer problema.
type CargaEstruturaIn struct {
	EmpresaID string
	UserID    string
	Arquivo   string
	Linhas    []LinhaEstrutura
	Problemas []Problema
}

// CargaEstruturaOut resume a carga de estrutura.
type CargaEstruturaOut struct {
	Linhas            int      `json:"linhas"`
	Subtotais         int      `json:"subtotais"`
	Novas             int      `json:"novas"`
	Atualizadas       int      `json:"atualizadas"`
	Desativadas       int      `json:"desativadas"`
	CodigosDesativado []string `json:"codigos_desativados,omitempty"`
	PapelPreservado   bool     `json:"papel_preservado,omitempty"`
	MesesFechados     string   `json:"meses_fechados"`
}

// CargaMapaIn é a entrada de CarregarMapaArquivo.
type CargaMapaIn struct {
	EmpresaID string
	UserID    string
	Arquivo   string
	Itens     []ItemMapaArquivo
	Problemas []Problema
}

// CargaMapaOut resume a carga do Mapa.
type CargaMapaOut struct {
	Linhas        int    `json:"linhas"`
	Inseridas     int    `json:"inseridas"`
	Atualizadas   int    `json:"atualizadas"`
	Removidas     int    `json:"removidas"`
	MesesFechados string `json:"meses_fechados"`
}

const avisoMesesFechados = "Meses fechados não mudam: o DRE deles vem do snapshot do fechamento."

// ErrEstruturaEmUso recusa a carga de estrutura que tiraria do DRE (ou deixaria
// de ser conta) linhas ainda ligadas a contas do Mapa.
type ErrEstruturaEmUso struct {
	Linhas []string // "codigo (N conta(s))", até maxContasListadas
	Total  int
}

func (e *ErrEstruturaEmUso) Error() string {
	s := fmt.Sprintf("%d linha(s) sairiam da estrutura (ou deixariam de ser conta) mas ainda têm contas no Mapa; nada foi gravado: %s",
		e.Total, strings.Join(e.Linhas, "; "))
	if e.Total > len(e.Linhas) {
		s += fmt.Sprintf("; ... e mais %d. Recarregue o Mapa sem essas contas ou mantenha as linhas no arquivo.", e.Total-len(e.Linhas))
	} else {
		s += ". Recarregue o Mapa sem essas contas ou mantenha as linhas no arquivo."
	}
	return s
}

// EstruturaCarga é o serviço de carga da estrutura e do Mapa; é o único que
// substitui a estrutura e o Mapa em lote.
type EstruturaCarga struct{ DB *sql.DB }

func travarEstrutura(ctx context.Context, tx *sql.Tx, empresaID string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('estrutura-mapa:' || $1))`, empresaID)
	return err
}

func ordenaProblemas(p []Problema) {
	sort.SliceStable(p, func(i, j int) bool { return p[i].Linha < p[j].Linha })
}

// ValidarEstrutura confere o arquivo sem banco: código único, tipo válido,
// subtotal com soma_de de códigos existentes no arquivo e sem ciclo, no máximo
// uma linha com papel (só faturamento_liquido). soma_de aceita "-COD" (subtrai).
func ValidarEstrutura(linhas []LinhaEstrutura) []Problema {
	var probs []Problema
	add := func(l int, m string) { probs = append(probs, Problema{Linha: l, Msg: m}) }
	por := map[string]LinhaEstrutura{}
	for _, l := range linhas {
		if prim, ok := por[l.Codigo]; ok {
			add(l.Linha, fmt.Sprintf("codigo repetido %s (já na linha %d)", l.Codigo, prim.Linha))
			continue
		}
		por[l.Codigo] = l
	}
	papeis := 0
	for _, l := range linhas {
		if l.Tipo != "conta" && l.Tipo != "subtotal" {
			add(l.Linha, "tipo deve ser conta ou subtotal")
			continue
		}
		if l.Tipo == "conta" && len(l.SomaDe) > 0 {
			add(l.Linha, "linha do tipo conta não tem soma_de")
		}
		if l.Tipo == "subtotal" {
			if len(l.SomaDe) == 0 {
				add(l.Linha, "subtotal sem soma_de")
			}
			for _, ref := range l.SomaDe {
				ref = strings.TrimPrefix(ref, "-")
				if _, ok := por[ref]; !ok {
					add(l.Linha, "soma_de referencia código que não existe no arquivo: "+ref)
				}
			}
		}
		if l.Papel != "" {
			if l.Papel != PapelFaturamentoLiquido {
				add(l.Linha, "papel deve ser vazio ou faturamento_liquido")
			}
			papeis++
			if papeis > 1 {
				add(l.Linha, "no máximo uma linha pode ter papel")
			}
		}
	}
	// Ciclos: busca em profundidade nos subtotais.
	estado := map[string]int{} // 1 visitando, 2 pronto
	reportados := map[string]bool{}
	var visita func(cod string)
	visita = func(cod string) {
		l, ok := por[cod]
		if !ok || l.Tipo != "subtotal" {
			return
		}
		estado[cod] = 1
		for _, ref := range l.SomaDe {
			ref = strings.TrimPrefix(ref, "-")
			switch estado[ref] {
			case 1:
				if !reportados[ref] {
					reportados[ref] = true
					add(por[ref].Linha, "ciclo nos subtotais: "+ref+" soma a si mesmo, direta ou indiretamente")
				}
			case 0:
				visita(ref)
			}
		}
		estado[cod] = 2
	}
	for _, l := range linhas {
		if estado[l.Codigo] == 0 {
			visita(l.Codigo)
		}
	}
	ordenaProblemas(probs)
	return probs
}

func limitaCodigos(c []string) []string {
	sort.Strings(c)
	if len(c) > maxCodigosTrilha {
		return c[:maxCodigosTrilha]
	}
	return c
}

// CarregarEstrutura substitui a estrutura da empresa pelo arquivo: upsert pelo
// código (atualiza descrição, nível, ordem, tipo, soma_de e papel; insere as
// novas) e marca ativo=false as linhas que não vieram, sem apagá-las (histórico
// e snapshots dependem delas). Recusa (ErrEstruturaEmUso) se uma linha que sai,
// ou que deixa de ser conta, ainda tem conta no Mapa. Papel: se o arquivo traz
// um, ele vale; se não traz nenhum, o papel já marcado na tela fica nas linhas
// que permanecem.
func (s *EstruturaCarga) CarregarEstrutura(ctx context.Context, in CargaEstruturaIn) (CargaEstruturaOut, error) {
	var out CargaEstruturaOut
	if len(in.Linhas) == 0 && len(in.Problemas) == 0 {
		return out, &ErroEntrada{Msg: "o arquivo não tem linhas de estrutura"}
	}
	probs := append(append([]Problema(nil), in.Problemas...), ValidarEstrutura(in.Linhas)...)
	if len(probs) > 0 {
		ordenaProblemas(probs)
		return out, &ErroLinhas{Problemas: probs}
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err := travarEstrutura(ctx, tx, in.EmpresaID); err != nil {
		return out, err
	}

	// Estado anterior: linhas ativas e contas do Mapa por linha.
	antes := map[string]string{} // codigo -> tipo (ativas)
	rows, err := tx.QueryContext(ctx, `SELECT codigo, tipo FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND ativo`, in.EmpresaID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var c, t string
		if err := rows.Scan(&c, &t); err != nil {
			rows.Close()
			return out, err
		}
		antes[c] = t
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	noMapa := map[string]int{}
	rows, err = tx.QueryContext(ctx, `SELECT linha_codigo, COUNT(*) FROM auditoria.dre_mapa WHERE empresa_id=$1::uuid GROUP BY linha_codigo`, in.EmpresaID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var c string
		var n int
		if err := rows.Scan(&c, &n); err != nil {
			rows.Close()
			return out, err
		}
		noMapa[c] = n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}

	novos := map[string]LinhaEstrutura{}
	temPapel := false
	for _, l := range in.Linhas {
		novos[l.Codigo] = l
		if l.Papel != "" {
			temPapel = true
		}
	}
	var emUso []string
	var desativar []string
	for c := range antes {
		if _, ok := novos[c]; !ok {
			desativar = append(desativar, c)
		}
	}
	for c, n := range noMapa {
		l, vem := novos[c]
		_, existia := antes[c]
		if !existia {
			continue // Mapa apontando para linha já inativa/inexistente: não é efeito desta carga
		}
		if !vem || l.Tipo != "conta" {
			emUso = append(emUso, fmt.Sprintf("%s (%d conta(s))", c, n))
		}
	}
	if len(emUso) > 0 {
		sort.Strings(emUso)
		total := len(emUso)
		if total > maxContasListadas {
			emUso = emUso[:maxContasListadas]
		}
		return out, &ErrEstruturaEmUso{Linhas: emUso, Total: total}
	}

	// Papel: o do arquivo substitui; sem papel no arquivo, o da tela fica nas que permanecem.
	// A linha que sai nunca guarda papel (o índice único é por empresa, inclusive inativas).
	if temPapel {
		if _, err := tx.ExecContext(ctx, `UPDATE auditoria.dre_linhas SET papel=NULL WHERE empresa_id=$1::uuid AND papel IS NOT NULL`, in.EmpresaID); err != nil {
			return out, err
		}
	} else if len(desativar) > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE auditoria.dre_linhas SET papel=NULL WHERE empresa_id=$1::uuid AND papel IS NOT NULL AND codigo = ANY($2)`,
			in.EmpresaID, pq.Array(desativar)); err != nil {
			return out, err
		}
	}
	var alterados []string
	for _, l := range in.Linhas {
		soma := l.SomaDe
		if soma == nil {
			soma = []string{} // soma_de é NOT NULL
		}
		var nova bool
		if err := tx.QueryRowContext(ctx, `INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo, soma_de, ativo, papel)
			VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,TRUE,NULLIF($8,''))
			ON CONFLICT (empresa_id, codigo) DO UPDATE SET descricao=EXCLUDED.descricao, nivel=EXCLUDED.nivel, ordem=EXCLUDED.ordem,
				tipo=EXCLUDED.tipo, soma_de=EXCLUDED.soma_de, ativo=TRUE,
				papel = CASE WHEN $9 THEN EXCLUDED.papel ELSE auditoria.dre_linhas.papel END
			RETURNING (xmax = 0)`,
			in.EmpresaID, l.Codigo, l.Descricao, l.Nivel, l.Ordem, l.Tipo, pq.Array(soma), l.Papel, temPapel).Scan(&nova); err != nil {
			return out, err
		}
		if nova {
			out.Novas++
		} else {
			out.Atualizadas++
		}
		if l.Tipo == "subtotal" {
			out.Subtotais++
		}
		alterados = append(alterados, l.Codigo)
	}
	if len(desativar) > 0 {
		res, err := tx.ExecContext(ctx, `UPDATE auditoria.dre_linhas SET ativo=FALSE WHERE empresa_id=$1::uuid AND ativo AND codigo = ANY($2)`,
			in.EmpresaID, pq.Array(desativar))
		if err != nil {
			return out, err
		}
		n, _ := res.RowsAffected()
		out.Desativadas = int(n)
		sort.Strings(desativar)
		out.CodigosDesativado = desativar
		if len(out.CodigosDesativado) > maxCodigosTrilha {
			out.CodigosDesativado = out.CodigosDesativado[:maxCodigosTrilha]
		}
	}
	out.Linhas = len(in.Linhas)
	out.PapelPreservado = !temPapel
	out.MesesFechados = avisoMesesFechados
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "dre_estrutura", "carga", "carregar", map[string]any{
		"arquivo": in.Arquivo,
		"antes":   map[string]int{"linhas_ativas": len(antes)},
		"depois":  map[string]int{"linhas_ativas": len(in.Linhas), "subtotais": out.Subtotais},
		"novas":   out.Novas, "atualizadas": out.Atualizadas, "desativadas": out.Desativadas,
		"codigos_carregados": limitaCodigos(alterados), "codigos_desativados": out.CodigosDesativado,
		"meses_fechados": avisoMesesFechados,
	}); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// CarregarMapaArquivo substitui o Mapa da empresa pelo arquivo: upsert pela
// conta e remoção do que não veio, na mesma transação. A linha_codigo tem de
// existir, ativa e do tipo conta. Conta repetida no arquivo é erro.
func (s *EstruturaCarga) CarregarMapaArquivo(ctx context.Context, in CargaMapaIn) (CargaMapaOut, error) {
	var out CargaMapaOut
	if len(in.Itens) == 0 && len(in.Problemas) == 0 {
		return out, &ErroEntrada{Msg: "o arquivo não tem contas no Mapa"}
	}
	probs := append([]Problema(nil), in.Problemas...)
	vista := map[string]int{}
	for _, it := range in.Itens {
		if prim, ok := vista[it.ContaPadrao]; ok {
			probs = append(probs, Problema{Linha: it.Linha, Msg: fmt.Sprintf("conta_padrao repetida %s (já na linha %d)", it.ContaPadrao, prim)})
			continue
		}
		vista[it.ContaPadrao] = it.Linha
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err := travarEstrutura(ctx, tx, in.EmpresaID); err != nil {
		return out, err
	}
	validas := map[string]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT codigo FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND ativo AND tipo='conta'`, in.EmpresaID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return out, err
		}
		validas[c] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	for _, it := range in.Itens {
		if !validas[it.LinhaCodigo] {
			probs = append(probs, Problema{Linha: it.Linha, Msg: "linha_codigo não existe na estrutura como linha de conta ativa: " + it.LinhaCodigo})
		}
	}
	if len(probs) > 0 {
		ordenaProblemas(probs)
		return out, &ErroLinhas{Problemas: probs}
	}

	existentes := map[string]MapaItem{}
	atuais, err := CarregarMapa(ctx, tx, in.EmpresaID)
	if err != nil {
		return out, err
	}
	for _, m := range atuais {
		existentes[m.ContaPadrao] = m
	}
	var ins []string
	for _, it := range in.Itens {
		if _, ok := existentes[it.ContaPadrao]; ok {
			out.Atualizadas++
		} else {
			out.Inseridas++
			ins = append(ins, it.ContaPadrao)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal)
			VALUES ($1::uuid,$2,$3,$4)
			ON CONFLICT (empresa_id, conta_padrao) DO UPDATE SET linha_codigo=EXCLUDED.linha_codigo, sinal=EXCLUDED.sinal`,
			in.EmpresaID, it.ContaPadrao, it.LinhaCodigo, it.Sinal); err != nil {
			return out, err
		}
	}
	var rem []string
	for c := range existentes {
		if _, ok := vista[c]; !ok {
			rem = append(rem, c)
		}
	}
	if len(rem) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM auditoria.dre_mapa WHERE empresa_id=$1::uuid AND conta_padrao = ANY($2)`,
			in.EmpresaID, pq.Array(rem)); err != nil {
			return out, err
		}
	}
	out.Removidas = len(rem)
	out.Linhas = len(in.Itens)
	out.MesesFechados = avisoMesesFechados
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "dre_mapa", "carga", "carregar", map[string]any{
		"arquivo":   in.Arquivo,
		"antes":     map[string]int{"contas": len(existentes)},
		"depois":    map[string]int{"contas": len(in.Itens)},
		"inseridas": out.Inseridas, "atualizadas": out.Atualizadas, "removidas": out.Removidas,
		"contas_inseridas": limitaCodigos(ins), "contas_removidas": limitaCodigos(rem),
		"meses_fechados": avisoMesesFechados,
	}); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
