# Design — Sinais Vitais da Triagem no EHR

> **Data:** 2026-07-17
> **Estado:** Aprovado (brainstorming) — pendente de plano de implementação
> **ADR associada:** ADR-037 (a redigir no plano)
> **Marco:** Integração Percurso Ambulatório → Clínico, 2.ª fatia. Fecha o segundo e
> último diferimento da ADR-034 (o primeiro — início da consulta — fechou na ADR-036).

---

## 1. Contexto e Motivação

A Triagem (ADR-034) regista a prioridade de Manchester e os sinais vitais; o início da
consulta (ADR-036) liga a `Chegada` ao `EpisodioClinico` (`recepcao.chegadas.episodio_id`).
Mas o médico que abre o EHR ou o detalhe do episódio não vê nada da triagem — os sinais
vitais medidos minutos antes da consulta ficam presos no BC Recepção. Esta fatia expõe a
triagem na projecção clínica, **sem copiar dados e sem tocar na Recepção**.

```
Triagem (ADR-034) ──► Início da consulta (ADR-036) ──► Triagem no EHR (ESTE)
sinais vitais + cor      chegadas.episodio_id             leitura via ACL
```

## 2. Decisões de Arquitectura

- **Leitura via ACL, não snapshot.** O Clínico ganha uma porta `LeitorTriagem`; o
  adaptador junta `recepcao.chegadas ⋈ recepcao.triagens` por `episodio_id` — a coluna
  criada na ADR-036 dá a proveniência sem nenhuma migração. Fonte única de verdade na
  Recepção (correcção futura de triagem propagaria automaticamente). Rejeitado: snapshot
  na criação do episódio (duplicaria dado clínico em dois schemas, exigiria migração e
  alargaria a transacção da ADR-036).
- **Filtragem por papel na projecção (minimização LPDP).** A ADR-034 restringe a leitura
  da triagem a **Médico/Enfermeiro/Director**; a `leituraClinica` do EHR é mais larga
  (inclui Farmacêutico, Técnico de Lab, DPO, Auditor). A triagem só entra na resposta
  quando o actor tem um dos papéis autorizados — os restantes veem o EHR exactamente como
  hoje. A constante partilhada espelha a ADR-034.
- **Superfície: detalhe completo + prioridade no resumo.** `GET /episodios/:eid` ganha o
  bloco triagem completo; os resumos de episódio (EHR e listagem) ganham só a cor de
  Manchester (leve, clinicamente útil ao percorrer o histórico), preenchida por leitura
  em lote.
- **Falha do leitor propaga** (500) — não degradar silenciosamente para "sem triagem":
  dado clínico incompleto sem aviso é pior do que um erro franco.
- **Zero alterações à Recepção, zero migrações, sem eventos** (leitura pura).

### Layout

```
internal/application/clinico/ports.go                # +TriagemDoEpisodio +SinaisVitaisDTO +LeitorTriagem
internal/application/clinico/obter_episodio.go       # +LeitorTriagem +papéis do actor
internal/application/clinico/obter_ehr.go            # +LeitorTriagem +papéis do actor
internal/application/clinico/listar_episodios.go     # +prioridade no resumo (lote)
internal/adapters/pgrepo/integracao_inicio_consulta.go  # implementa LeitorTriagem (mesma peça de integração)
internal/adapters/http/episodio_handler.go           # passa SessaoDe(c).Papeis aos casos de uso
internal/platform/app.go                             # injecta o leitor
```

## 3. Porta ACL (Camada 2 — Aplicação do Clínico)

Em `internal/application/clinico/ports.go` (o Clínico nunca importa o domínio Recepção):

```go
// SinaisVitaisDTO são os sinais vitais da triagem numa resposta clínica.
// Ponteiro nil = não medido (como no VO da Recepção, sem o importar).
type SinaisVitaisDTO struct {
    TensaoSistolica        *int     `json:"tensao_sistolica,omitempty"`
    TensaoDiastolica       *int     `json:"tensao_diastolica,omitempty"`
    FrequenciaCardiaca     *int     `json:"frequencia_cardiaca,omitempty"`
    Temperatura            *float64 `json:"temperatura,omitempty"`
    FrequenciaRespiratoria *int     `json:"frequencia_respiratoria,omitempty"`
    SaturacaoO2            *int     `json:"saturacao_o2,omitempty"`
    Dor                    *int     `json:"dor,omitempty"`
    Glicemia               *int     `json:"glicemia,omitempty"`
    Peso                   *float64 `json:"peso,omitempty"`
}

// TriagemDoEpisodio é o retrato da triagem que originou um episódio — DTO da
// porta anti-corrupção.
type TriagemDoEpisodio struct {
    Prioridade   string          `json:"prioridade"`
    SinaisVitais SinaisVitaisDTO `json:"sinais_vitais"`
    Observacoes  string          `json:"observacoes,omitempty"`
    EnfermeiroID string          `json:"enfermeiro_id"`
    TriadaEm     time.Time       `json:"triada_em"`
}

// LeitorTriagem é a porta anti-corrupção para leitura da triagem no BC Recepção.
type LeitorTriagem interface {
    // TriagemDoEpisodio devolve a triagem que originou o episódio; ok=false se
    // o episódio não nasceu da fila clínica (endpoint antigo, sem chegada).
    TriagemDoEpisodio(ctx context.Context, episodioID string) (TriagemDoEpisodio, bool, error)
    // PrioridadesDosEpisodios devolve a cor de Manchester por episódio (lote,
    // para páginas de resumos); ids sem triagem ficam fora do mapa.
    PrioridadesDosEpisodios(ctx context.Context, episodioIDs []string) (map[string]string, error)
}
```

## 4. Casos de Uso — filtragem por papel

- Constante partilhada em `ports.go` (espelha a ADR-034). Os papéis chegam como
  `[]string` do handler (que converte `SessaoDe(c).Papeis`) — a aplicação do Clínico
  **não** importa o domínio Identidade, evitando uma dependência nova entre BCs na
  Camada 2:

```go
// papeisLeituraTriagem são os papéis que veem a triagem na projecção clínica
// (minimização LPDP, ADR-034): Médico, Enfermeiro e Director.
var papeisLeituraTriagem = []string{"medico", "enfermeiro", "director_clinico"}
```

  *(Os literais têm de bater com os códigos reais dos papéis do BC Identidade — o plano
  verifica os valores exactos em `internal/domain/identidade` antes de os fixar.)*

- `CasoObterEpisodio.Executar(ctx, actor string, papeis []string, id string)`:
  após carregar o episódio, se `temPapelTriagem(papeis)` → `LeitorTriagem.TriagemDoEpisodio`;
  `ok=true` → `DetalheEpisodio.Triagem` preenchido; `ok=false` → omitido; erro → propaga.
- `CasoObterEHR.Executar(ctx, actor string, papeis []string, doenteID, filtro)`:
  igual, mas em lote — `PrioridadesDosEpisodios(ids da página)` e preenchimento de
  `ResumoEpisodio.PrioridadeTriagem` por id.
- `CasoListarEpisodios` (listagem por doente, mesma vista de resumos): mesma lógica de
  lote, mesmos papéis.
- Auditoria: inalterada (as leituras já auditadas continuam; a triagem não acrescenta
  acção nova — é a mesma consulta clínica).

## 5. DTOs

- `DetalheEpisodio` ganha `Triagem *TriagemDoEpisodio` com `json:"triagem,omitempty"`.
- `ResumoEpisodio` (read-model do domínio Clínico) ganha
  `PrioridadeTriagem string` com `json:"prioridade_triagem,omitempty"` — **preenchido na
  aplicação** (mapa do lote), nunca no repositório do Clínico (que não conhece a
  Recepção). O domínio ganha só o campo passivo no struct.

## 6. Adaptador (Camada 3)

`IntegracaoInicioConsulta` (pgrepo) — a peça de integração Recepção→Clínico existente —
implementa também `LeitorTriagem` (+ asserção `var _`):

```sql
-- TriagemDoEpisodio
SELECT t.prioridade, t.tensao_sistolica, ..., COALESCE(t.observacoes,''),
       t.enfermeiro_id::text, t.triada_em
FROM recepcao.chegadas c
JOIN recepcao.triagens t ON t.chegada_id = c.id
WHERE c.episodio_id = $1;          -- 0 linhas → ok=false, sem erro

-- PrioridadesDosEpisodios
SELECT c.episodio_id::text, t.prioridade
FROM recepcao.chegadas c
JOIN recepcao.triagens t ON t.chegada_id = c.id
WHERE c.episodio_id = ANY($1::uuid[]);
```

## 7. HTTP e composition root

- Sem rotas novas; RBAC inalterado. `episodio_handler.go`: os handlers de obter
  episódio, EHR e listar episódios passam `SessaoDe(c).Papeis` (convertidos a
  `[]string`) aos casos de uso.
- `app.go`: injecta `integracaoConsulta` (que já existe) como `LeitorTriagem` nos três
  casos de uso.

## 8. Testes e Cobertura

- **Aplicação ≥75%** (fakes, incl. `fakeLeitorTriagem`): papel autorizado → bloco
  incluído; papel não autorizado (Farmacêutico) → omitido E leitor nem sequer invocado;
  episódio sem triagem (`ok=false`) → omitido sem erro; falha do leitor → propaga; lote
  do EHR/listagem preenche as prioridades certas por id e deixa os outros vazios.
- **Integração** (`//go:build integration`): episódio criado via `ConsumirEIniciar` →
  `TriagemDoEpisodio` devolve a triagem real (prioridade + sinais vitais persistidos);
  episódio do endpoint antigo → `ok=false`; lote com mistura (com e sem triagem).
- **HTTP**: Médico vê `triagem` no detalhe; Farmacêutico recebe 200 sem o campo;
  papéis da sessão passados aos casos de uso.
- Sem alterações à Recepção → suites da Recepção intactas.

## 9. ADR e Critérios de Saída

- **ADR-037 — "EHR: triagem por leitura ACL com filtragem por papel"** (a redigir como
  task do plano). Regista: leitura via ACL sobre a ponte `episodio_id` (rejeitado
  snapshot), minimização LPDP dentro da projecção (papéis da ADR-034), superfície
  detalhe-completo + prioridade-no-resumo, falha franca do leitor.

**Critérios de saída:**
1. Médico/Enfermeiro/Director veem no detalhe do episódio o bloco `triagem` (prioridade,
   sinais vitais, observações, enfermeiro, instante) quando o episódio nasceu da fila.
2. Os resumos de episódio (EHR e listagem) mostram a cor de Manchester a esses papéis.
3. Farmacêutico/Técnico de Lab/DPO/Auditor recebem as mesmas respostas de hoje (sem
   triagem) — e o leitor nem é invocado.
4. Episódios que não nasceram da fila ficam exactamente como hoje (sem bloco, sem erro).
5. Zero migrações; zero alterações ao BC Recepção; fonte única de verdade.
6. Cobertura nos limiares; integração prova a junção real por `episodio_id`.

---

## Fora de Âmbito (futuro)

- Evento de integração / Outbox (próxima fatia — ADR-038).
- Re-triagem/correcção da triagem (continua imutável).
- Tendências/histórico de sinais vitais no EHR (só a triagem de origem do episódio).
- Sinais vitais medidos DURANTE a consulta (pertenceriam ao EHR do episódio, não à
  Recepção — módulo futuro).
