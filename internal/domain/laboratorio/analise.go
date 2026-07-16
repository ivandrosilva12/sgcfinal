package laboratorio

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/ivandrosilva12/sgcfinal/internal/domain/shared/erros"
)

// PerfilReferencia é o perfil etário a que um intervalo de referência se aplica.
type PerfilReferencia string

const (
	PerfilAdulto     PerfilReferencia = "ADULTO"
	PerfilPediatrico PerfilReferencia = "PEDIATRICO"
	PerfilGeriatrico PerfilReferencia = "GERIATRICO"
)

var perfisValidos = map[PerfilReferencia]bool{
	PerfilAdulto: true, PerfilPediatrico: true, PerfilGeriatrico: true,
}

// SexoReferencia é o sexo a que um intervalo de referência se aplica.
type SexoReferencia string

const (
	SexoMasculino SexoReferencia = "M"
	SexoFeminino  SexoReferencia = "F"
	SexoAmbos     SexoReferencia = "AMBOS"
)

var sexosValidos = map[SexoReferencia]bool{
	SexoMasculino: true, SexoFeminino: true, SexoAmbos: true,
}

// OperadorCritico é o operador de comparação de um valor crítico.
type OperadorCritico string

const (
	CriticoMenor      OperadorCritico = "<"
	CriticoMaior      OperadorCritico = ">"
	CriticoMenorIgual OperadorCritico = "<="
	CriticoMaiorIgual OperadorCritico = ">="
)

var operadoresValidos = map[OperadorCritico]bool{
	CriticoMenor: true, CriticoMaior: true, CriticoMenorIgual: true, CriticoMaiorIgual: true,
}

// IntervaloReferencia é o intervalo normal de uma análise para um perfil e sexo.
type IntervaloReferencia struct {
	Perfil PerfilReferencia `json:"perfil"`
	Sexo   SexoReferencia   `json:"sexo"`
	Minimo float64          `json:"minimo"`
	Maximo float64          `json:"maximo"`
}

// ValorCritico é uma condição que, quando satisfeita, torna o resultado crítico.
// Registado nesta fatia; avaliado na validação (Sprint 13).
type ValorCritico struct {
	Operador  OperadorCritico `json:"operador"`
	Limite    float64         `json:"limite"`
	Descricao string          `json:"descricao"`
}

// Analise é um agregado raiz do BC Laboratório: uma entrada do catálogo de análises.
// A chave é o código (não há id gerado pela BD).
type Analise struct {
	codigo     string
	nome       string
	unidade    string
	intervalos []IntervaloReferencia
	criticos   []ValorCritico
	activo     bool
	criadoEm   time.Time
}

// NovaAnalise valida e constrói uma entrada do catálogo. O código é normalizado
// para maiúsculas — é a chave por que os resultados referenciam a análise.
func NovaAnalise(codigo, nome, unidade string, intervalos []IntervaloReferencia, criticos []ValorCritico) (*Analise, error) {
	codigo = strings.ToUpper(strings.TrimSpace(codigo))
	if codigo == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "código da análise em falta")
	}
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "nome da análise em falta")
	}
	unidade = strings.TrimSpace(unidade)
	if unidade == "" {
		return nil, erros.Novo(erros.CategoriaValidacao, "unidade da análise em falta")
	}
	for _, i := range intervalos {
		if !perfisValidos[i.Perfil] {
			return nil, erros.Novo(erros.CategoriaValidacao,
				"perfil do intervalo de referência inválido (esperado ADULTO, PEDIATRICO ou GERIATRICO)")
		}
		if !sexosValidos[i.Sexo] {
			return nil, erros.Novo(erros.CategoriaValidacao,
				"sexo do intervalo de referência inválido (esperado M, F ou AMBOS)")
		}
		if i.Minimo > i.Maximo {
			return nil, erros.Novo(erros.CategoriaValidacao,
				"o mínimo do intervalo de referência não pode exceder o máximo")
		}
	}
	for _, v := range criticos {
		if !operadoresValidos[v.Operador] {
			return nil, erros.Novo(erros.CategoriaValidacao,
				"operador do valor crítico inválido (esperado <, >, <= ou >=)")
		}
		if strings.TrimSpace(v.Descricao) == "" {
			return nil, erros.Novo(erros.CategoriaValidacao, "descrição do valor crítico em falta")
		}
	}
	return &Analise{
		codigo: codigo, nome: nome, unidade: unidade,
		intervalos: intervalos, criticos: criticos, activo: true,
	}, nil
}

// AvaliarCritico indica se o valor textual satisfaz alguma das condições de valor
// crítico do catálogo. Valores não numéricos (ex.: "Positivo") nunca são críticos:
// os limiares configurados são numéricos. Avaliado na validação (Sprint 13).
func (a *Analise) AvaliarCritico(valorTexto string) bool {
	v, err := strconv.ParseFloat(strings.TrimSpace(valorTexto), 64)
	if err != nil {
		return false
	}
	for _, c := range a.criticos {
		switch c.Operador {
		case CriticoMenor:
			if v < c.Limite {
				return true
			}
		case CriticoMaior:
			if v > c.Limite {
				return true
			}
		case CriticoMenorIgual:
			if v <= c.Limite {
				return true
			}
		case CriticoMaiorIgual:
			if v >= c.Limite {
				return true
			}
		}
	}
	return false
}

// Codigo devolve o código canónico (maiúsculas).
func (a *Analise) Codigo() string { return a.codigo }

// Nome devolve o nome da análise.
func (a *Analise) Nome() string { return a.nome }

// Unidade devolve a unidade de medida.
func (a *Analise) Unidade() string { return a.unidade }

// Activo indica se a análise pode ser requisitada.
func (a *Analise) Activo() bool { return a.activo }

// SnapshotAnalise carrega o estado completo para persistência ou rehidratação.
type SnapshotAnalise struct {
	Codigo          string
	Nome            string
	Unidade         string
	Intervalos      []IntervaloReferencia
	ValoresCriticos []ValorCritico
	Activo          bool
	CriadoEm        time.Time
}

// Snapshot devolve o estado completo do agregado.
func (a *Analise) Snapshot() SnapshotAnalise {
	return SnapshotAnalise{
		Codigo: a.codigo, Nome: a.nome, Unidade: a.unidade,
		Intervalos: a.intervalos, ValoresCriticos: a.criticos,
		Activo: a.activo, CriadoEm: a.criadoEm,
	}
}

// ReconstruirAnalise reconstrói o agregado a partir de um snapshot persistido.
func ReconstruirAnalise(s SnapshotAnalise) *Analise {
	return &Analise{
		codigo: s.Codigo, nome: s.Nome, unidade: s.Unidade,
		intervalos: s.Intervalos, criticos: s.ValoresCriticos,
		activo: s.Activo, criadoEm: s.CriadoEm,
	}
}

// ResumoAnalise é a projecção de leitura do catálogo.
type ResumoAnalise struct {
	Codigo  string `json:"codigo"`
	Nome    string `json:"nome"`
	Unidade string `json:"unidade"`
	Activo  bool   `json:"activo"`
}

// RepositorioAnalises é a porta de saída de persistência do catálogo.
type RepositorioAnalises interface {
	Guardar(ctx context.Context, a *Analise) error
	ObterPorCodigo(ctx context.Context, codigo string) (*Analise, error)
	Listar(ctx context.Context) ([]ResumoAnalise, error)
}
