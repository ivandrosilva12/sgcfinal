// Package i18n concentra as mensagens em PT-PT angolano (pt-AO) apresentadas ao
// utilizador — mensagens de erro RFC 7807, healthchecks e respostas da API.
//
// Pertence ao Shared Kernel (Camada 1 — Domínio): a linguagem ubíqua PT-PT é um
// princípio de domínio (CLAUDE.md §1). Sem dependências — é uma folha
// importável por qualquer camada. Centralizar desde o Sprint 1 evita literais
// dispersos e facilita revisão linguística.
package i18n

// Chave identifica uma mensagem traduzível.
type Chave string

const (
	// MsgServicoIndisponivel — dependência crítica em baixo (readyz).
	MsgServicoIndisponivel Chave = "servico.indisponivel"
	// MsgServicoOperacional — todas as dependências prontas.
	MsgServicoOperacional Chave = "servico.operacional"
	// MsgNaoAutenticado — pedido sem credenciais válidas.
	MsgNaoAutenticado Chave = "erro.nao_autenticado"
	// MsgSemPermissao — autenticado mas sem permissão para o recurso.
	MsgSemPermissao Chave = "erro.sem_permissao"
	// MsgRecursoNaoEncontrado — recurso inexistente.
	MsgRecursoNaoEncontrado Chave = "erro.nao_encontrado"
	// MsgErroInterno — falha inesperada no servidor.
	MsgErroInterno Chave = "erro.interno"
	// MsgPedidoInvalido — validação de entrada falhou.
	MsgPedidoInvalido Chave = "erro.pedido_invalido"
	// MsgConflito — conflito de estado do recurso.
	MsgConflito Chave = "erro.conflito"
	// MsgDemasiadosPedidos — limite de taxa excedido (429).
	MsgDemasiadosPedidos Chave = "erro.demasiados_pedidos"
	// MsgMFAObrigatoria — papel sensível sem segundo fator de autenticação.
	MsgMFAObrigatoria Chave = "erro.mfa_obrigatoria"
	// MsgPapelInvalido — código de papel desconhecido.
	MsgPapelInvalido Chave = "erro.papel_invalido"
	// MsgUtilizadorNaoEncontrado — utilizador inexistente no Keycloak.
	MsgUtilizadorNaoEncontrado Chave = "erro.utilizador_nao_encontrado"
)

// mensagensPtAO é o catálogo pt-AO.
var mensagensPtAO = map[Chave]string{
	MsgServicoIndisponivel:     "Serviço temporariamente indisponível.",
	MsgServicoOperacional:      "Serviço operacional.",
	MsgNaoAutenticado:          "Autenticação necessária.",
	MsgSemPermissao:            "Não tem permissão para aceder a este recurso.",
	MsgRecursoNaoEncontrado:    "Recurso não encontrado.",
	MsgErroInterno:             "Ocorreu um erro interno. Tente novamente mais tarde.",
	MsgPedidoInvalido:          "Pedido inválido.",
	MsgConflito:                "Conflito com o estado actual do recurso.",
	MsgDemasiadosPedidos:       "Demasiados pedidos. Tente novamente mais tarde.",
	MsgMFAObrigatoria:          "Autenticação com segundo factor obrigatória para este perfil.",
	MsgPapelInvalido:           "Papel inválido.",
	MsgUtilizadorNaoEncontrado: "Utilizador não encontrado.",
}

// T devolve a mensagem pt-AO para a chave. Se a chave for desconhecida, devolve
// a própria chave (falha visível, não silenciosa).
func T(chave Chave) string {
	if m, ok := mensagensPtAO[chave]; ok {
		return m
	}
	return string(chave)
}
