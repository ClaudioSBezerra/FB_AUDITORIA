package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// História 4.7 (AD-10): varredura. Toda rota que o Gerente alcança (leitura por
// ler_no_escopo) ou aplica o escopo (escopoDe) ou está na lista das que não
// devolvem valor por conta; e todo handler que lê valor do DRE ou dos lançamentos
// usa o escopo ou exige `ler` (DRE do grupo).

var reRotaEscopo = regexp.MustCompile(`HandleFunc\("(/api/aud/[^"]+)",\s*(?:gz\()?withAud\(handlers\.(\w+),\s*[^,)]+,\s*handlers\.CapLerNoEscopo`)

// Rotas que o Gerente alcança por ler_no_escopo e que NÃO devolvem valor por conta
// (cadastros, estrutura e situação de fechamento). Qualquer rota nova de leitura
// que devolva valor por conta tem de aplicar o escopo ou ficar atrás de `ler`.
var semValorPorConta = map[string]string{
	"FiliaisGerenciaisHandler":   "cadastro de filiais gerenciais",
	"AgrupamentosFiliaisHandler": "cadastro de agrupamentos",
	"DepartamentosHandler":       "cadastro de departamentos",
	"DicionarioHandler":          "dicionário de contas (descrição e departamento, sem valor)",
	"DREFiliaisHandler":          "lista de códigos de filial",
	"DRELinhasHandler":           "estrutura do DRE",
	"DREFechamentoHandler":       "situação dos fechamentos (sem valores)",
	"DREMapaHandler":             "de-para conta para linha (sem valores)",
}

func TestEscopo_RotasDoGerenteAplicamOEscopoOuNaoTemValor(t *testing.T) {
	src := lerFonte(t, "main.go")
	vistas := 0
	for _, l := range strings.Split(src, "\n") {
		m := reRotaEscopo.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		vistas++
		caminho, handler := m[1], m[2]
		if _, ok := semValorPorConta[handler]; ok {
			continue
		}
		if !strings.Contains(corpoDaFuncao(t, handler), "escopoDe(") {
			t.Errorf("%s (%s) é alcançada pelo Gerente (ler_no_escopo) mas não aplica escopoDe", caminho, handler)
		}
	}
	if vistas < 8 {
		t.Errorf("só %d rotas ler_no_escopo reconhecidas; o teste perdeu rotas", vistas)
	}
	// A lista de exceções não pode ter nome sobrando.
	for h := range semValorPorConta {
		if !strings.Contains(src, "handlers."+h) {
			t.Errorf("%s da lista de exceções não existe em main.go", h)
		}
	}
}

// Chamadas que leem valor (por conta ou de linha) do DRE e dos lançamentos.
var leValor = regexp.MustCompile(`\.LerFiliais\(|\.Ler\(|\.LerTrimestres\(|\.ContasDoEscopo\(|\.ContasPorDepartamento|\.ContasDaLinha\(|\.DetalheConta\(|\.ContasCongeladas\(|FROM auditoria\.lancamentos|dre_fechamento_contas`)

// Handlers que tocam essas fontes sem devolver valor ao usuário.
var leSemDevolverValor = map[string]string{
	"DREFiliaisHandler":     "só os códigos de filial",
	"AmbienteLimparHandler": "apaga, não devolve",
	"AmbienteVirarHandler":  "só o admin de plataforma; devolve contagens",
	"DREFechamentoHandler":  "o POST congela; a resposta é só id e versão",
	"DREReabrirHandler":     "reabre; sem valores na resposta",
	"DREImportarHandler":    "escrita",
	"DREOrcadoHandler":      "escrita",
}

func TestEscopo_HandlerQueLeValorUsaEscopoOuExigeLer(t *testing.T) {
	arqs, _ := filepath.Glob("handlers/*.go")
	achados := []string{}
	for _, a := range arqs {
		if strings.HasSuffix(a, "_test.go") {
			continue
		}
		b, err := os.ReadFile(a)
		if err != nil {
			t.Fatal(err)
		}
		fonte := string(b)
		f, err := parser.ParseFile(token.NewFileSet(), a, fonte, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !strings.HasSuffix(fn.Name.Name, "Handler") {
				continue
			}
			corpo := fonte[fn.Pos()-1 : fn.End()-1]
			if !leValor.MatchString(corpo) {
				continue
			}
			achados = append(achados, fn.Name.Name)
			if _, ok := leSemDevolverValor[fn.Name.Name]; ok {
				continue
			}
			if !strings.Contains(corpo, "escopoDe(") && !strings.Contains(corpo, "exigeLer(") {
				t.Errorf("%s (%s) lê valor do DRE/lançamentos sem escopoDe nem exigeLer", fn.Name.Name, a)
			}
		}
	}
	sort.Strings(achados)
	for _, esperado := range []string{"DREHandler", "DREDepartamentosHandler", "DREContasHandler", "DREContaLancamentosHandler", "DRETrimestresHandler", "DRELancamentosHandler"} {
		i := sort.SearchStrings(achados, esperado)
		if i >= len(achados) || achados[i] != esperado {
			t.Errorf("a varredura não encontrou %s; o teste perdeu o alvo (achados: %v)", esperado, achados)
		}
	}
}
