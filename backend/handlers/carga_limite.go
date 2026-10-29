package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
)

// limiteCargaPadraoMB é o tamanho máximo de um arquivo de carga (lançamentos e
// dicionário). Configurável por CARGA_MAX_MB; o upload passa pelo Cloudflare
// Tunnel, que recusa corpos acima do limite do plano (100 MB nos planos Free e
// Pro), então valores maiores só valem para quem chega direto à API.
const limiteCargaPadraoMB = 1024

func limiteCargaBytes() int64 {
	mb := int64(limiteCargaPadraoMB)
	if v, err := strconv.ParseInt(os.Getenv("CARGA_MAX_MB"), 10, 64); err == nil && v > 0 {
		mb = v
	}
	return mb << 20
}

// erroUploadCarga responde à falha de r.FormFile: 413 quando o arquivo passou do
// limite, 400 nos demais casos.
func erroUploadCarga(w http.ResponseWriter, err error) {
	var grande *http.MaxBytesError
	if errors.As(err, &grande) {
		http.Error(w, fmt.Sprintf("arquivo maior que o limite de %d MB", limiteCargaBytes()>>20), http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, "arquivo obrigatório", http.StatusBadRequest)
}
