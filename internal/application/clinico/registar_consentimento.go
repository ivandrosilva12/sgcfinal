package clinico

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarConsentimento regista um consentimento LPDP de um doente e audita.
type CasoRegistarConsentimento struct {
	consentimentos dominio.RepositorioConsentimentos
	doentes        dominio.RepositorioDoentes
	auditor        Auditor
	agora          func() time.Time
}

// NovoCasoRegistarConsentimento constrói o caso de uso.
func NovoCasoRegistarConsentimento(c dominio.RepositorioConsentimentos, d dominio.RepositorioDoentes, a Auditor) *CasoRegistarConsentimento {
	return &CasoRegistarConsentimento{consentimentos: c, doentes: d, auditor: a, agora: time.Now}
}

// Executar valida o doente, cria o consentimento, persiste e audita.
func (uc *CasoRegistarConsentimento) Executar(ctx context.Context, actor string, dados DadosNovoConsentimento) (DetalheConsentimento, error) {
	if _, err := uc.doentes.ObterPorID(ctx, dados.DoenteID); err != nil {
		return DetalheConsentimento{}, err
	}
	fin, err := dominio.ParseFinalidade(dados.Finalidade)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	quando := uc.agora()
	if dados.ConcedidoEm != nil {
		quando = *dados.ConcedidoEm
	}
	c, err := dominio.NovoConsentimento(dados.DoenteID, fin, dados.Concedido, dados.DocumentoURL, quando)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	id, err := uc.consentimentos.Guardar(ctx, c)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "clinico.consentimento.registado",
		Entidade: "consentimento", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheConsentimento{}, err
	}
	final, err := uc.consentimentos.ObterPorID(ctx, id)
	if err != nil {
		return DetalheConsentimento{}, err
	}
	return paraDetalheConsentimento(final), nil
}
