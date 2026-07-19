package platform

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A exposição que a ADR-042 corrige (R3 da ADR-040) existiu porque cada local de
// chamada escolhia os seus middlewares: três grupos receberam o mfaMW e onze não,
// e nada tornava isso visível. O pacote `protecao` torna a omissão um desvio
// deliberado — esta guarda torna-a detectável.
//
// O teste lê o código-fonte em vez de exercitar comportamento. É invulgar, e a
// razão está registada na ADR-042 §4.1: o que falhou aqui foi a LIGAÇÃO, não o
// comportamento, e prová-la por comportamento exigiria montar os catorze
// handlers com todos os seus fakes só para verificar uma propriedade de wiring.
//
// RegistarHealth está isento por desenho: healthchecks e o scrape do Prometheus
// são não-autenticados. Isentá-lo aqui é deliberado, não esquecimento.
func TestRegistarRotas_TodasAsRotasDeNegocioUsamOPacoteProteccao(t *testing.T) {
	fonte, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatalf("ler app.go: %v", err)
	}
	corpo := string(fonte)

	inicio := strings.Index(corpo, "registarRotas := func(")
	if inicio < 0 {
		t.Fatal("não encontrei registarRotas em app.go — o teste precisa de ser actualizado")
	}
	fim := strings.Index(corpo[inicio:], "\n\t}")
	if fim < 0 {
		t.Fatal("não encontrei o fim de registarRotas em app.go")
	}
	bloco := corpo[inicio : inicio+fim]

	chamadas := regexp.MustCompile(`adhttp\.(Registar\w+)\(([^\n]*)\)`).FindAllStringSubmatch(bloco, -1)
	if len(chamadas) == 0 {
		t.Fatal("não encontrei chamadas adhttp.Registar* dentro de registarRotas")
	}

	var semProteccao []string
	for _, c := range chamadas {
		nome, argumentos := c[1], c[2]
		if nome == "RegistarHealth" {
			continue // isento por desenho: não-autenticado
		}
		if !strings.HasSuffix(strings.TrimSpace(argumentos), "protecao...") {
			semProteccao = append(semProteccao, nome)
		}
	}
	if len(semProteccao) > 0 {
		t.Errorf("grupos de rotas sem o pacote `protecao`: %v\n"+
			"Todo o grupo de negócio tem de terminar em `protecao...`. Se um grupo "+
			"tiver mesmo de ficar sem MFA, isso exige uma ADR — não uma excepção "+
			"silenciosa aqui.", semProteccao)
	}
	if len(chamadas) < 14 {
		t.Errorf("esperava pelo menos 14 grupos de rotas, encontrei %d — "+
			"se um grupo foi removido, actualiza este número deliberadamente", len(chamadas))
	}
}
