-- Catálogo de procedimentos cirúrgicos (dados de referência). Seed PRC001-PRC007
-- do DDM-001 v2.1 adenda §4.3.
CREATE TABLE IF NOT EXISTS clinico.catalogo_procedimentos (
    codigo               text    PRIMARY KEY,
    descricao            text    NOT NULL,
    especialidade        text,
    duracao_estimada_min integer,
    requer_anestesista   boolean NOT NULL DEFAULT false,
    activo               boolean NOT NULL DEFAULT true
);
INSERT INTO clinico.catalogo_procedimentos (codigo, descricao, especialidade, duracao_estimada_min) VALUES
    ('PRC001','Sutura de ferida superficial','CIRURGIA_GERAL',30),
    ('PRC002','Drenagem de abcesso','CIRURGIA_GERAL',45),
    ('PRC003','Exérese de lesão cutânea','DERMATOLOGIA',30),
    ('PRC004','Biópsia cutânea','DERMATOLOGIA',20),
    ('PRC005','Infiltração articular','ORTOPEDIA',20),
    ('PRC006','Extracção dentária simples','ESTOMATOLOGIA',30),
    ('PRC007','Extracção de corpo estranho ocular','OFTALMOLOGIA',20)
ON CONFLICT (codigo) DO NOTHING;
