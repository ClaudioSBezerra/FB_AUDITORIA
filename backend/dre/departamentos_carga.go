package dre

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"fb_auditoria/trilha"
)

// DepartamentoArquivo é uma linha do arquivo de departamentos.
type DepartamentoArquivo struct {
	Linha  int
	Codigo string
	Nome   string
	Grupo  string
}

// CargaDepartamentosIn é a entrada de CarregarDepartamentos. Problemas traz o que
// a leitura do arquivo já recusou; nada é gravado se houver qualquer problema.
type CargaDepartamentosIn struct {
	EmpresaID string
	UserID    string
	Arquivo   string
	Linhas    []DepartamentoArquivo
	Problemas []Problema
}

// CargaDepartamentosOut resume a carga.
type CargaDepartamentosOut struct {
	Linhas      int `json:"linhas"`
	Criados     int `json:"criados"`
	Atualizados int `json:"atualizados"`
	Inalterados int `json:"inalterados"`
}

// CarregarDepartamentos grava os departamentos do arquivo, por código: cria os
// novos e atualiza nome e grupo dos existentes. Não apaga quem não veio (pode ter
// conta ou usuário ligado). Tudo ou nada, numa transação com advisory lock por
// empresa; a trilha (só contagens e códigos) vai na mesma transação.
func (d *Dicionario) CarregarDepartamentos(ctx context.Context, in CargaDepartamentosIn) (CargaDepartamentosOut, error) {
	var out CargaDepartamentosOut
	if len(in.Linhas) == 0 && len(in.Problemas) == 0 {
		return out, &ErroEntrada{Msg: "o arquivo não tem departamentos"}
	}
	probs := append([]Problema(nil), in.Problemas...)
	vista := map[string]int{}
	for _, l := range in.Linhas {
		if prim, ok := vista[l.Codigo]; ok {
			probs = append(probs, Problema{Linha: l.Linha, Msg: fmt.Sprintf("codigo repetido %s (já na linha %d)", l.Codigo, prim)})
			continue
		}
		vista[l.Codigo] = l.Linha
	}
	if len(probs) > 0 {
		sort.SliceStable(probs, func(i, j int) bool { return probs[i].Linha < probs[j].Linha })
		return out, &ErroLinhas{Problemas: probs}
	}

	tx, err := d.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('departamentos:' || $1))`, in.EmpresaID); err != nil {
		return out, err
	}
	atuais, err := listarDepartamentos(ctx, tx, in.EmpresaID, "")
	if err != nil {
		return out, err
	}
	porCodigo := map[string]Departamento{}
	for _, x := range atuais {
		porCodigo[x.Codigo] = x
	}
	var criados, atualizados []string
	for _, l := range in.Linhas {
		ant, existe := porCodigo[l.Codigo]
		switch {
		case !existe:
			if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.departamentos (empresa_id, codigo, nome, grupo)
				VALUES ($1::uuid,$2,$3,$4)`, in.EmpresaID, l.Codigo, l.Nome, l.Grupo); err != nil {
				return out, err
			}
			criados = append(criados, l.Codigo)
		case ant.Nome != l.Nome || ant.Grupo != l.Grupo:
			if _, err := tx.ExecContext(ctx, `UPDATE auditoria.departamentos SET nome=$3, grupo=$4
				WHERE empresa_id=$1::uuid AND id=$2::uuid`, in.EmpresaID, ant.ID, l.Nome, l.Grupo); err != nil {
				return out, err
			}
			atualizados = append(atualizados, l.Codigo)
		default:
			out.Inalterados++
		}
	}
	out.Linhas, out.Criados, out.Atualizados = len(in.Linhas), len(criados), len(atualizados)
	// Só contagens e códigos: nomes ficam fora da trilha.
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "departamento", "carga", "carga", map[string]any{
		"arquivo": in.Arquivo, "linhas": out.Linhas, "criados": out.Criados, "atualizados": out.Atualizados,
		"inalterados": out.Inalterados, "codigos_criados": criados, "codigos_atualizados": atualizados,
	}); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// departamentosInexistentes junta, de forma agregada, os códigos de departamento
// do arquivo do dicionário que não existem: "departamentos inexistentes: A, B (N linhas)".
// Devolve a primeira linha em que algum aparece; vazio se todos existem.
func departamentosInexistentes(linhas []ContaDic, existe func(string) bool) (Problema, bool) {
	var codigos []string
	vistos := map[string]bool{}
	n, primeira := 0, 0
	for _, l := range linhas {
		if l.Departamento == "" || existe(l.Departamento) {
			continue
		}
		if n == 0 {
			primeira = l.Linha
		}
		n++
		if !vistos[l.Departamento] {
			vistos[l.Departamento] = true
			codigos = append(codigos, l.Departamento)
		}
	}
	if n == 0 {
		return Problema{}, false
	}
	lista := codigos
	extra := ""
	if len(lista) > maxProblemasListados {
		extra = fmt.Sprintf(" e mais %d", len(lista)-maxProblemasListados)
		lista = lista[:maxProblemasListados]
	}
	rotulo := "linhas"
	if n == 1 {
		rotulo = "linha"
	}
	prefixo := "departamentos inexistentes: "
	if len(codigos) == 1 {
		prefixo = "departamento inexistente: "
	}
	return Problema{Linha: primeira, Msg: fmt.Sprintf("%s%s%s (%d %s)", prefixo, strings.Join(lista, ", "), extra, n, rotulo)}, true
}
