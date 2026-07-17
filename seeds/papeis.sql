-- Seed dos 12 papéis RBAC do SGC Angola (fonte: DDM-001 v2.0).
-- Ver docs/ERRATA-001-papeis.md (divergência 8 vs 11, resolvida a favor do DDM-001).
-- Idempotente: reexecutar não duplica nem altera atribuições existentes.
-- Papéis sensíveis (sensivel=true) exigirão MFA em M1/Sprint 3.

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
    ('Auditor',            'Auditor',                      true),
    ('Tesoureiro',         'Tesoureiro (facturação)',      false)
ON CONFLICT (codigo) DO UPDATE
    SET descricao = EXCLUDED.descricao,
        sensivel  = EXCLUDED.sensivel;
