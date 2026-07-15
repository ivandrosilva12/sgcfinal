# Design — BC Recepção: Triagem e Fila Clínica

> **Data:** 2026-07-15
> **Estado:** Aprovado (brainstorming) — pendente de plano de implementação
> **ADR associada:** ADR-034 (a redigir no plano)
> **Marco:** Percurso Ambulatório. **Terceiro e último sub-projecto — Triagem.** Sucede
> à Marcação (ADR-032) e ao Check-in (ADR-033). Fecha o marco.

---

## 1. Contexto e Motivação

O Check-in (ADR-033) coloca o doente na fila e permite chamá-lo (`Chegada` `CHAMADO`).
A Triagem é a etapa clínica seguinte: um enfermeiro avalia o doente chamado, classifica a
prioridade (Sistema de Manchester) e regista os sinais vitais. O resultado ordena uma
**fila clínica** por prioridade, a partir da qual o médico atende. É também na triagem que
o **walk-in** (que chegou sem médico) recebe um médico atribuído.

```
Marcação → Check-in → Triagem (ESTE) → Consulta/Episódio (BC Clínico, fora de âmbito)
(ADR-032)  (ADR-033)   (prioridade + sinais vitais → fila clínica)
```

## 2. Decisões de Arquitectura

- **Estende o BC `recepcao`** (sem novo BC). Novo agregado `Triagem` + dois VOs
  (`PrioridadeManchester`, `SinaisVitais`) + extensão pontual à `Chegada`. Reutiliza os
  padrões compare-and-set (CAS), auditoria e coordenação transaccional já no BC.
- **A triagem é um registo clínico próprio** (agregado `Triagem`, 1:1 com a chegada),
  separado da `Chegada` (entidade de fila). É **imutável** após criação — não tem máquina
  de estados.
- **O registo de triagem é transaccional e cruza dois agregados:** transita a `Chegada`
  `CHAMADO→TRIADO` (e atribui o médico ao walk-in) **e** insere a `Triagem`, numa única
  transacção com guarda CAS sobre o estado da chegada.
- **Prioridade pelo Sistema de Manchester** (5 cores com tempos-alvo) — é o modelo de
  domínio, não configuração.
- **Leitura clínica restrita** (Médico/Enfermeiro/Director; sem Administrativo/Admin),
  porque os sinais vitais e a prioridade derivada são dado clínico (minimização LPDP, como
  no Laboratório).
- **Sem FK cross-context.** `enfermeiro_id`/`medico_id` são uuid sem FK. A única FK é
  interna: `triagens.chegada_id → recepcao.chegadas(id)`.

### Layout

```
internal/domain/recepcao/prioridade.go        # VO PrioridadeManchester
internal/domain/recepcao/sinais_vitais.go     # VO SinaisVitais
internal/domain/recepcao/triagem.go           # agregado Triagem + RepositorioTriagens + ResumoFilaClinica
internal/domain/recepcao/chegada.go           # +estado TRIADO +RegistarTriada
internal/application/recepcao/triagens.go     # casos de uso
internal/application/recepcao/ports.go        # +DTOs de triagem
internal/adapters/pgrepo/triagens_repo.go     # repositório pgx
internal/adapters/http/recepcao_triagem_handler.go  # 3.º handler (separado)
migrations/recepcao/0003_triagens.sql
```

## 3. Modelo de Domínio

### 3.1 VO `PrioridadeManchester`

```
type PrioridadeManchester string
const (
    ManVermelho PrioridadeManchester = "VERMELHO"  // Emergente   — 0 min
    ManLaranja  PrioridadeManchester = "LARANJA"   // Muito urgente — 10 min
    ManAmarelo  PrioridadeManchester = "AMARELO"   // Urgente     — 60 min
    ManVerde    PrioridadeManchester = "VERDE"     // Pouco urgente — 120 min
    ManAzul     PrioridadeManchester = "AZUL"      // Não urgente — 240 min
)
```

- `Severidade() int` — 1 (VERMELHO) a 5 (AZUL); menor = mais urgente. Ordena a fila clínica.
- `TempoAlvo() time.Duration` — 0/10/60/120/240 minutos.
- `ParsePrioridade(codigo string) (PrioridadeManchester, error)` — valida/normaliza (aceita minúsculas); inválido → `CategoriaValidacao`.

### 3.2 VO `SinaisVitais`

Todos os campos **opcionais** (ponteiro nil = não medido). O construtor valida os
intervalos plausíveis (limites de sanidade que rejeitam erros de digitação — não são
intervalos de normalidade clínica):

| Campo | Tipo | Intervalo plausível | Unidade |
|---|---|---|---|
| TensaoSistolica | `*int` | 50–300 | mmHg |
| TensaoDiastolica | `*int` | 30–200 | mmHg |
| FrequenciaCardiaca | `*int` | 20–300 | bpm |
| Temperatura | `*float64` | 30–45 | °C |
| FrequenciaRespiratoria | `*int` | 5–80 | cpm |
| SaturacaoO2 | `*int` | 50–100 | % |
| Dor | `*int` | 0–10 | escala |
| Glicemia | `*int` | 20–600 | mg/dL |
| Peso | `*float64` | 0,5–400 | kg |

`NovosSinaisVitais(...) (SinaisVitais, error)` — valida cada campo presente; um valor fora
do intervalo → `CategoriaValidacao`. Um conjunto totalmente vazio é válido (a prioridade
pode ser clínica sem todos os vitais medidos).

### 3.3 Agregado `Triagem` (raiz)

Registo clínico imutável de uma triagem, 1:1 com uma chegada.

**Campos:** `id`, `chegadaID`, `prioridade` (Manchester), `sinaisVitais` (VO),
`observacoes` (livre, opcional), `enfermeiroID` (o triador), `triadaEm`, `criadoEm`.

`NovaTriagem(chegadaID, enfermeiroID string, prioridade PrioridadeManchester, sinais SinaisVitais, observacoes string, em time.Time) (*Triagem, error)`
— valida `chegadaID`/`enfermeiroID` não-vazios, `prioridade` válida e `em` não-zero. Sem
métodos de transição (imutável). Getters + `SnapshotTriagem`/`Snapshot()`/`ReconstruirTriagem`.

`enfermeiroID` é o sujeito autenticado (na aplicação), nunca do corpo.

### 3.4 Extensão de `Chegada`

- Novo estado `ChegTriado = "TRIADO"` (acrescentado ao enum `EstadoChegada`).
- Novo método `RegistarTriada(medicoID string, em time.Time) error` (`CHAMADO→TRIADO`):
  - só de `CHAMADO` (senão `CategoriaConflito`);
  - se a chegada **não tem médico** (walk-in): `medicoID` é obrigatório (senão
    `CategoriaValidacao`) e é gravado na chegada;
  - se a chegada **já tem médico** (agendada): `medicoID` tem de vir vazio (senão
    `CategoriaValidacao`, "a chegada já tem médico atribuído") — herda o da marcação;
  - transita para `TRIADO`.

### 3.5 Coordenação do registo de triagem

Cruza dois agregados numa transacção (padrão do check-in):
1. o caso de uso obtém a chegada (`CHAMADO`);
2. `chegada.RegistarTriada(medicoID, agora)` (valida estado + atribui médico ao walk-in);
3. constrói a `Triagem` (enfermeiro = actor);
4. `RepositorioTriagens.RegistarTriagem(ctx, triagem, chegada)`:
   `UPDATE recepcao.chegadas SET estado='TRIADO', medico_id=…, actualizado_em=$ WHERE id=$
   AND estado='CHAMADO'` (0 linhas → 404 se não existe, 409 se já não está `CHAMADO`)
   seguido do `INSERT` da triagem, na mesma `tx`.

Defesa em profundidade: `UNIQUE` sobre `triagens.chegada_id` (uma triagem por chegada) —
uma violação (`23505`) é traduzida para `CategoriaConflito`.

## 4. Camada de Aplicação

### 4.1 Casos de Uso

| Caso de uso | Comportamento | Acção de auditoria |
|---|---|---|
| `NovoCasoRegistarTriagem` | Obtém a chegada; `RegistarTriada`; cria `Triagem` (enfermeiro = actor); persiste na transacção coordenada. | `recepcao.triagem.registada` |
| `NovoCasoObterTriagem` | Lê a triagem de uma chegada. | — (leitura) |
| `NovoCasoListarFilaClinica` | Lê a fila clínica (TRIADO por prioridade Manchester), filtrável por médico. | — (leitura) |

**Actor** = `SessaoDe(ctx).Sujeito` (o enfermeiro triador), nunca do corpo.

### 4.2 Ports

Reutiliza `Auditor`. DTOs novos em `ports.go`:
- `DadosTriagem{ Prioridade string; TensaoSistolica *int; … (os 9 campos); Observacoes string; MedicoID string }`.
- `DetalheTriagem`, reexport `ResumoFilaClinica`.

Nova interface de domínio em `triagem.go`:

```
type ResumoFilaClinica struct {
    ChegadaID, TriagemID, DoenteID, MedicoID, EspecialidadeID, Prioridade string
    HoraChegada, TriadaEm time.Time
}
type RepositorioTriagens interface {
    RegistarTriagem(ctx, triagem *Triagem, chegada *Chegada) (string, error) // transaccional
    ObterPorChegada(ctx, chegadaID string) (*Triagem, error)
    ListarFilaClinica(ctx, medicoID string) ([]ResumoFilaClinica, error)     // TRIADO, ord. Manchester
}
```

## 5. Adaptadores

### 5.1 HTTP — 3.º handler `recepcao_triagem_handler.go`

Handler separado (mantém os construtores enxutos, como o do check-in). Grupos RBAC:
`triagemEscrita = {Enfermeiro, Medico}`, `leituraClinica = {Medico, Enfermeiro, Director}`.

| Método + rota | Caso de uso | RBAC |
|---|---|---|
| `POST /api/v1/chegadas/:cid/triagem` | RegistarTriagem | triagemEscrita |
| `GET /api/v1/chegadas/:cid/triagem` | ObterTriagem | leituraClinica |
| `GET /api/v1/recepcao/fila-clinica?medico=` | ListarFilaClinica | leituraClinica |

Actor de `SessaoDe(c).Sujeito`. Corpo malformado → 400 (`CategoriaValidacao` + `i18n.MsgPedidoInvalido`).

### 5.2 Persistência — `migrations/recepcao/0003_triagens.sql` (forward-only)

```sql
-- Estende o enum de estado da chegada com TRIADO.
ALTER TABLE recepcao.chegadas DROP CONSTRAINT chegadas_estado_check;
ALTER TABLE recepcao.chegadas ADD CONSTRAINT chegadas_estado_check
    CHECK (estado IN ('AGUARDA','CHAMADO','DESISTIU','TRIADO'));

CREATE TABLE IF NOT EXISTS recepcao.triagens (
    id                       uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    chegada_id               uuid        NOT NULL REFERENCES recepcao.chegadas(id),
    enfermeiro_id            uuid        NOT NULL,
    prioridade               text        NOT NULL CHECK (prioridade IN
                               ('VERMELHO','LARANJA','AMARELO','VERDE','AZUL')),
    tensao_sistolica         int         CHECK (tensao_sistolica       BETWEEN 50 AND 300),
    tensao_diastolica        int         CHECK (tensao_diastolica      BETWEEN 30 AND 200),
    frequencia_cardiaca      int         CHECK (frequencia_cardiaca    BETWEEN 20 AND 300),
    temperatura              numeric(4,1) CHECK (temperatura           BETWEEN 30 AND 45),
    frequencia_respiratoria  int         CHECK (frequencia_respiratoria BETWEEN 5 AND 80),
    saturacao_o2             int         CHECK (saturacao_o2           BETWEEN 50 AND 100),
    dor                      int         CHECK (dor                    BETWEEN 0 AND 10),
    glicemia                 int         CHECK (glicemia               BETWEEN 20 AND 600),
    peso                     numeric(5,1) CHECK (peso                  BETWEEN 0.5 AND 400),
    observacoes              text,
    triada_em                timestamptz NOT NULL,
    criado_em                timestamptz NOT NULL DEFAULT now(),
    -- Uma triagem por chegada (o registo duplicado é negado também pela guarda CAS do
    -- domínio; a BD fecha a corrida concorrente).
    UNIQUE (chegada_id)
);
```

As colunas de sinais vitais são `NULL` quando não medidas; as CHECK só se aplicam a
valores presentes (SQL: `CHECK` sobre `NULL` é `UNKNOWN` → não falha). A ordenação
Manchester da fila clínica faz-se por `CASE prioridade` na query, sem coluna extra.

`chegada_id` é FK interna ao schema `recepcao`. `enfermeiro_id`/`medico_id` são uuid nu.

## 6. Erros e Auditoria

Categorias existentes: `CategoriaConflito` (409, chegada não em CHAMADO / triagem
duplicada), `CategoriaValidacao` (400, sinais vitais fora de intervalo, médico em falta no
walk-in, médico indevido no agendado, prioridade inválida), `CategoriaNaoEncontrado` (404).
O comando `RegistarTriagem` é auditado (append-only, 10 anos); as leituras não.

## 7. Testes e Cobertura

- **Domínio ≥85%:** `ParsePrioridade`/`Severidade`/`TempoAlvo` (as 5 cores); `SinaisVitais`
  (cada intervalo nos limites e fora, presente e ausente, conjunto vazio válido); `Triagem`
  (construção, obrigatórios); `Chegada.RegistarTriada` (walk-in exige médico; agendada
  recusa médico indevido; conflito a partir de estado ≠ CHAMADO).
- **Aplicação ≥75%:** fakes; a coordenação (transita `TRIADO` + cria triagem + atribui
  médico ao walk-in); triagem duplicada → 409; enfermeiro = actor; ordenação da fila
  clínica por severidade (VERMELHO antes de AZUL).
- **Adaptadores ≥60%:** teste HTTP com duplos (RBAC das 3 rotas; enfermeiro da sessão;
  corpo malformado → 400); integração `//go:build integration` contra Postgres real —
  registo transaccional (chegada `TRIADO` + médico atribuído + triagem, ou nada em caso de
  falha), o `UNIQUE` a negar o duplicado, as CHECK de intervalo, e a fila clínica ordenada
  por severidade Manchester.

## 8. ADR e Critérios de Saída

- **ADR-034 — "BC Recepção — Triagem e fila clínica"** (próximo número livre; a redigir
  como task do plano). Regista: agregado `Triagem` imutável, VOs Manchester/SinaisVitais,
  `TRIADO` na chegada com atribuição de médico ao walk-in, coordenação transaccional com CAS
  + `UNIQUE`, leitura clínica restrita (LPDP).

**Critérios de saída (fecha o marco Percurso Ambulatório):**
1. Registar triagem (prioridade Manchester + sinais vitais validados) transita a chegada
   para `TRIADO` **e** atribui o médico ao walk-in, atomicamente.
2. A triagem é imutável e única por chegada (agregado + BD).
3. Fila clínica consultável, ordenada por severidade Manchester e hora de chegada,
   filtrável por médico.
4. Registo de triagem duplicado negado pelo agregado (guarda CAS) **e** pela BD (`UNIQUE`).
5. Leitura clínica restrita a Médico/Enfermeiro/Director; o registo é do Enfermeiro/Médico.
6. Sem FK cross-context; o enfermeiro triador é o sujeito autenticado.
7. Comando auditado; cobertura nos limiares.

---

## Fora de Âmbito (futuro)

- **Início da consulta:** consumir a `Chegada` `TRIADO` → criar o `EpisodioClinico` no BC
  Clínico (integração cross-context, marco futuro).
- Ligar os sinais vitais da triagem ao EHR do episódio.
- Re-triagem / correcção de uma triagem (a triagem é imutável neste marco).
- Escala de dor por discriminadores de Manchester / fluxogramas de decisão (a prioridade é
  introduzida pelo enfermeiro, não derivada automaticamente).
