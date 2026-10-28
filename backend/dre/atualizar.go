package dre

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"fb_auditoria/trilha"
)

// EscopoAtualizacao é o pedido de "Atualizar dados": quem pede e para qual empresa.
type EscopoAtualizacao struct {
	EmpresaID string
	UserID    string
}

// Atualizavel é uma fonte que o botão "Atualizar dados" sabe disparar (realizado
// ou orçado). A implementação grava pelos caminhos que já têm guarda de período:
// o Ingestor (Guardar, por competência) e, para o orçado, a guarda da escrita do
// orçado. Competência com fechamento ativo volta como *ErrPeriodoFechado.
type Atualizavel interface {
	Nome() string
	Atualizar(ctx context.Context, e EscopoAtualizacao) (Resultado, error)
}

// ViaIngestor adapta uma Fonte de lançamentos (por exemplo, a do Oracle) a
// Atualizavel: a importação passa pelo Ingestor e, portanto, pela guarda de período.
func ViaIngestor(ing *Ingestor, f Fonte) Atualizavel { return &viaIngestor{ing, f} }

type viaIngestor struct {
	ing *Ingestor
	f   Fonte
}

func (v *viaIngestor) Nome() string { return v.f.Nome() }
func (v *viaIngestor) Atualizar(ctx context.Context, e EscopoAtualizacao) (Resultado, error) {
	return v.ing.Importar(ctx, e.EmpresaID, e.UserID, v.f)
}

// Registro guarda as fontes atualizáveis, na ordem de registro.
type Registro struct {
	mu     sync.Mutex
	fontes []Atualizavel
}

// RegistroPadrao é o registro do processo. Nenhuma fonte entra por padrão: a do
// Oracle (histórias 2.1 e 2.3 a 2.6) se registra aqui quando existir.
var RegistroPadrao = &Registro{}

// Registrar acrescenta uma fonte; nome vazio ou repetido é recusado.
func (r *Registro) Registrar(a Atualizavel) error {
	if a == nil || a.Nome() == "" {
		return errors.New("fonte atualizável sem nome")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.fontes {
		if x.Nome() == a.Nome() {
			return fmt.Errorf("fonte atualizável já registrada: %s", a.Nome())
		}
	}
	r.fontes = append(r.fontes, a)
	return nil
}

func (r *Registro) lista() []Atualizavel {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Atualizavel(nil), r.fontes...)
}

// Estados de uma atualização (por fonte e geral).
const (
	AtualizacaoOK       = "ok"
	AtualizacaoSemFonte = "sem_fonte"
	AtualizacaoRecusada = "recusada"
	AtualizacaoErro     = "erro"
	AtualizacaoParcial  = "parcial"
)

// ResultadoFonte é o que aconteceu com uma fonte.
type ResultadoFonte struct {
	Fonte    string   `json:"fonte"`
	Status   string   `json:"status"` // ok, recusada, erro
	Linhas   int      `json:"linhas"`
	CargaID  int64    `json:"carga_id,omitempty"`
	Mensagem string   `json:"mensagem,omitempty"`
	Fechadas []string `json:"competencias_fechadas,omitempty"`
}

// Atualizacao é o resultado do pedido, com a hora em UTC.
type Atualizacao struct {
	Hora     time.Time        `json:"hora"`
	Status   string           `json:"status"`
	Mensagem string           `json:"mensagem"`
	Fontes   []ResultadoFonte `json:"fontes"`
}

// Atualizador dispara as fontes registradas e registra tudo na trilha.
type Atualizador struct {
	DB       *sql.DB
	Registro *Registro
	Agora    func() time.Time // para teste; nil = time.Now
}

// MensagemSemFonte é o aviso quando não há fonte configurada: nunca é sucesso.
const MensagemSemFonte = "nenhuma fonte configurada: não há de onde buscar dados novos"

// Atualizar registra o pedido na trilha (quem, escopo), roda cada fonte
// registrada em sequência e registra o resultado, também quando recusado, com
// falha ou sem fonte. Falha de uma fonte não impede as demais. O erro devolvido
// é só de infraestrutura (trilha); o resultado de negócio vem em Atualizacao.
func (a *Atualizador) Atualizar(ctx context.Context, e EscopoAtualizacao) (Atualizacao, error) {
	agora := time.Now
	if a.Agora != nil {
		agora = a.Agora
	}
	fontes := a.Registro.lista()
	nomes := make([]string, 0, len(fontes))
	for _, f := range fontes {
		nomes = append(nomes, f.Nome())
	}
	inicio := agora().UTC()
	id := inicio.Format(time.RFC3339Nano)
	if err := a.gravar(ctx, e, id, "solicitar", map[string]any{"fontes": nomes}); err != nil {
		return Atualizacao{}, err
	}

	res := Atualizacao{Hora: inicio, Fontes: []ResultadoFonte{}}
	for _, f := range fontes {
		res.Fontes = append(res.Fontes, rodar(ctx, f, e))
	}
	res.Status, res.Mensagem = consolidar(res.Fontes)
	res.Hora = agora().UTC()
	if err := a.gravar(ctx, e, id, "concluir", map[string]any{
		"fontes": nomes, "status": res.Status, "mensagem": res.Mensagem, "resultado": res.Fontes,
	}); err != nil {
		return Atualizacao{}, err
	}
	return res, nil
}

func (a *Atualizador) gravar(ctx context.Context, e EscopoAtualizacao, id, acao string, payload any) error {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := trilha.Gravar(tx, e.EmpresaID, e.UserID, "atualizacao_dados", id, acao, payload); err != nil {
		return err
	}
	return tx.Commit()
}

func rodar(ctx context.Context, f Atualizavel, e EscopoAtualizacao) ResultadoFonte {
	r, err := f.Atualizar(ctx, e)
	out := ResultadoFonte{Fonte: f.Nome(), Linhas: r.Linhas, CargaID: r.CargaID, Status: AtualizacaoOK}
	if err == nil {
		return out
	}
	out.Linhas, out.CargaID = 0, 0
	var fechado *ErrPeriodoFechado
	var entrada *ErroEntrada
	var linhas *ErroLinhas
	switch {
	case errors.As(err, &fechado):
		out.Status = AtualizacaoRecusada
		out.Mensagem = fechado.Error() + ". Reabra o fechamento do mês, atualize e feche de novo."
		for _, s := range fechado.Escopos {
			out.Fechadas = append(out.Fechadas, s.Competencia)
		}
	case errors.As(err, &entrada), errors.As(err, &linhas):
		out.Status = AtualizacaoErro
		out.Mensagem = err.Error()
	default:
		log.Printf("atualizar dados (%s): %v", f.Nome(), err)
		out.Status = AtualizacaoErro
		out.Mensagem = "falha ao atualizar a fonte " + f.Nome()
	}
	return out
}

func consolidar(fs []ResultadoFonte) (string, string) {
	if len(fs) == 0 {
		return AtualizacaoSemFonte, MensagemSemFonte
	}
	var ok, recusada, erro, linhas int
	for _, f := range fs {
		switch f.Status {
		case AtualizacaoOK:
			ok++
			linhas += f.Linhas
		case AtualizacaoRecusada:
			recusada++
		default:
			erro++
		}
	}
	switch {
	case recusada+erro == 0:
		return AtualizacaoOK, fmt.Sprintf("dados atualizados: %d linha(s) em %d fonte(s)", linhas, ok)
	case ok > 0:
		return AtualizacaoParcial, "atualização parcial: veja o resultado de cada fonte"
	case erro == 0:
		return AtualizacaoRecusada, "atualização recusada: competência com fechamento ativo; reabra o fechamento do mês, atualize e feche de novo"
	default:
		return AtualizacaoErro, "a atualização falhou: veja o resultado de cada fonte"
	}
}
