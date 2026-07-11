package http

import (
	"context"
	nethttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/identidade"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/i18n"
)

// chaveSessao é a chave de contexto Gin onde a sessão autenticada é guardada.
const chaveSessao = "sessao_identidade"

// Autenticador valida um token e devolve a sessão. Implementado por
// application/identidade.CasoAutenticar.
type Autenticador interface {
	Executar(ctx context.Context, tokenBruto string) (dominio.Sessao, error)
}

// Auth exige um Bearer token válido. Em caso de falha responde 401 (RFC 7807) e
// aborta a cadeia; em sucesso guarda a sessão no contexto.
func Auth(auth Autenticador) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extrairBearer(c.GetHeader("Authorization"))
		sessao, err := auth.Executar(c.Request.Context(), token)
		if err != nil {
			responderErro(c, err)
			return
		}
		c.Set(chaveSessao, sessao)
		c.Next()
	}
}

// RBAC exige que a sessão possua pelo menos um dos papéis indicados; caso
// contrário responde 403 (RFC 7807).
func RBAC(permitidos ...dominio.Papel) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessao, ok := SessaoDe(c)
		if !ok {
			responderErro(c, erros.Novo(erros.CategoriaNaoAutorizado, i18n.T(i18n.MsgNaoAutenticado)))
			return
		}
		if err := dominio.Autorizar(sessao, permitidos...); err != nil {
			responderErro(c, err)
			return
		}
		c.Next()
	}
}

// SessaoDe devolve a sessão autenticada colocada no contexto pelo middleware Auth.
func SessaoDe(c *gin.Context) (dominio.Sessao, bool) {
	v, existe := c.Get(chaveSessao)
	if !existe {
		return dominio.Sessao{}, false
	}
	s, ok := v.(dominio.Sessao)
	return s, ok
}

func extrairBearer(cabecalho string) string {
	const prefixo = "Bearer "
	if len(cabecalho) > len(prefixo) && strings.EqualFold(cabecalho[:len(prefixo)], prefixo) {
		return strings.TrimSpace(cabecalho[len(prefixo):])
	}
	return ""
}

// SegurancaHTTP aplica cabeçalhos de segurança e CORS por ambiente. Em produção
// activa HSTS. Pedidos OPTIONS (preflight) terminam com 204.
func SegurancaHTTP(origensPermitidas []string, producao bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if producao {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		if origem := c.GetHeader("Origin"); origem != "" && origemPermitida(origem, origensPermitidas) {
			h.Set("Access-Control-Allow-Origin", origem)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
			h.Set("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == nethttp.MethodOptions {
			c.AbortWithStatus(nethttp.StatusNoContent)
			return
		}
		c.Next()
	}
}

func origemPermitida(origem string, permitidas []string) bool {
	for _, p := range permitidas {
		if p == "*" || p == origem {
			return true
		}
	}
	return false
}

// Limitador é a porta do rate limiter (implementada por adapters/redis).
type Limitador interface {
	Permitir(ctx context.Context, chave string, limite int, janela time.Duration) (bool, int, time.Duration, error)
}

// LimiteTaxa aplica rate limiting por IP. Se o backend (Redis) falhar, deixa
// passar (fail-open) para não derrubar o serviço por indisponibilidade da cache.
func LimiteTaxa(l Limitador, limite int, janela time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		chave := "taxa:ip:" + c.ClientIP()
		ok, restante, retry, err := l.Permitir(c.Request.Context(), chave, limite, janela)
		if err != nil {
			c.Next()
			return
		}
		c.Header("X-RateLimit-Remaining", strconv.Itoa(restante))
		if !ok {
			c.Header("Retry-After", strconv.Itoa(int(retry.Seconds())))
			responderProblema(c, nethttp.StatusTooManyRequests,
				i18n.T(i18n.MsgDemasiadosPedidos), i18n.T(i18n.MsgDemasiadosPedidos))
			return
		}
		c.Next()
	}
}
