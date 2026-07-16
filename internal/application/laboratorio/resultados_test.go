package laboratorio_test

import (
	"testing"

	applaboratorio "github.com/ivandrosilva12/sgcfinal/internal/application/laboratorio"
	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/laboratorio"
)

// TestEstadosVisiveisAoMedico_SoValidada fixa a regra de visibilidade do Sprint 13:
// o médico vê o resultado vigente (VALIDADA) e não o preliminar (PROCESSADA) nem o
// arquivado por correcção (CONCLUIDA).
func TestEstadosVisiveisAoMedico_SoValidada(t *testing.T) {
	vis := applaboratorio.EstadosVisiveisAoMedico
	if len(vis) != 1 || vis[0] != dominio.ResValidada {
		t.Fatalf("a leitura clínica deve mostrar só VALIDADA, veio %+v", vis)
	}
}
