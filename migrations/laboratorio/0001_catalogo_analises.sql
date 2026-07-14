-- Bounded Context: laboratorio
-- Migration forward-only. Catálogo de análises (dados de referência).
--
-- Os intervalos de referência e os valores críticos são jsonb: são listas de VOs
-- lidas em bloco com o agregado e nunca consultadas isoladamente por SQL, pelo que
-- tabelas-filho só acrescentariam junções sem benefício. Os valores críticos são
-- registados nesta fatia e avaliados no Sprint 13.

CREATE SCHEMA IF NOT EXISTS laboratorio;

CREATE TABLE IF NOT EXISTS laboratorio.analises (
    codigo                text        PRIMARY KEY,
    nome                  text        NOT NULL,
    unidade               text        NOT NULL,
    intervalos_referencia jsonb       NOT NULL DEFAULT '[]'::jsonb,
    valores_criticos      jsonb       NOT NULL DEFAULT '[]'::jsonb,
    activo                boolean     NOT NULL DEFAULT true,
    criado_em             timestamptz NOT NULL DEFAULT now()
);

INSERT INTO laboratorio.analises (codigo, nome, unidade, intervalos_referencia, valores_criticos) VALUES
    ('HB', 'Hemoglobina', 'g/dL',
     '[{"perfil":"ADULTO","sexo":"M","minimo":13.0,"maximo":17.0},
       {"perfil":"ADULTO","sexo":"F","minimo":12.0,"maximo":15.0}]'::jsonb,
     '[{"operador":"<","limite":7.0,"descricao":"anemia grave — contactar o médico requisitante"}]'::jsonb),
    ('GLIC', 'Glicemia em jejum', 'mg/dL',
     '[{"perfil":"ADULTO","sexo":"AMBOS","minimo":70.0,"maximo":110.0}]'::jsonb,
     '[{"operador":"<","limite":50.0,"descricao":"hipoglicemia grave"},
       {"operador":">","limite":400.0,"descricao":"hiperglicemia grave"}]'::jsonb),
    ('CREAT', 'Creatinina sérica', 'mg/dL',
     '[{"perfil":"ADULTO","sexo":"M","minimo":0.7,"maximo":1.3},
       {"perfil":"ADULTO","sexo":"F","minimo":0.6,"maximo":1.1}]'::jsonb,
     '[{"operador":">","limite":5.0,"descricao":"insuficiência renal — contactar o médico"}]'::jsonb),
    ('UREIA', 'Ureia', 'mg/dL',
     '[{"perfil":"ADULTO","sexo":"AMBOS","minimo":15.0,"maximo":45.0}]'::jsonb,
     '[]'::jsonb),
    ('HEMOG', 'Hemograma completo', 'texto',
     '[]'::jsonb, '[]'::jsonb)
ON CONFLICT (codigo) DO NOTHING;

COMMENT ON TABLE laboratorio.analises IS
    'Catálogo de análises. valores_criticos é avaliado na validação (Sprint 13).';
