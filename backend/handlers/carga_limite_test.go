package handlers

import "testing"

func TestLimiteCargaBytes(t *testing.T) {
	t.Setenv("CARGA_MAX_MB", "")
	if got := limiteCargaBytes(); got != 1024<<20 {
		t.Fatalf("padrão deveria ser 1 GB, veio %d", got)
	}
	t.Setenv("CARGA_MAX_MB", "100")
	if got := limiteCargaBytes(); got != 100<<20 {
		t.Fatalf("CARGA_MAX_MB=100 deveria dar 100 MB, veio %d", got)
	}
	t.Setenv("CARGA_MAX_MB", "lixo")
	if got := limiteCargaBytes(); got != 1024<<20 {
		t.Fatalf("valor inválido deveria cair no padrão, veio %d", got)
	}
}
