// Package observ concentra a observabilidade da API: métricas Prometheus e o
// middleware Gin que as alimenta. Usa um registry próprio (não o global) para
// isolamento e testabilidade. Camada 4 — Plataforma.
package observ

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metricas agrupa o registry e os coletores HTTP e do outbox da API. Os
// colectores do outbox (Pendentes/Publicado/FalhaHandler) satisfazem
// outbox.Observador por estrutura — este pacote não importa o outbox (a regra
// de dependência confina o Prometheus à Plataforma).
type Metricas struct {
	registry         *prometheus.Registry
	pedidosTotal     *prometheus.CounterVec
	duracao          *prometheus.HistogramVec
	outboxPendentes  prometheus.Gauge
	outboxPublicados prometheus.Counter
	outboxFalhas     *prometheus.CounterVec
}

// Novo constrói as métricas e regista os coletores base (Go runtime + processo)
// e os coletores HTTP.
func Novo() *Metricas {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	pedidosTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sgc_http_pedidos_total",
			Help: "Total de pedidos HTTP por método, rota e código.",
		},
		[]string{"metodo", "rota", "codigo"},
	)
	duracao := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "sgc_http_duracao_segundos",
			Help: "Duração dos pedidos HTTP em segundos (alvo P95 CRUD < 0,5s).",
			// Buckets ajustados ao alvo de 500ms.
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"metodo", "rota"},
	)
	reg.MustRegister(pedidosTotal, duracao)

	outboxPendentes := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "sgc_outbox_pendentes",
		Help: "Eventos de outbox por publicar.",
	})
	outboxPublicados := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "sgc_outbox_publicados_total",
		Help: "Eventos de outbox publicados.",
	})
	outboxFalhas := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sgc_outbox_falhas_handler_total",
			Help: "Falhas de handler por tipo de evento.",
		},
		[]string{"tipo_evento"},
	)
	reg.MustRegister(outboxPendentes, outboxPublicados, outboxFalhas)

	return &Metricas{
		registry:         reg,
		pedidosTotal:     pedidosTotal,
		duracao:          duracao,
		outboxPendentes:  outboxPendentes,
		outboxPublicados: outboxPublicados,
		outboxFalhas:     outboxFalhas,
	}
}

// Pendentes regista o número de eventos de outbox por publicar (satisfaz
// outbox.Observador por estrutura).
func (m *Metricas) Pendentes(n int) { m.outboxPendentes.Set(float64(n)) }

// Publicado incrementa o total de eventos de outbox publicados com sucesso.
func (m *Metricas) Publicado() { m.outboxPublicados.Inc() }

// FalhaHandler incrementa o total de falhas de handler por tipo de evento.
func (m *Metricas) FalhaHandler(tipo string) { m.outboxFalhas.WithLabelValues(tipo).Inc() }

// Middleware devolve o middleware Gin que mede cada pedido. Usa a rota
// registada (FullPath) como etiqueta, evitando explosão de cardinalidade por
// parâmetros de caminho.
func (m *Metricas) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		inicio := time.Now()
		c.Next()

		rota := c.FullPath()
		if rota == "" {
			rota = "desconhecida"
		}
		m.duracao.WithLabelValues(c.Request.Method, rota).Observe(time.Since(inicio).Seconds())
		m.pedidosTotal.WithLabelValues(c.Request.Method, rota, strconv.Itoa(c.Writer.Status())).Inc()
	}
}

// Handler devolve o http.Handler que expõe as métricas em /metrics.
func (m *Metricas) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
