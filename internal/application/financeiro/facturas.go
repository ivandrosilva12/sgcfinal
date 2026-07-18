package financeiro

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/financeiro"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/moeda"
)

// CasoCriarFactura cria uma factura em rascunho.
type CasoCriarFactura struct {
	facturas dominio.RepositorioFacturas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoCriarFactura constrói o caso de uso.
func NovoCasoCriarFactura(f dominio.RepositorioFacturas, aud Auditor) *CasoCriarFactura {
	return &CasoCriarFactura{facturas: f, auditor: aud, agora: time.Now}
}

// Executar valida, persiste e audita a criação da factura.
func (uc *CasoCriarFactura) Executar(ctx context.Context, actor string, d DadosNovaFactura) (DetalheFactura, error) {
	cliente, err := dominio.NovoClienteSnapshot(d.ClienteNome, d.ClienteNIF, d.ClienteMorada)
	if err != nil {
		return DetalheFactura{}, err
	}
	f, err := dominio.NovaFactura(cliente, d.EpisodioID)
	if err != nil {
		return DetalheFactura{}, err
	}
	id, err := uc.facturas.Guardar(ctx, f)
	if err != nil {
		return DetalheFactura{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "financeiro.factura.criada",
		Entidade: "factura", EntidadeID: id, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheFactura{}, err
	}
	return uc.obter(ctx, id)
}

func (uc *CasoCriarFactura) obter(ctx context.Context, id string) (DetalheFactura, error) {
	f, err := uc.facturas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheFactura{}, err
	}
	return paraDetalheFactura(f), nil
}

// CasoAdicionarItem acrescenta uma linha a uma factura em rascunho.
type CasoAdicionarItem struct {
	facturas dominio.RepositorioFacturas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoAdicionarItem constrói o caso de uso.
func NovoCasoAdicionarItem(f dominio.RepositorioFacturas, aud Auditor) *CasoAdicionarItem {
	return &CasoAdicionarItem{facturas: f, auditor: aud, agora: time.Now}
}

// Executar carrega a factura, acrescenta a linha, persiste e audita.
func (uc *CasoAdicionarItem) Executar(ctx context.Context, actor string, d DadosNovoItem) (DetalheFactura, error) {
	f, err := uc.facturas.ObterPorID(ctx, d.FacturaID)
	if err != nil {
		return DetalheFactura{}, err
	}
	if err := f.AdicionarItem(d.Descricao, dominio.TipoLinha(d.Tipo), d.OperacaoID,
		d.Quantidade, moeda.DeCentimos(d.PrecoUnitarioCentimos), dominio.RegimeIVA(d.RegimeIVA)); err != nil {
		return DetalheFactura{}, err
	}
	if _, err := uc.facturas.Guardar(ctx, f); err != nil {
		return DetalheFactura{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "financeiro.factura.item.adicionado",
		Entidade: "factura", EntidadeID: d.FacturaID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheFactura{}, err
	}
	recarregada, err := uc.facturas.ObterPorID(ctx, d.FacturaID)
	if err != nil {
		return DetalheFactura{}, err
	}
	return paraDetalheFactura(recarregada), nil
}

// CasoRemoverItem retira uma linha de uma factura em rascunho.
type CasoRemoverItem struct {
	facturas dominio.RepositorioFacturas
	auditor  Auditor
	agora    func() time.Time
}

// NovoCasoRemoverItem constrói o caso de uso.
func NovoCasoRemoverItem(f dominio.RepositorioFacturas, aud Auditor) *CasoRemoverItem {
	return &CasoRemoverItem{facturas: f, auditor: aud, agora: time.Now}
}

// Executar carrega a factura, remove a linha, persiste e audita.
func (uc *CasoRemoverItem) Executar(ctx context.Context, actor, facturaID, itemID string) (DetalheFactura, error) {
	f, err := uc.facturas.ObterPorID(ctx, facturaID)
	if err != nil {
		return DetalheFactura{}, err
	}
	if err := f.RemoverItem(itemID); err != nil {
		return DetalheFactura{}, err
	}
	if _, err := uc.facturas.Guardar(ctx, f); err != nil {
		return DetalheFactura{}, err
	}
	if err := uc.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "financeiro.factura.item.removido",
		Entidade: "factura", EntidadeID: facturaID, OcorridoEm: uc.agora(),
	}); err != nil {
		return DetalheFactura{}, err
	}
	recarregada, err := uc.facturas.ObterPorID(ctx, facturaID)
	if err != nil {
		return DetalheFactura{}, err
	}
	return paraDetalheFactura(recarregada), nil
}

// CasoObterFactura devolve o detalhe de uma factura.
type CasoObterFactura struct {
	facturas dominio.RepositorioFacturas
}

// NovoCasoObterFactura constrói o caso de uso.
func NovoCasoObterFactura(f dominio.RepositorioFacturas) *CasoObterFactura {
	return &CasoObterFactura{facturas: f}
}

// Executar devolve o detalhe da factura.
func (uc *CasoObterFactura) Executar(ctx context.Context, id string) (DetalheFactura, error) {
	f, err := uc.facturas.ObterPorID(ctx, id)
	if err != nil {
		return DetalheFactura{}, err
	}
	return paraDetalheFactura(f), nil
}

// CasoListarFacturasPorEpisodio lista as facturas de um episódio.
type CasoListarFacturasPorEpisodio struct {
	facturas dominio.RepositorioFacturas
}

// NovoCasoListarFacturasPorEpisodio constrói o caso de uso.
func NovoCasoListarFacturasPorEpisodio(f dominio.RepositorioFacturas) *CasoListarFacturasPorEpisodio {
	return &CasoListarFacturasPorEpisodio{facturas: f}
}

// Executar devolve os resumos das facturas do episódio.
func (uc *CasoListarFacturasPorEpisodio) Executar(ctx context.Context, episodioID string) ([]ResumoFactura, error) {
	return uc.facturas.ListarPorEpisodio(ctx, episodioID)
}
