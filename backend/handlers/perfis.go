package handlers

// perfis.go — capacidades e matriz perfil → capacidades (AD-11).
//
// Uma rota declara a capacidade de que precisa; quem decide se o perfil a tem é
// esta matriz, num só lugar. O admin de plataforma (users.role = 'admin') tem
// todas. Ver spec-4-2 e spec-4-3 em _bmad-output/implementation-artifacts.

// Capacidade é o que um perfil pode fazer.
type Capacidade string

const (
	// Leitura.
	CapLer         Capacidade = "ler"           // telas do DRE sem restrição de escopo (inclui detalhe e departamentos)
	CapLerNoEscopo Capacidade = "ler_no_escopo" // leitura do que o escopo do perfil alcança (gestor: o escopo vem na 4.7)
	CapLerTrilha   Capacidade = "ler_trilha"    // trilha de auditoria

	// Escrita de dados e cadastros.
	CapCarregar         Capacidade = "carregar"          // carga/importação de lançamentos, orçado, atualizar dados
	CapAjustar          Capacidade = "ajustar"           // ajustes de lançamento (épicos futuros)
	CapEditarMapa       Capacidade = "editar_mapa"       // de-para conta → linha do DRE
	CapEditarEstrutura  Capacidade = "editar_estrutura"  // linhas e papéis do DRE, departamentos, filiais gerenciais, agrupamentos
	CapEditarDicionario Capacidade = "editar_dicionario" // dicionário de contas
	CapConfigurarRateio Capacidade = "configurar_rateio" // regras de rateio (épicos futuros)
	CapAplicarRateio    Capacidade = "aplicar_rateio"    // aplicar rateio (épicos futuros)
	CapDefinirLimite    Capacidade = "definir_limite"    // limites de alerta (épicos futuros)
	CapJustificar       Capacidade = "justificar"        // justificativas de variação (épicos futuros)

	// Ações de controle.
	CapFechar        Capacidade = "fechar"
	CapReabrir       Capacidade = "reabrir"
	CapGerirUsuarios Capacidade = "gerir_usuarios"
)

// TodasCapacidades lista cada capacidade uma vez; a matriz e os testes partem dela.
var TodasCapacidades = []Capacidade{
	CapLer, CapLerNoEscopo, CapLerTrilha,
	CapCarregar, CapAjustar, CapEditarMapa, CapEditarEstrutura, CapEditarDicionario,
	CapConfigurarRateio, CapAplicarRateio, CapDefinirLimite, CapJustificar,
	CapFechar, CapReabrir, CapGerirUsuarios,
}

// CapacidadesDeEscrita são as que alteram estado; diretoria e auditoria não têm nenhuma.
var CapacidadesDeEscrita = []Capacidade{
	CapCarregar, CapAjustar, CapEditarMapa, CapEditarEstrutura, CapEditarDicionario,
	CapConfigurarRateio, CapAplicarRateio, CapDefinirLimite, CapJustificar,
	CapFechar, CapReabrir, CapGerirUsuarios,
}

func conjunto(cs ...Capacidade) map[Capacidade]bool {
	m := make(map[Capacidade]bool, len(cs))
	for _, c := range cs {
		m[c] = true
	}
	return m
}

// matrizPerfis: rótulos do produto entre parênteses. Perfil fora da matriz não tem nada.
var matrizPerfis = map[string]map[Capacidade]bool{
	// Analista: lê, carrega e edita Mapa, estrutura e dicionário; não fecha nem reabre.
	"analista": conjunto(CapLer, CapLerNoEscopo, CapCarregar, CapEditarMapa, CapEditarEstrutura, CapEditarDicionario),
	// Controller: tudo.
	"controladoria": conjunto(TodasCapacidades...),
	// Gerente: só o seu escopo (a 4.7 define o escopo por departamento) e justificar.
	"gestor": conjunto(CapLerNoEscopo, CapJustificar),
	// Somente leitura. A auditoria lê também a trilha.
	"diretoria": conjunto(CapLer, CapLerNoEscopo),
	"auditoria": conjunto(CapLer, CapLerNoEscopo, CapLerTrilha),
}

// PerfilTem diz se o perfil tem a capacidade (sem considerar o admin de plataforma).
func PerfilTem(perfil string, cap Capacidade) bool { return matrizPerfis[perfil][cap] }

// Pode: admin de plataforma tem todas; os demais seguem a matriz.
func (c *AudContext) Pode(cap Capacidade) bool {
	if c.Admin {
		return true
	}
	return PerfilTem(c.Perfil, cap)
}
