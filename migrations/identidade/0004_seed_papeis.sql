-- Bounded Context: identidade
-- Migration forward-only. Semeia o catálogo canónico de papéis RBAC (dados de
-- referência, não dados de exemplo): os 11 papéis do DDM-001 v2.0 têm de existir
-- para que a atribuição de papéis (utilizadores_papeis → papeis via FK) funcione,
-- nomeadamente no provisionamento JIT do BC Identidade (Sprint 2).
--
-- Fonte idêntica a seeds/papeis.sql. Idempotente: reexecutar actualiza a
-- descrição/sensibilidade sem duplicar.

INSERT INTO identidade.papeis (codigo, descricao, sensivel) VALUES
    ('Medico',             'Médico',                       false),
    ('Enfermeiro',         'Enfermeiro',                   false),
    ('Administrativo',     'Administrativo (recepção/financeiro)', false),
    ('Farmaceutico',       'Farmacêutico',                 false),
    ('FarmaceuticoSenior', 'Farmacêutico Sénior',          false),
    ('TecnicoLab',         'Técnico de Laboratório',       false),
    ('Patologista',        'Patologista',                  false),
    ('Director',           'Director Clínico',             true),
    ('Admin',              'Administrador de Sistema',     true),
    ('DPO',                'Encarregado de Protecção de Dados (DPO)', true),
    ('Auditor',            'Auditor',                      true)
ON CONFLICT (codigo) DO UPDATE
    SET descricao = EXCLUDED.descricao,
        sensivel  = EXCLUDED.sensivel;
