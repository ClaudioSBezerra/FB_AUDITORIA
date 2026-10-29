package dre

// ingest_resumo.go — história 2.19: resumo do que o arquivo traz (pares
// competência/filial com contagem) e avisos de plausibilidade. Os avisos nunca
// bloqueiam a carga; só a guarda de período (mês fechado) e a validação das
// linhas recusam.

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ParResumo é um par competência/filial presente no arquivo.
type ParResumo struct {
	Competencia string `json:"competencia"` // AAAA-MM
	Filial      string `json:"filial"`
	Lancamentos int    `json:"lancamentos"`
}

// Aviso é uma observação de plausibilidade; não bloqueia a gravação.
type Aviso struct {
	Codigo   string `json:"codigo"`
	Mensagem string `json:"mensagem"`
}

// Resumo é o que foi lido do arquivo. A simulação e a gravação devolvem o mesmo
// resumo; só a gravação traz o CargaID (em Resultado).
type Resumo struct {
	Origem  string      `json:"origem"`
	Arquivo string      `json:"arquivo"`
	Total   int         `json:"total"`
	Pares   []ParResumo `json:"pares"`
	Avisos  []Aviso     `json:"avisos"`
}

// Códigos dos avisos.
const (
	AvisoCompetenciaAntiga   = "competencia_antiga"
	AvisoCompetenciaFutura   = "competencia_futura"
	AvisoNomeDivergente      = "nome_do_arquivo"
	AvisoFilialNova          = "filial_nova"
	AvisoOutraOrigem         = "outra_origem"
	mesesPassadoPlausivel    = 12
	formatoCompetenciaResumo = "2006-01"
)

func (g *Ingestor) agora() time.Time {
	if g.Agora != nil {
		return g.Agora()
	}
	return time.Now()
}

// paresDe conta os lançamentos por (competência, filial), em ordem.
func paresDe(lancs []Lancamento) []ParResumo {
	idx := map[escopoImp]int{}
	var out []ParResumo
	for _, l := range lancs {
		k := escopoImp{l.Competencia.Format(formatoCompetenciaResumo), l.Filial}
		i, ok := idx[k]
		if !ok {
			i = len(out)
			idx[k] = i
			out = append(out, ParResumo{Competencia: k.Competencia, Filial: k.Filial})
		}
		out[i].Lancamentos++
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Competencia != out[j].Competencia {
			return out[i].Competencia < out[j].Competencia
		}
		return out[i].Filial < out[j].Filial
	})
	return out
}

// Padrões de mês no nome do arquivo: AAAAMM, MM-AAAA, MM_AAAA (com espaços em
// volta do separador) e MMAAAA. O ano é 20AA; os dígitos não podem encostar em
// outros dígitos.
var (
	reNomeAAAAMM = regexp.MustCompile(`(?:^|\D)(20\d{2})(0[1-9]|1[0-2])(?:\D|$)`)
	reNomeMMsepA = regexp.MustCompile(`(?:^|\D)(0[1-9]|1[0-2])\s*[-_]\s*(20\d{2})(?:\D|$)`)
	reNomeMMAAAA = regexp.MustCompile(`(?:^|\D)(0[1-9]|1[0-2])(20\d{2})(?:\D|$)`)
)

// competenciasDoNome devolve os meses (AAAA-MM) que o nome do arquivo sugere.
func competenciasDoNome(arquivo string) []string {
	nome := filepath.Base(strings.ReplaceAll(arquivo, `\`, "/"))
	nome = strings.TrimSuffix(nome, filepath.Ext(nome))
	vistos := map[string]bool{}
	var out []string
	add := func(a, m string) {
		k := a + "-" + m
		if !vistos[k] {
			vistos[k] = true
			out = append(out, k)
		}
	}
	// FindAll com (?:\D|$) consome o separador; sobreposição de matches é rara
	// o bastante num nome de arquivo para não valer um scanner próprio.
	for _, m := range reNomeAAAAMM.FindAllStringSubmatch(nome, -1) {
		add(m[1], m[2])
	}
	for _, m := range reNomeMMsepA.FindAllStringSubmatch(nome, -1) {
		add(m[2], m[1])
	}
	for _, m := range reNomeMMAAAA.FindAllStringSubmatch(nome, -1) {
		add(m[2], m[1])
	}
	return out
}

// montarResumo calcula o resumo e os avisos. q é a transação (ou o banco) do
// chamador; a consulta é só de leitura.
func (g *Ingestor) montarResumo(ctx context.Context, q Consultador, empresaID, origem, arquivo string, lancs []Lancamento) (Resumo, error) {
	res := Resumo{Origem: origem, Arquivo: arquivo, Total: len(lancs), Pares: paresDe(lancs), Avisos: []Aviso{}}

	comps := []string{}
	vistaComp := map[string]bool{}
	filiais := []string{}
	vistaFil := map[string]bool{}
	for _, p := range res.Pares {
		if !vistaComp[p.Competencia] {
			vistaComp[p.Competencia] = true
			comps = append(comps, p.Competencia)
		}
		if !vistaFil[p.Filial] {
			vistaFil[p.Filial] = true
			filiais = append(filiais, p.Filial)
		}
	}

	// Janela de plausibilidade: de 12 meses atrás até o mês corrente.
	agora := g.agora()
	atual := time.Date(agora.Year(), agora.Month(), 1, 0, 0, 0, 0, time.UTC)
	piso := atual.AddDate(0, -mesesPassadoPlausivel, 0)
	for _, c := range comps {
		t, err := time.Parse(formatoCompetenciaResumo, c)
		if err != nil {
			continue
		}
		switch {
		case t.Before(piso):
			res.Avisos = append(res.Avisos, Aviso{AvisoCompetenciaAntiga,
				fmt.Sprintf("Competência %s está a mais de %d meses no passado.", c, mesesPassadoPlausivel)})
		case t.After(atual):
			res.Avisos = append(res.Avisos, Aviso{AvisoCompetenciaFutura,
				fmt.Sprintf("Competência %s está no futuro (o mês corrente é %s).", c, atual.Format(formatoCompetenciaResumo))})
		}
	}

	// Mês do nome do arquivo diferente do mês dos dados.
	if sug := competenciasDoNome(arquivo); len(sug) > 0 {
		bate := false
		for _, s := range sug {
			if vistaComp[s] {
				bate = true
			}
		}
		if !bate {
			res.Avisos = append(res.Avisos, Aviso{AvisoNomeDivergente,
				fmt.Sprintf("O nome do arquivo sugere %s, mas os dados são de %s. Confira se o arquivo é o certo e se a coluna de mês está atualizada.",
					strings.Join(sug, " ou "), strings.Join(comps, ", "))})
		}
	}

	// Filial que nunca apareceu: nem no Mapa de filiais, nem em lançamentos.
	conhecidas := map[string]bool{}
	rows, err := q.QueryContext(ctx, `SELECT filial_erp FROM auditoria.filiais_gerenciais_erp
		WHERE empresa_id=$1::uuid AND filial_erp = ANY($2)
		UNION
		SELECT DISTINCT filial FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND filial = ANY($2)`,
		empresaID, pq.Array(filiais))
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			rows.Close()
			return res, err
		}
		conhecidas[f] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return res, err
	}
	rows.Close()
	for _, f := range filiais {
		if !conhecidas[f] {
			res.Avisos = append(res.Avisos, Aviso{AvisoFilialNova,
				fmt.Sprintf("Filial %s: primeira vez que esta filial aparece (não está no Mapa de filiais gerenciais nem em lançamentos anteriores).", f)})
		}
	}

	// Competência/filial que já tem lançamentos de outra origem.
	rows, err = q.QueryContext(ctx, `SELECT to_char(competencia,'YYYY-MM'), filial, origem, COUNT(*)
		FROM auditoria.lancamentos
		WHERE empresa_id=$1::uuid AND origem<>$2 AND to_char(competencia,'YYYY-MM') = ANY($3) AND filial = ANY($4)
		GROUP BY 1,2,3 ORDER BY 1,2,3`, empresaID, origem, pq.Array(comps), pq.Array(filiais))
	if err != nil {
		return res, err
	}
	defer rows.Close()
	nosPares := map[escopoImp]bool{}
	for _, p := range res.Pares {
		nosPares[escopoImp{p.Competencia, p.Filial}] = true
	}
	for rows.Next() {
		var c, f, o string
		var n int
		if err := rows.Scan(&c, &f, &o, &n); err != nil {
			return res, err
		}
		if nosPares[escopoImp{c, f}] {
			res.Avisos = append(res.Avisos, Aviso{AvisoOutraOrigem,
				fmt.Sprintf("%s, filial %s: já tem %d lançamento(s) da origem %s; esta carga é da origem %s e não os substitui.", c, f, n, o, origem)})
		}
	}
	return res, rows.Err()
}
