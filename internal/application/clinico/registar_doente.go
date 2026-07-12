package clinico

import (
	"context"
	"strings"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarDoente regista um novo doente (com número de processo automático
// ou manual) e audita a operação.
type CasoRegistarDoente struct {
	repo    dominio.RepositorioDoentes
	auditor Auditor
	agora   func() time.Time
}

// NovoCasoRegistarDoente constrói o caso de uso.
func NovoCasoRegistarDoente(repo dominio.RepositorioDoentes, aud Auditor) *CasoRegistarDoente {
	return &CasoRegistarDoente{repo: repo, auditor: aud, agora: time.Now}
}

// Executar valida os dados, resolve o número de processo, persiste o doente,
// audita e devolve o detalhe.
func (c *CasoRegistarDoente) Executar(ctx context.Context, actor string, dados DadosNovoDoente) (DetalheDoente, error) {
	ident, err := construirIdentificacao(dados.Identificacao)
	if err != nil {
		return DetalheDoente{}, err
	}
	contactos, err := construirContactos(dados.Contactos)
	if err != nil {
		return DetalheDoente{}, err
	}

	numProcesso := strings.TrimSpace(dados.NumProcesso)
	if numProcesso == "" {
		gerado, err := c.repo.ProximoNumeroProcesso(ctx, c.agora().Year())
		if err != nil {
			return DetalheDoente{}, err
		}
		numProcesso = gerado
	}

	doente, err := dominio.NovoDoente(numProcesso, ident, contactos, dados.Nacionalidade)
	if err != nil {
		return DetalheDoente{}, err
	}
	if dados.GrupoSanguineo != nil {
		if g, err := grupoOpcional(*dados.GrupoSanguineo); err != nil {
			return DetalheDoente{}, err
		} else {
			doente.DefinirGrupoSanguineo(g)
		}
	}

	id, err := c.repo.Guardar(ctx, doente)
	if err != nil {
		return DetalheDoente{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor:      actor,
		Accao:      "clinico.doente.registado",
		Entidade:   "doente",
		EntidadeID: id,
		OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheDoente{}, err
	}

	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheDoente{}, err
	}
	return paraDetalhe(final), nil
}

// grupoOpcional converte uma string em *GrupoSanguineo; string vazia → nil.
func grupoOpcional(valor string) (*dominio.GrupoSanguineo, error) {
	if strings.TrimSpace(valor) == "" {
		return nil, nil
	}
	g, err := dominio.ParseGrupoSanguineo(valor)
	if err != nil {
		return nil, err
	}
	return &g, nil
}
