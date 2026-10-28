package main

import (
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
