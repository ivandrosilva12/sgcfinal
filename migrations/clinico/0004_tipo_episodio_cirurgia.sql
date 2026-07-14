-- Estende a CHECK do tipo de episódio para incluir CIRURGIA_AMBULATORIA (ADR-018 pt2).
ALTER TABLE clinico.episodios_clinicos DROP CONSTRAINT IF EXISTS episodios_clinicos_tipo_check;
ALTER TABLE clinico.episodios_clinicos ADD CONSTRAINT episodios_clinicos_tipo_check
    CHECK (tipo IN ('CONSULTA','URGENCIA','INTERNAMENTO','CIRURGIA_AMBULATORIA'));
