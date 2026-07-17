package observ

import "testing"

// TestMetricas_SatisfazObservadorOutbox garante que os colectores do outbox
// (Pendentes/Publicado/FalhaHandler — ADR-038) estão registados e funcionam
// sem entrar em pânico. *Metricas satisfaz outbox.Observador por estrutura;
// este teste fica no pacote observ para não criar uma dependência inversa.
func TestMetricas_SatisfazObservadorOutbox(t *testing.T) {
	m := Novo()
	m.Pendentes(3)
	m.Publicado()
	m.FalhaHandler("clinico.episodio.fechado")
}
