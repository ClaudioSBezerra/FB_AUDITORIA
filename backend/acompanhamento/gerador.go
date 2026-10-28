package acompanhamento

// gerador.go — lê sprint-status.yaml e epics.md (já públicos) e monta Dados.
// Não lê respostas do cliente nem deferred-work.md, e não copia as "Notas" das
// histórias: só título, resumo e critérios de aceite.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	arqEpics  = "planning-artifacts/epics.md"
	arqSprint = "implementation-artifacts/sprint-status.yaml"
)

var (
	reStatusLinha = regexp.MustCompile(`^\s+(\d+)-(\d+)-\S+:\s*(\S+)\s*$`)
	reEpicTitulo  = regexp.MustCompile(`^#{2,3} Epic (\d+): (.+?)\s*$`)
	reStoryTitulo = regexp.MustCompile(`^#{3,4} Story (\d+)\.(\d+): (.+?)\s*$`)
	reQualquerCab = regexp.MustCompile(`^#{1,4} `)
	reRefsFinais  = regexp.MustCompile(`\s*\((?:[A-Z]{1,3}-?\d+(?:,\s*)?)+\)`)
	reHistEpico   = regexp.MustCompile(`história (\d+\.\d+)\b.*depende d[ae] uma resposta d[oa] cliente`)
	reEpicoBloq   = regexp.MustCompile(`(?i)depende de respostas? do cliente|só começa depois de o cliente responder`)
	reDescartada  = regexp.MustCompile(`\*\*descartada`)
	reDadoReal    = regexp.MustCompile(`\d{2}\.\d{3}\.\d{3}|\d{8,}|R\$`)
	reMarkup      = regexp.MustCompile("[`*]|^>\\s*")
	reFraseBloq   = regexp.MustCompile(`(?i)depende d[ao]s? (?:uma )?(?:resposta|decisão)|ainda depende d|só é executável quando|até lá, não executar|depende de uma resposta|aguarda(?:r)? (?:a )?(?:resposta|decisão)`)
)

var statusColuna = map[string]string{
	"backlog":       ColBacklog,
	"ready-for-dev": ColPronto,
	"in-progress":   ColEmAndamento,
	"review":        ColEmRevisao,
	"done":          ColFeito,
}

func lerStatus(raiz string) (map[string]string, error) {
	b, err := os.ReadFile(filepath.Join(raiz, arqSprint))
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		m := reStatusLinha.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		col, ok := statusColuna[m[3]]
		if !ok {
			return nil, fmt.Errorf("status desconhecido %q na história %s.%s", m[3], m[1], m[2])
		}
		out[m[1]+"."+m[2]] = col
	}
	return out, nil
}

func limpar(s string) string {
	s = reMarkup.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func traduzirLinha(l string) string {
	for _, p := range [][2]string{{"Given ", "Dado "}, {"When ", "Quando "}, {"Then ", "Então "}, {"And ", "E "}, {"But ", "Mas "}} {
		if strings.HasPrefix(l, p[0]) {
			return p[1] + l[len(p[0]):]
		}
	}
	return l
}

// resumoDe monta "Como X, quero Y, para que Z." a partir das três linhas.
func resumoDe(linhas []string) string {
	var como, quero, para string
	for _, l := range linhas {
		l = limpar(l)
		switch {
		case strings.HasPrefix(l, "As a "):
			como = strings.TrimSuffix(strings.TrimPrefix(l, "As a "), ",")
		case strings.HasPrefix(l, "As an "):
			como = strings.TrimSuffix(strings.TrimPrefix(l, "As an "), ",")
		case strings.HasPrefix(l, "I want "):
			quero = strings.TrimSuffix(strings.TrimPrefix(l, "I want "), ",")
		case strings.HasPrefix(l, "So that "):
			para = strings.TrimSuffix(strings.TrimPrefix(l, "So that "), ".")
		}
	}
	if como == "" && quero == "" && para == "" {
		return ""
	}
	return fmt.Sprintf("Como %s, quero %s, para que %s.", como, quero, para)
}

// motivoBloqueio devolve a frase do corpo da história que marca dependência de
// resposta ou decisão do cliente; vazio se não houver. Frase com cara de dado
// real vira um motivo genérico.
func motivoBloqueio(corpo []string) string {
	for _, l := range corpo {
		// Só notas e revisões (não os critérios de aceite).
		if t := strings.TrimSpace(l); !strings.HasPrefix(t, "**Nota") && !strings.HasPrefix(t, ">") {
			continue
		}
		if !reFraseBloq.MatchString(l) {
			continue
		}
		t := limpar(l)
		t = strings.TrimPrefix(t, "Nota: ")
		// Pega a frase que contém o gatilho.
		for _, frase := range strings.SplitAfter(t, ". ") {
			if reFraseBloq.MatchString(frase) {
				t = strings.TrimSpace(frase)
				break
			}
		}
		if reDadoReal.MatchString(t) || strings.Contains(strings.ToLower(t), "respostas-cliente") {
			return "Depende de resposta do cliente."
		}
		if r := []rune(t); len(r) > 220 {
			t = string(r[:217]) + "..."
		}
		return t
	}
	return ""
}

// Gerar monta os dados a partir da pasta _bmad-output (raiz). Metadados vazios:
// quem chama decide o gerado_em e a versao.
func Gerar(raiz string) (Dados, error) {
	status, err := lerStatus(raiz)
	if err != nil {
		return Dados{}, err
	}
	b, err := os.ReadFile(filepath.Join(raiz, arqEpics))
	if err != nil {
		return Dados{}, err
	}
	linhas := strings.Split(string(b), "\n")

	nomes := map[int]string{}
	motivoEpico := map[int]string{}       // épico inteiro depende do cliente
	motivoHistoria := map[string]string{} // nota de épico que cita uma história
	type sec struct {
		ep, n  int
		titulo string
		corpo  []string
	}
	var secoes []sec
	for i := 0; i < len(linhas); i++ {
		if m := reEpicTitulo.FindStringSubmatch(linhas[i]); m != nil {
			n, _ := strconv.Atoi(m[1])
			nomes[n] = strings.TrimSpace(m[2])
			for j := i + 1; j < len(linhas) && !reQualquerCab.MatchString(linhas[j]); j++ {
				if hm := reHistEpico.FindStringSubmatch(linhas[j]); hm != nil {
					motivoHistoria[hm[1]] = "Depende de resposta do cliente."
				}
				if reEpicoBloq.MatchString(linhas[j]) {
					motivoEpico[n] = "O épico só começa depois da resposta do cliente."
				}
			}
			continue
		}
		m := reStoryTitulo.FindStringSubmatch(linhas[i])
		if m == nil {
			continue
		}
		ep, _ := strconv.Atoi(m[1])
		n, _ := strconv.Atoi(m[2])
		s := sec{ep: ep, n: n, titulo: m[3]}
		j := i + 1
		for ; j < len(linhas) && !reQualquerCab.MatchString(linhas[j]); j++ {
			s.corpo = append(s.corpo, linhas[j])
		}
		secoes = append(secoes, s)
		i = j - 1
	}

	var hist []Historia
	vistos := map[string]bool{}
	for _, s := range secoes {
		id := fmt.Sprintf("%d.%d", s.ep, s.n)
		if vistos[id] {
			return Dados{}, fmt.Errorf("história %s duplicada no epics.md", id)
		}
		vistos[id] = true
		if reDescartada.MatchString(strings.Join(s.corpo, "\n")) {
			continue
		}
		col, ok := status[id]
		if !ok {
			col = ColBacklog
		}
		h := Historia{
			ID:      id,
			Titulo:  limpar(reRefsFinais.ReplaceAllString(s.titulo, "")),
			Epico:   Epico{Numero: s.ep, Nome: nomes[s.ep]},
			Release: ReleaseAtual,
			Status:  col,
		}
		if h.Epico.Nome == "" {
			return Dados{}, fmt.Errorf("história %s sem épico %d no epics.md", id, s.ep)
		}
		// Resumo: linhas "As a/I want/So that" antes dos critérios.
		var resumo []string
		emCriterios := false
		var blocos []string
		var atual []string
		fecha := func() {
			if len(atual) > 0 {
				blocos = append(blocos, strings.Join(atual, "\n"))
				atual = nil
			}
		}
		for _, l := range s.corpo {
			t := strings.TrimSpace(l)
			switch {
			case strings.HasPrefix(t, "**Acceptance Criteria"):
				emCriterios = true
				fecha()
			case emCriterios:
				if t == "" {
					fecha()
				} else if strings.HasPrefix(t, "**Nota") {
					fecha()
				} else {
					atual = append(atual, traduzirLinha(limpar(t)))
				}
			case strings.HasPrefix(t, "As a") || strings.HasPrefix(t, "I want ") || strings.HasPrefix(t, "So that "):
				resumo = append(resumo, t)
			}
		}
		fecha()
		h.Resumo = resumoDe(resumo)
		h.Criterios = blocos
		if h.Criterios == nil {
			h.Criterios = []string{}
		}
		m := motivoBloqueio(s.corpo)
		if m == "" {
			m = motivoHistoria[id]
		}
		if m == "" {
			m = motivoEpico[s.ep]
		}
		if m != "" {
			h.BloqueadaCliente = true
			h.MotivoBloqueio = m
		}
		hist = append(hist, h)
	}
	for id := range status {
		if !vistos[id] {
			return Dados{}, fmt.Errorf("história %s está no sprint-status.yaml mas não no epics.md", id)
		}
	}
	sort.SliceStable(hist, func(i, j int) bool {
		if hist[i].Epico.Numero != hist[j].Epico.Numero {
			return hist[i].Epico.Numero < hist[j].Epico.Numero
		}
		a, _ := strconv.Atoi(strings.Split(hist[i].ID, ".")[1])
		b, _ := strconv.Atoi(strings.Split(hist[j].ID, ".")[1])
		return a < b
	})
	return Dados{Historias: hist}, nil
}

// MesmasHistorias compara só o conteúdo (o teste ignora os metadados).
func MesmasHistorias(a, b Dados) bool { return reflect.DeepEqual(a.Historias, b.Historias) }

func gitCurto(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Escrever gera e grava o arquivo. Se as histórias não mudaram em relação ao
// arquivo atual, mantém gerado_em e versao (regenerar sem mudança não altera nada).
func Escrever(raiz, destino string) (bool, error) {
	novo, err := Gerar(raiz)
	if err != nil {
		return false, err
	}
	var antigo Dados
	if b, err := os.ReadFile(destino); err == nil {
		_ = json.Unmarshal(b, &antigo)
	}
	if MesmasHistorias(antigo, novo) && antigo.GeradoEm != "" {
		novo.GeradoEm, novo.Versao = antigo.GeradoEm, antigo.Versao
	} else {
		novo.GeradoEm = time.Now().Format(formatoDataGera)
		novo.Versao = gitCurto(filepath.Dir(destino))
	}
	b, err := json.MarshalIndent(novo, "", "  ")
	if err != nil {
		return false, err
	}
	b = append(b, '\n')
	if old, err := os.ReadFile(destino); err == nil && string(old) == string(b) {
		return false, nil
	}
	return true, os.WriteFile(destino, b, 0o644)
}
