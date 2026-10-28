package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Teste de arquitetura (código-fonte, sem banco): só backend/dre escreve em
// lancamentos, e as fontes não acessam o banco.

var escreveLancamentos = regexp.MustCompile(`(?is)\b(insert\s+into|delete\s+from|update|truncate(?:\s+table)?)\s+(?:auditoria\.)?lancamentos\b`)

func arquivosGo(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "vendor") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".go") {
			out[filepath.ToSlash(p)] = lerFonte(t, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSoODreEscreveEmLancamentos(t *testing.T) {
	for p, src := range arquivosGo(t) {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		if strings.HasPrefix(p, "dre/") && !strings.HasPrefix(p, "dre/fontes/") {
			continue
		}
		if m := escreveLancamentos.FindString(src); m != "" {
			t.Errorf("%s escreve em lancamentos (%q); só backend/dre pode", p, m)
		}
	}
}

func TestCamadasDoDre(t *testing.T) {
	for p, src := range arquivosGo(t) {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		switch {
		case strings.HasPrefix(p, "dre/fontes/"):
			if strings.Contains(src, `"database/sql"`) {
				t.Errorf("%s importa database/sql; fontes não acessam o banco", p)
			}
		case strings.HasPrefix(p, "dre/"):
			if strings.Contains(src, `"fb_auditoria/handlers"`) || strings.Contains(src, `"fb_auditoria/dre/fontes`) {
				t.Errorf("%s importa handlers ou fontes", p)
			}
		}
	}
}

func TestSoODreEscreveFechamento(t *testing.T) {
	for p, src := range arquivosGo(t) {
		if strings.HasSuffix(p, "_test.go") || !strings.HasPrefix(p, "handlers/") {
			continue
		}
		if strings.Contains(strings.ToLower(src), "insert into auditoria.dre_fechamento") {
			t.Errorf("%s escreve em dre_fechamento*; só backend/dre pode", p)
		}
	}
}

// Toda função de backend/dre que escreve em lancamentos chama Guardar (AD-8).
// Análise por AST: uma função "escreve" se tem literal de string que casa com
// escreveLancamentos; "guarda" se chama algo chamado Guardar.
func TestEscritaEmLancamentosPassaPorGuardar(t *testing.T) {
	escritas := 0
	for p := range arquivosGo(t) {
		if !strings.HasPrefix(p, "dre/") || strings.HasPrefix(p, "dre/fontes/") || strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			escreve, guarda := false, false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.BasicLit:
					if x.Kind == token.STRING && escreveLancamentos.MatchString(x.Value) {
						escreve = true
					}
				case *ast.CallExpr:
					if s, ok := x.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "Guardar" {
						guarda = true
					}
					if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "Guardar" {
						guarda = true
					}
				}
				return true
			})
			if escreve {
				escritas++
				if !guarda {
					t.Errorf("%s: %s escreve em lancamentos sem chamar Guardar", p, fn.Name.Name)
				}
			}
		}
	}
	if escritas == 0 {
		t.Error("nenhuma escrita em lancamentos encontrada em dre/; o teste perdeu o alvo")
	}
}
