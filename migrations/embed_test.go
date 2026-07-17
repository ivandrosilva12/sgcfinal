package migrations_test

import (
	"io/fs"
	"testing"

	"github.com/ivandrosilva12/sgcfinal/migrations"
)

// Garante que as migrations foram efectivamente embebidas no binário (o embed é
// resolvido em tempo de compilação; este teste confirma a presença em runtime).
func TestFS_ContemMigrationsEsperadas(t *testing.T) {
	esperadas := []string{
		"auditoria/0001_auditoria_eventos.sql",
		"clinico/0002_episodios.sql",
		"farmacia/0001_medicamentos_receitas.sql",
		"farmacia/0002_stock.sql",
		"identidade/0001_utilizadores.sql",
		"identidade/0002_utilizadores_papeis.sql",
		"identidade/0003_papeis.sql",
		"recepcao/0005_chegadas_atendido.sql",
		"shared/0001_outbox.sql",
		"shared/0002_outbox_tentativas.sql",
	}
	for _, caminho := range esperadas {
		if _, err := fs.Stat(migrations.FS, caminho); err != nil {
			t.Errorf("migration em falta na FS embebida: %s (%v)", caminho, err)
		}
	}
}
