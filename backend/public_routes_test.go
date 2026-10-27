package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Teste de código-fonte (não usa banco): lista as rotas públicas registradas em
// main.go e garante que nenhuma delas cria usuário. Uma rota pública nova obriga
// a atualizar esta lista, o que força alguém a pensar nela.

// Rotas públicas esperadas, com handler identificável em uma linha de main.go.
var publicasEsperadas = []string{
	"/api/auth/forgot-password",
	"/api/auth/login",
	"/api/auth/logout",
	"/api/auth/refresh",
	"/api/auth/register",
	"/api/auth/reset-password",
}

// Rotas registradas com função inline, em várias linhas: o teste não classifica o
// handler delas pelo texto, então são listadas à mão. Incluir uma rota aqui exige
// editar este arquivo, e main.go inteiro é conferido contra criação de usuário.
var publicasMultilinha = map[string]string{
	"/api/health":              "responde só o status do serviço",
	"/api/config/empresa/logo": "chama AuthMiddleware dentro da função",
	"/":                        "serve o frontend estático",
}

// Qualquer INSERT em users, sem diferenciar maiúsculas, com ou sem schema, em uma ou várias linhas.
var criaUsuario = regexp.MustCompile(`(?is)insert\s+into\s+(?:\w+\.)?users\b`)

var reRota = regexp.MustCompile(`http\.HandleFunc\("([^"]+)",\s*(.*)`)
var autenticada = regexp.MustCompile(`withAuth\(|withAud\(|AuthMiddleware\(|AuditoriaAuthMiddleware\(`)

func lerFonte(t *testing.T, caminho string) string {
	t.Helper()
	b, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("ler %s: %v", caminho, err)
	}
	return string(b)
}

// corpoDaFuncao devolve o texto da função de nível superior `nome` em handlers/*.go.
func corpoDaFuncao(t *testing.T, nome string) string {
	t.Helper()
	arquivos, _ := filepath.Glob("handlers/*.go")
	for _, a := range arquivos {
		if strings.HasSuffix(a, "_test.go") {
			continue
		}
		fonte := lerFonte(t, a)
		ini := strings.Index(fonte, "\nfunc "+nome+"(")
		if ini < 0 {
			continue
		}
		fim := strings.Index(fonte[ini+1:], "\nfunc ")
		if fim < 0 {
			return fonte[ini:]
		}
		return fonte[ini : ini+1+fim]
	}
	t.Fatalf("função %s não encontrada em handlers/", nome)
	return ""
}

func TestRotasPublicasNaoCriamUsuario(t *testing.T) {
	main := lerFonte(t, "main.go")

	publicas := map[string]string{} // caminho -> handler (vazio se multilinha)
	lidas := 0
	for _, linha := range strings.Split(main, "\n") {
		m := reRota.FindStringSubmatch(strings.TrimSpace(linha))
		if m == nil {
			continue
		}
		lidas++
		caminho, resto := m[1], m[2]
		if autenticada.MatchString(resto) {
			continue
		}
		if _, ok := publicasMultilinha[caminho]; ok {
			continue
		}
		h := regexp.MustCompile(`handlers\.(\w+)`).FindStringSubmatch(resto)
		if h == nil {
			t.Errorf("rota pública %s sem handler identificável: classifique-a em publicasMultilinha", caminho)
			continue
		}
		publicas[caminho] = h[1]
	}

	// Um registro em várias linhas (http.HandleFunc( e o caminho na linha seguinte)
	// escaparia da leitura acima; a contagem pega isso.
	if total := strings.Count(main, "http.HandleFunc("); total != lidas {
		t.Fatalf("main.go tem %d registros http.HandleFunc, mas o teste leu %d: algum está escrito de forma que o teste não enxerga", total, lidas)
	}
	if criaUsuario.MatchString(main) {
		t.Error("main.go não deve criar usuário; isso pertence a um handler")
	}

	var achadas []string
	for c := range publicas {
		achadas = append(achadas, c)
	}
	sort.Strings(achadas)
	if strings.Join(achadas, ",") != strings.Join(publicasEsperadas, ",") {
		t.Fatalf("o conjunto de rotas públicas mudou.\n esperado: %v\n achado:   %v\nSe a mudança for intencional, atualize publicasEsperadas e confirme que a rota nova não cria usuário.", publicasEsperadas, achadas)
	}

	for caminho, handler := range publicas {
		corpo := corpoDaFuncao(t, handler)
		if criaUsuario.MatchString(corpo) && handler != "RegisterHandler" {
			t.Errorf("%s (%s) cria usuário, mas não é o cadastro fechado por ALLOW_SELF_REGISTER", caminho, handler)
		}
	}

	// O único caminho público que cria usuário precisa estar atrás da guarda.
	registro := corpoDaFuncao(t, "RegisterHandler")
	if !strings.Contains(registro, "SelfRegisterEnabled()") {
		t.Error("RegisterHandler cria usuário e não consulta SelfRegisterEnabled()")
	}
	if i, loc := strings.Index(registro, "SelfRegisterEnabled()"), criaUsuario.FindStringIndex(registro); loc != nil && i > loc[0] {
		t.Error("SelfRegisterEnabled() precisa vir antes do INSERT INTO users em RegisterHandler")
	}

	// Exceção documentada: o LoginHandler auto-provisiona ambiente, grupo e empresa
	// para quem já entrou com credencial válida e não tem empresa. Não é cadastro
	// público (exige conta existente), e não cria usuário. Risco registrado na
	// história 4.9, que deve desligar esse caminho.
	if criaUsuario.MatchString(corpoDaFuncao(t, "LoginHandler")) {
		t.Error("LoginHandler passou a criar usuário")
	}
}
