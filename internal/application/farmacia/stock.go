package farmacia

import (
	"context"
	"time"

	dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/farmacia"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/auditoria"
	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// CasoRegistarFornecedor regista um fornecedor e audita.
type CasoRegistarFornecedor struct {
	repo    dominio.RepositorioFornecedores
	auditor Auditor
	agora   func() time.Time
}

func NovoCasoRegistarFornecedor(r dominio.RepositorioFornecedores, aud Auditor) *CasoRegistarFornecedor {
	return &CasoRegistarFornecedor{repo: r, auditor: aud, agora: time.Now}
}
func (c *CasoRegistarFornecedor) Executar(ctx context.Context, actor string, dados DadosNovoFornecedor) (DetalheFornecedor, error) {
	f, err := dominio.NovoFornecedor(dados.Nome, dados.NIF, dados.Contacto)
	if err != nil {
		return DetalheFornecedor{}, err
	}
	id, err := c.repo.Guardar(ctx, f)
	if err != nil {
		return DetalheFornecedor{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.fornecedor.registado", Entidade: "fornecedor", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheFornecedor{}, err
	}
	final, err := c.repo.ObterPorID(ctx, id)
	if err != nil {
		return DetalheFornecedor{}, err
	}
	return paraDetalheFornecedor(final), nil
}

// CasoListarFornecedores lista fornecedores (não audita).
type CasoListarFornecedores struct {
	repo dominio.RepositorioFornecedores
}

func NovoCasoListarFornecedores(r dominio.RepositorioFornecedores) *CasoListarFornecedores {
	return &CasoListarFornecedores{repo: r}
}
func (c *CasoListarFornecedores) Executar(ctx context.Context, filtro FiltroFornecedores) (PaginaFornecedores, error) {
	if filtro.Limite <= 0 {
		filtro.Limite = limiteDefault
	}
	if filtro.Limite > limiteMaximo {
		filtro.Limite = limiteMaximo
	}
	if filtro.Deslocamento < 0 {
		filtro.Deslocamento = 0
	}
	return c.repo.Listar(ctx, filtro)
}

// CasoRegistarEntradaStock dá entrada de um lote de stock (UC-FAR-01) e audita.
type CasoRegistarEntradaStock struct {
	lotes        dominio.RepositorioLotes
	medicamentos dominio.RepositorioMedicamentos
	fornecedores dominio.RepositorioFornecedores
	auditor      Auditor
	agora        func() time.Time
}

func NovoCasoRegistarEntradaStock(lotes dominio.RepositorioLotes, medicamentos dominio.RepositorioMedicamentos, fornecedores dominio.RepositorioFornecedores, aud Auditor) *CasoRegistarEntradaStock {
	return &CasoRegistarEntradaStock{lotes: lotes, medicamentos: medicamentos, fornecedores: fornecedores, auditor: aud, agora: time.Now}
}
func (c *CasoRegistarEntradaStock) Executar(ctx context.Context, actor string, dados DadosEntradaStock) (DetalheLote, error) {
	med, err := c.medicamentos.ObterPorID(ctx, dados.MedicamentoID)
	if err != nil {
		return DetalheLote{}, err
	}
	if !med.Activo() {
		return DetalheLote{}, erros.Novo(erros.CategoriaConflito, "não é possível dar entrada de stock a um medicamento inactivo")
	}
	if dados.FornecedorID != nil && *dados.FornecedorID != "" {
		if _, err := c.fornecedores.ObterPorID(ctx, *dados.FornecedorID); err != nil {
			return DetalheLote{}, err
		}
	}
	lote, err := dominio.NovoLote(dados.MedicamentoID, dados.NumeroLote, dados.Validade, dados.Quantidade, dados.PrecoUnitarioCusto, dados.FornecedorID, dados.Notas)
	if err != nil {
		return DetalheLote{}, err
	}
	id, err := c.lotes.RegistarEntrada(ctx, lote, actor)
	if err != nil {
		return DetalheLote{}, err
	}
	if err := c.auditor.Registar(ctx, auditoria.Registo{
		Actor: actor, Accao: "farmacia.stock.entrada", Entidade: "lote", EntidadeID: id, OcorridoEm: c.agora(),
	}); err != nil {
		return DetalheLote{}, err
	}
	final, err := c.lotes.ObterPorID(ctx, id)
	if err != nil {
		return DetalheLote{}, err
	}
	return paraDetalheLote(final), nil
}

// CasoConsultarStock devolve o stock disponível de um medicamento (UC-FAR-05).
type CasoConsultarStock struct {
	lotes dominio.RepositorioLotes
}

func NovoCasoConsultarStock(lotes dominio.RepositorioLotes) *CasoConsultarStock {
	return &CasoConsultarStock{lotes: lotes}
}
func (c *CasoConsultarStock) Executar(ctx context.Context, medicamentoID string) (StockDTO, error) {
	total, err := c.lotes.StockDisponivel(ctx, medicamentoID)
	if err != nil {
		return StockDTO{}, err
	}
	return StockDTO{MedicamentoID: medicamentoID, Disponivel: total}, nil
}

// CasoListarLotes lista os lotes de um medicamento.
type CasoListarLotes struct {
	lotes dominio.RepositorioLotes
}

func NovoCasoListarLotes(lotes dominio.RepositorioLotes) *CasoListarLotes {
	return &CasoListarLotes{lotes: lotes}
}
func (c *CasoListarLotes) Executar(ctx context.Context, medicamentoID string, apenasDisponiveis bool) ([]ResumoLote, error) {
	return c.lotes.ListarPorMedicamento(ctx, medicamentoID, apenasDisponiveis)
}
