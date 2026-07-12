package farmacia

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
)

// CasoRegistarMedicamento regista um medicamento no catálogo e audita.
type CasoRegistarMedicamento struct {
	repo    dominio.RepositorioMedicamentos
	auditor Auditor
	agora   func() time.Time
}

func NovoCasoRegistarMedicamento(r dominio.RepositorioMedicamentos, aud Auditor) *CasoRegistarMedicamento {
	return &CasoRegistarMedicamento{repo: r, auditor: aud, agora: time.Now}
}

func (c *CasoRegistarMedicamento) Executar(ctx context.Context, actor string, dados DadosNovoMedicamento) (DetalheMedicamento, error) {
	codigo, err := c.repo.ProximoCodigo(ctx)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	m, err := dominio.NovoMedicamento(codigo, dados.NomeComercial, dados.NomeGenerico, dados.FormaFarmaceutica, dados.Dosagem, dados.ViaAdministracao, dados.Fabricante, dados.RequerReceita, dados.Psicotropico, dados.ClasseATC, dados.StockMinimo)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	id, err := c.repo.Guardar(ctx, m)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.medicamento.registado", Entidade: "medicamento", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheMedicamento{}, err
	}
	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	return paraDetalheMedicamento(final), nil
}

// CasoActualizarMedicamento actualiza os campos de um medicamento e audita.
type CasoActualizarMedicamento struct {
	repo    dominio.RepositorioMedicamentos
	auditor Auditor
	agora   func() time.Time
}

func NovoCasoActualizarMedicamento(r dominio.RepositorioMedicamentos, aud Auditor) *CasoActualizarMedicamento {
	return &CasoActualizarMedicamento{repo: r, auditor: aud, agora: time.Now}
}

func (c *CasoActualizarMedicamento) Executar(ctx context.Context, actor, id string, dados DadosActualizarMedicamento) (DetalheMedicamento, error) {
	m, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	if err := m.Actualizar(dados.NomeComercial, dados.NomeGenerico, dados.FormaFarmaceutica, dados.Dosagem, dados.ViaAdministracao, dados.Fabricante, dados.RequerReceita, dados.Psicotropico, dados.ClasseATC, dados.StockMinimo); err != nil {
		return DetalheMedicamento{}, err
	}
	if _, err := c.repo.Guardar(ctx, m); err != nil {
		return DetalheMedicamento{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.medicamento.actualizado", Entidade: "medicamento", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheMedicamento{}, err
	}
	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	return paraDetalheMedicamento(final), nil
}

// CasoDefinirEstadoMedicamento activa/desactiva um medicamento e audita.
type CasoDefinirEstadoMedicamento struct {
	repo    dominio.RepositorioMedicamentos
	auditor Auditor
	agora   func() time.Time
}

func NovoCasoDefinirEstadoMedicamento(r dominio.RepositorioMedicamentos, aud Auditor) *CasoDefinirEstadoMedicamento {
	return &CasoDefinirEstadoMedicamento{repo: r, auditor: aud, agora: time.Now}
}

func (c *CasoDefinirEstadoMedicamento) Activar(ctx context.Context, actor, id string) (DetalheMedicamento, error) {
	return c.aplicar(ctx, actor, id, true)
}
func (c *CasoDefinirEstadoMedicamento) Desactivar(ctx context.Context, actor, id string) (DetalheMedicamento, error) {
	return c.aplicar(ctx, actor, id, false)
}
func (c *CasoDefinirEstadoMedicamento) aplicar(ctx context.Context, actor, id string, activar bool) (DetalheMedicamento, error) {
	m, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	accao := "farmacia.medicamento.desactivado"
	if activar {
		m.Activar()
		accao = "farmacia.medicamento.activado"
	} else {
		m.Desactivar()
	}
	if _, err := c.repo.Guardar(ctx, m); err != nil {
		return DetalheMedicamento{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: accao, Entidade: "medicamento", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheMedicamento{}, err
	}
	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	return paraDetalheMedicamento(final), nil
}

// CasoObterMedicamento devolve o detalhe de um medicamento (não audita — catálogo).
type CasoObterMedicamento struct {
	repo dominio.RepositorioMedicamentos
}

func NovoCasoObterMedicamento(r dominio.RepositorioMedicamentos) *CasoObterMedicamento {
	return &CasoObterMedicamento{repo: r}
}
func (c *CasoObterMedicamento) Executar(ctx context.Context, id string) (DetalheMedicamento, error) {
	m, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheMedicamento{}, err
	}
	return paraDetalheMedicamento(m), nil
}

// CasoPesquisarMedicamentos pesquisa o catálogo (não audita).
type CasoPesquisarMedicamentos struct {
	repo dominio.RepositorioMedicamentos
}

func NovoCasoPesquisarMedicamentos(r dominio.RepositorioMedicamentos) *CasoPesquisarMedicamentos {
	return &CasoPesquisarMedicamentos{repo: r}
}
func (c *CasoPesquisarMedicamentos) Executar(ctx context.Context, filtro FiltroMedicamentos) (PaginaMedicamentos, error) {
	if filtro.Limite <= 0 {
		filtro.Limite = limiteDefault
	}
	if filtro.Limite > limiteMaximo {
		filtro.Limite = limiteMaximo
	}
	if filtro.Deslocamento < 0 {
		filtro.Deslocamento = 0
	}
	return c.repo.Pesquisar(ctx, filtro)
}
