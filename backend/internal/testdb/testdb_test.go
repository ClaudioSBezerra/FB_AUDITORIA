package testdb

import (
	"strings"
	"testing"
)

// A guarda roda antes de qualquer conexão: aponta o host para um endereço
// inválido, então qualquer tentativa de conectar mudaria a mensagem de erro.
func TestAbrirRecusaNomeInvalido(t *testing.T) {
	t.Setenv("TEST_DB_HOST", "host.invalido.local")
	casos := []string{"fb_auditoria", "fb_auditoria_prod", "postgres", "", "x_test; DROP DATABASE y", "FB_TEST"}
	for _, nome := range casos {
		db, err := Abrir(nome)
		if err == nil || db != nil {
			t.Fatalf("Abrir(%q) devia recusar", nome)
		}
		if !strings.Contains(err.Error(), "_test") || strings.Contains(err.Error(), "inacessível") {
			t.Errorf("Abrir(%q): mensagem inesperada: %v", nome, err)
		}
	}
}

func TestValidarNomeAceita(t *testing.T) {
	for _, nome := range []string{"fb_auditoria_test", "fb_auditoria_dre_test"} {
		if err := ValidarNome(nome); err != nil {
			t.Errorf("ValidarNome(%q): %v", nome, err)
		}
	}
}
