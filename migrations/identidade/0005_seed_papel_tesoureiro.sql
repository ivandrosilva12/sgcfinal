-- Bounded Context: identidade
-- Migration forward-only. Acrescenta o 12.º papel RBAC (Tesoureiro) ao catálogo
-- canónico. Ver docs/ERRATA-002-papel-tesoureiro.md (Marco M4 pressupõe Tesoureiro,
-- ausente dos 11 papéis do DDM-001 v2.0/ERRATA-001). Não-sensível nesta fatia.
-- Idempotente: reexecutar actualiza a descrição/sensibilidade sem duplicar.

INSERT INTO identidade.papeis (codigo, descricao, sensivel) VALUES
    ('Tesoureiro', 'Tesoureiro (facturação)', false)
ON CONFLICT (codigo) DO UPDATE
    SET descricao = EXCLUDED.descricao,
        sensivel  = EXCLUDED.sensivel;
