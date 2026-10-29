package dre

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"fb_auditoria/trilha"
)

// FilialMapaArquivo é uma linha do arquivo do Mapa de filiais: liga uma filial do
// ERP a uma filial gerencial. Nome é opcional.
type FilialMapaArquivo struct {
	Linha     int
	Gerencial string
	Nome      string
	FilialERP string
}

// MembroAgrupamentoArquivo é uma linha do arquivo de agrupamentos: uma filial
// gerencial membro de um agrupamento. Nome é opcional.
type MembroAgrupamentoArquivo struct {
	Linha  int
	Codigo string
	Nome   string
	Filial string
}

// CargaMapaFiliaisIn é a entrada de CarregarMapa. Problemas traz o que a leitura do
// arquivo já recusou; nada é gravado se houver qualquer problema.
type CargaMapaFiliaisIn struct {
	EmpresaID string
	UserID    string
	Arquivo   string
	Linhas    []FilialMapaArquivo
	Problemas []Problema
}

// MovimentoFilial registra a filial do ERP que trocou de filial gerencial.
type MovimentoFilial struct {
	Filial string `json:"filial"`
	De     string `json:"de"`
	Para   string `json:"para"`
}

// CargaMapaFiliaisOut resume a carga do Mapa de filiais.
type CargaMapaFiliaisOut struct {
	Linhas          int               `json:"linhas"`
	Criadas         int               `json:"criadas"`
	Atualizadas     int               `json:"atualizadas"`
	LigacoesNovas   int               `json:"ligacoes_novas"`
	LigacoesMovidas int               `json:"ligacoes_movidas"`
	Inalteradas     int               `json:"inalteradas"`
	Movidas         []MovimentoFilial `json:"movidas"`
}

// CargaAgrupamentosIn é a entrada de CarregarAgrupamentos.
type CargaAgrupamentosIn struct {
	EmpresaID string
	UserID    string
	Arquivo   string
	Linhas    []MembroAgrupamentoArquivo
	Problemas []Problema
}

// CargaAgrupamentosOut resume a carga dos agrupamentos.
type CargaAgrupamentosOut struct {
	Linhas             int `json:"linhas"`
	Criados            int `json:"criados"`
	Atualizados        int `json:"atualizados"`
	MembrosAdicionados int `json:"membros_adicionados"`
	Inalteradas        int `json:"inalteradas"`
}

func problemasOrdenados(probs []Problema) *ErroLinhas {
	sort.SliceStable(probs, func(i, j int) bool { return probs[i].Linha < probs[j].Linha })
	return &ErroLinhas{Problemas: probs}
}

// nomePadraoFilial é o nome gravado quando o arquivo não traz nome.
func nomePadraoFilial(codigo string) string { return "Filial " + codigo }

// CarregarMapaFiliais grava o Mapa de filiais do arquivo: cria as filiais gerenciais que
// faltam, liga as filiais do ERP que ainda não estão ligadas e, se uma filial do
// ERP já pertence a outra filial gerencial, a move (informada em Movidas). Nunca
// apaga filial gerencial nem ligação que o arquivo não cite. Tudo ou nada, numa
// transação com advisory lock por empresa; a trilha vai na mesma transação.
func (f *Filiais) CarregarMapaFiliais(ctx context.Context, in CargaMapaFiliaisIn) (CargaMapaFiliaisOut, error) {
	out := CargaMapaFiliaisOut{Movidas: []MovimentoFilial{}}
	if len(in.Linhas) == 0 && len(in.Problemas) == 0 {
		return out, &ErroEntrada{Msg: "o arquivo não tem filiais"}
	}
	probs := append([]Problema(nil), in.Problemas...)

	// Conferência dentro do arquivo.
	type destino struct {
		gerencial string
		linha     int
	}
	vistaERP := map[string]destino{}
	nomeDe := map[string]destino{} // nome informado por filial gerencial
	nomes := map[string]string{}
	var ordem []string // filiais gerenciais, na ordem do arquivo
	var linhas []FilialMapaArquivo
	for _, l := range in.Linhas {
		bad := func(msg string) { probs = append(probs, Problema{Linha: l.Linha, Msg: msg}) }
		switch {
		case strings.EqualFold(l.Gerencial, CodigoReservadoGrupo):
			bad("o código " + l.Gerencial + " é a palavra reservada da visão de todas as filiais")
			continue
		case utf8.RuneCountInString(l.Gerencial) > maxCodigoFilial:
			bad("filial_gerencial: no máximo 40 caracteres")
			continue
		case utf8.RuneCountInString(l.Nome) > maxNomeFilial:
			bad("nome: no máximo 120 caracteres")
			continue
		case utf8.RuneCountInString(l.FilialERP) > maxFilialERP:
			bad("filial_erp: no máximo 20 caracteres")
			continue
		}
		if prim, ok := vistaERP[l.FilialERP]; ok {
			if prim.gerencial != l.Gerencial {
				bad(fmt.Sprintf("a filial %s já está na filial gerencial %s (linha %d)", l.FilialERP, prim.gerencial, prim.linha))
			}
			continue // par repetido: sem efeito
		}
		vistaERP[l.FilialERP] = destino{l.Gerencial, l.Linha}
		if l.Nome != "" {
			if prim, ok := nomeDe[l.Gerencial]; ok && nomes[l.Gerencial] != l.Nome {
				bad(fmt.Sprintf("nome diferente para a filial gerencial %s (linha %d)", l.Gerencial, prim.linha))
				continue
			} else if !ok {
				nomeDe[l.Gerencial] = destino{l.Gerencial, l.Linha}
				nomes[l.Gerencial] = l.Nome
			}
		}
		if _, ok := indiceDe(ordem, l.Gerencial); !ok {
			ordem = append(ordem, l.Gerencial)
		}
		linhas = append(linhas, l)
	}
	if len(probs) > 0 {
		return out, problemasOrdenados(probs)
	}

	tx, err := f.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('filiais:' || $1))`, in.EmpresaID); err != nil {
		return out, err
	}
	atuais, err := listarFiliais(ctx, tx, in.EmpresaID, "")
	if err != nil {
		return out, err
	}
	porCodigo := map[string]FilialGerencial{} // código normalizado -> filial gerencial gravada
	donoERP := map[string]string{}            // filial do ERP -> código gravado da filial gerencial
	for _, g := range atuais {
		if _, ok := porCodigo[NormFilialConferencia(g.Codigo)]; !ok {
			porCodigo[NormFilialConferencia(g.Codigo)] = g
		}
		for _, e := range g.Filiais {
			donoERP[e] = g.Codigo
		}
	}
	agrups, err := listarAgrupamentos(ctx, tx, in.EmpresaID, "")
	if err != nil {
		return out, err
	}
	codigosAgrup := map[string]bool{}
	for _, a := range agrups {
		codigosAgrup[a.Codigo] = true
	}

	// Código final de cada filial gerencial do arquivo (o já gravado, se existir).
	codFinal := map[string]string{}
	for _, c := range ordem {
		if g, ok := porCodigo[NormFilialConferencia(c)]; ok {
			codFinal[c] = g.Codigo
		} else {
			codFinal[c] = c
		}
		if codigosAgrup[codFinal[c]] {
			return out, problemasOrdenados([]Problema{{Linha: destinoLinha(linhas, c),
				Msg: "o código " + codFinal[c] + " já é o código de um agrupamento"}})
		}
	}

	// Estado final das ligações, para conferir a regra "código igual a filial do
	// ERP de outra filial gerencial" (a mesma do cadastro manual).
	final := map[string]string{}
	for e, g := range donoERP {
		final[e] = g
	}
	for _, l := range linhas {
		final[l.FilialERP] = codFinal[l.Gerencial]
	}
	codigos := map[string]int{}
	for _, g := range atuais {
		codigos[g.Codigo] = 0
	}
	for _, c := range ordem {
		codigos[codFinal[c]] = destinoLinha(linhas, c)
	}
	for c, linha := range codigos {
		if dono, ok := final[c]; ok && dono != c && (donoERP[c] != dono || linha > 0) {
			probs = append(probs, Problema{Linha: linha,
				Msg: "o código " + c + " é igual a uma filial do ERP ligada à filial gerencial " + dono})
		}
	}
	if len(probs) > 0 {
		return out, problemasOrdenados(probs)
	}

	// Aplicação.
	ids := map[string]string{} // código final -> id
	for _, g := range atuais {
		ids[g.Codigo] = g.ID
	}
	var criadas, atualizadas []string
	for _, c := range ordem {
		cod := codFinal[c]
		nome := nomes[c]
		if g, ok := porCodigo[NormFilialConferencia(c)]; ok {
			if nome != "" && nome != g.Nome {
				if _, err := tx.ExecContext(ctx, `UPDATE auditoria.filiais_gerenciais SET nome=$3
					WHERE empresa_id=$1::uuid AND id=$2::uuid`, in.EmpresaID, g.ID, nome); err != nil {
					return out, err
				}
				atualizadas = append(atualizadas, cod)
			}
			continue
		}
		if nome == "" {
			nome = nomePadraoFilial(cod)
		}
		var id string
		if err := tx.QueryRowContext(ctx, `INSERT INTO auditoria.filiais_gerenciais (empresa_id, codigo, nome)
			VALUES ($1::uuid,$2,$3) RETURNING id::text`, in.EmpresaID, cod, nome).Scan(&id); err != nil {
			return out, err
		}
		ids[cod] = id
		criadas = append(criadas, cod)
	}
	for _, l := range linhas {
		cod := codFinal[l.Gerencial]
		dono, ligada := donoERP[l.FilialERP]
		switch {
		case !ligada:
			if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.filiais_gerenciais_erp (empresa_id, filial_gerencial_id, filial_erp)
				VALUES ($1::uuid,$2::uuid,$3)`, in.EmpresaID, ids[cod], l.FilialERP); err != nil {
				return out, err
			}
			out.LigacoesNovas++
		case dono != cod:
			if _, err := tx.ExecContext(ctx, `UPDATE auditoria.filiais_gerenciais_erp SET filial_gerencial_id=$3::uuid
				WHERE empresa_id=$1::uuid AND filial_erp=$2`, in.EmpresaID, l.FilialERP, ids[cod]); err != nil {
				return out, err
			}
			out.LigacoesMovidas++
			out.Movidas = append(out.Movidas, MovimentoFilial{Filial: l.FilialERP, De: dono, Para: cod})
		default:
			out.Inalteradas++
		}
	}
	out.Linhas, out.Criadas, out.Atualizadas = len(linhas), len(criadas), len(atualizadas)
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "filial_gerencial", "carga", "carga", map[string]any{
		"arquivo": in.Arquivo, "linhas": out.Linhas, "criadas": criadas, "atualizadas": atualizadas,
		"ligacoes_novas": out.LigacoesNovas, "ligacoes_movidas": out.LigacoesMovidas,
		"inalteradas": out.Inalteradas, "movidas": out.Movidas,
	}); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// CarregarAgrupamentos grava os agrupamentos do arquivo: cria os que faltam e
// acrescenta os membros que ainda não estão. Os membros são filiais gerenciais
// que já existem (o Mapa de filiais vem antes). Nunca apaga agrupamento nem
// membro que o arquivo não cite. Tudo ou nada.
func (a *Agrupamentos) CarregarAgrupamentos(ctx context.Context, in CargaAgrupamentosIn) (CargaAgrupamentosOut, error) {
	var out CargaAgrupamentosOut
	if len(in.Linhas) == 0 && len(in.Problemas) == 0 {
		return out, &ErroEntrada{Msg: "o arquivo não tem agrupamentos"}
	}
	probs := append([]Problema(nil), in.Problemas...)

	type par struct{ codigo, filial string }
	vistos := map[par]bool{}
	nomes := map[string]string{}
	nomeLinha := map[string]int{}
	var ordem []string
	var linhas []MembroAgrupamentoArquivo
	for _, l := range in.Linhas {
		bad := func(msg string) { probs = append(probs, Problema{Linha: l.Linha, Msg: msg}) }
		switch {
		case strings.EqualFold(l.Codigo, CodigoReservadoGrupo):
			bad("o código " + l.Codigo + " é a palavra reservada da visão de todas as filiais")
			continue
		case utf8.RuneCountInString(l.Codigo) > maxCodigoFilial:
			bad("agrupamento: no máximo 40 caracteres")
			continue
		case utf8.RuneCountInString(l.Nome) > maxNomeFilial:
			bad("nome: no máximo 120 caracteres")
			continue
		case utf8.RuneCountInString(l.Filial) > maxCodigoFilial:
			bad("filial: no máximo 40 caracteres")
			continue
		}
		if l.Nome != "" {
			if n, ok := nomes[l.Codigo]; ok && n != l.Nome {
				bad(fmt.Sprintf("nome diferente para o agrupamento %s (linha %d)", l.Codigo, nomeLinha[l.Codigo]))
				continue
			} else if !ok {
				nomes[l.Codigo], nomeLinha[l.Codigo] = l.Nome, l.Linha
			}
		}
		if _, ok := indiceDe(ordem, l.Codigo); !ok {
			ordem = append(ordem, l.Codigo)
		}
		k := par{l.Codigo, l.Filial}
		if vistos[k] {
			continue // par repetido: sem efeito
		}
		vistos[k] = true
		linhas = append(linhas, l)
	}
	if len(probs) > 0 {
		return out, problemasOrdenados(probs)
	}

	tx, err := a.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('filiais:' || $1))`, in.EmpresaID); err != nil {
		return out, err
	}
	gerenciais, err := listarFiliais(ctx, tx, in.EmpresaID, "")
	if err != nil {
		return out, err
	}
	gid := map[string]string{}   // código gravado -> id
	gnorm := map[string]string{} // código normalizado -> código gravado
	for _, g := range gerenciais {
		gid[g.Codigo] = g.ID
		if _, ok := gnorm[NormFilialConferencia(g.Codigo)]; !ok {
			gnorm[NormFilialConferencia(g.Codigo)] = g.Codigo
		}
	}
	atuais, err := listarAgrupamentos(ctx, tx, in.EmpresaID, "")
	if err != nil {
		return out, err
	}
	porCodigo := map[string]Agrupamento{}
	for _, ag := range atuais {
		porCodigo[ag.Codigo] = ag
	}

	// Membros que não existem como filial gerencial, agregados numa só mensagem.
	var faltam []string
	vistaFalta := map[string]bool{}
	nFalta, primeira := 0, 0
	for _, l := range linhas {
		if _, ok := gnorm[NormFilialConferencia(l.Filial)]; ok {
			continue
		}
		if nFalta == 0 {
			primeira = l.Linha
		}
		nFalta++
		if !vistaFalta[l.Filial] {
			vistaFalta[l.Filial] = true
			faltam = append(faltam, l.Filial)
		}
	}
	if nFalta > 0 {
		lista, extra := faltam, ""
		if len(lista) > maxProblemasListados {
			extra = fmt.Sprintf(" e mais %d", len(lista)-maxProblemasListados)
			lista = lista[:maxProblemasListados]
		}
		prefixo := "filiais gerenciais inexistentes: "
		if len(faltam) == 1 {
			prefixo = "filial gerencial inexistente: "
		}
		return out, problemasOrdenados([]Problema{{Linha: primeira, Msg: fmt.Sprintf(
			"%s%s%s (%d linha(s)); cadastre-as antes no Mapa de filiais", prefixo, strings.Join(lista, ", "), extra, nFalta)}})
	}
	// O código do agrupamento não pode ser o de uma filial gerencial.
	for _, c := range ordem {
		if _, ok := gid[c]; ok {
			return out, problemasOrdenados([]Problema{{Linha: nomeOuPrimeira(in.Linhas, c),
				Msg: "o código " + c + " já é o código de uma filial gerencial"}})
		}
	}

	var criados, atualizados []string
	idAgr := map[string]string{}
	for _, c := range ordem {
		nome := nomes[c]
		if ag, ok := porCodigo[c]; ok {
			idAgr[c] = ag.ID
			if nome != "" && nome != ag.Nome {
				if _, err := tx.ExecContext(ctx, `UPDATE auditoria.agrupamentos_filiais SET nome=$3
					WHERE empresa_id=$1::uuid AND id=$2::uuid`, in.EmpresaID, ag.ID, nome); err != nil {
					return out, err
				}
				atualizados = append(atualizados, c)
			}
			continue
		}
		if nome == "" {
			nome = c
		}
		var id string
		if err := tx.QueryRowContext(ctx, `INSERT INTO auditoria.agrupamentos_filiais (empresa_id, codigo, nome)
			VALUES ($1::uuid,$2,$3) RETURNING id::text`, in.EmpresaID, c, nome).Scan(&id); err != nil {
			return out, err
		}
		idAgr[c] = id
		criados = append(criados, c)
	}
	for _, l := range linhas {
		cod := gnorm[NormFilialConferencia(l.Filial)]
		if ag, ok := porCodigo[l.Codigo]; ok && contem(ag.Filiais, cod) {
			out.Inalteradas++
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.agrupamentos_filiais_membros
			(empresa_id, agrupamento_id, filial_gerencial_id) VALUES ($1::uuid,$2::uuid,$3::uuid)`,
			in.EmpresaID, idAgr[l.Codigo], gid[cod]); err != nil {
			return out, err
		}
		out.MembrosAdicionados++
	}
	out.Linhas, out.Criados, out.Atualizados = len(linhas), len(criados), len(atualizados)
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "agrupamento_filiais", "carga", "carga", map[string]any{
		"arquivo": in.Arquivo, "linhas": out.Linhas, "criados": criados, "atualizados": atualizados,
		"membros_adicionados": out.MembrosAdicionados, "inalteradas": out.Inalteradas,
	}); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func indiceDe(l []string, s string) (int, bool) {
	for i, x := range l {
		if x == s {
			return i, true
		}
	}
	return -1, false
}

func contem(l []string, s string) bool {
	_, ok := indiceDe(l, s)
	return ok
}

// destinoLinha devolve a primeira linha do arquivo da filial gerencial.
func destinoLinha(linhas []FilialMapaArquivo, gerencial string) int {
	for _, l := range linhas {
		if l.Gerencial == gerencial {
			return l.Linha
		}
	}
	return 0
}

func nomeOuPrimeira(linhas []MembroAgrupamentoArquivo, codigo string) int {
	for _, l := range linhas {
		if l.Codigo == codigo {
			return l.Linha
		}
	}
	return 0
}
