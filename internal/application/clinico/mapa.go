package clinico

import dominio "github.com/ivandrosilva12/sgcfinal/internal/domain/clinico"

// construirIdentificacao converte o DTO de identificação num VO validado.
func construirIdentificacao(d DadosIdentificacao) (dominio.Identificacao, error) {
	sexo, err := dominio.ParseSexo(d.Sexo)
	if err != nil {
		return dominio.Identificacao{}, err
	}
	return dominio.NovaIdentificacao(d.NomeCompleto, d.DataNascimento, sexo, d.BI, d.NIF, d.Passaporte)
}

// construirContactos converte o DTO de contactos num VO validado.
func construirContactos(d DadosContactos) (dominio.Contactos, error) {
	var morada *dominio.Morada
	if d.Morada != nil {
		morada = &dominio.Morada{
			Provincia: d.Morada.Provincia, Municipio: d.Morada.Municipio,
			Comuna: d.Morada.Comuna, Bairro: d.Morada.Bairro, Rua: d.Morada.Rua,
			Casa: d.Morada.Casa, Referencia: d.Morada.Referencia,
		}
	}
	return dominio.NovosContactos(d.Telefone, d.Email, morada)
}

// paraDetalhe mapeia um agregado Doente para o DTO de detalhe.
func paraDetalhe(d *dominio.Doente) DetalheDoente {
	s := d.Snapshot()
	det := DetalheDoente{
		ID:             s.ID,
		NumProcesso:    s.NumProcesso,
		NomeCompleto:   s.Identificacao.NomeCompleto,
		DataNascimento: s.Identificacao.DataNascimento,
		Sexo:           string(s.Identificacao.Sexo),
		BI:             s.Identificacao.BI,
		NIF:            s.Identificacao.NIF,
		Passaporte:     s.Identificacao.Passaporte,
		Nacionalidade:  s.Nacionalidade,
		Telefone:       s.Contactos.Telefone,
		Email:          s.Contactos.Email,
		Estado:         string(s.Estado),
		CriadoEm:       s.CriadoEm,
		ActualizadoEm:  s.ActualizadoEm,
		Alergias:       []AlergiaDTO{},
		Antecedentes:   []AntecedenteDTO{},
	}
	if s.Contactos.Morada != nil {
		m := s.Contactos.Morada
		det.Morada = &MoradaDTO{
			Provincia: m.Provincia, Municipio: m.Municipio, Comuna: m.Comuna,
			Bairro: m.Bairro, Rua: m.Rua, Casa: m.Casa, Referencia: m.Referencia,
		}
	}
	if s.GrupoSanguineo != nil {
		g := s.GrupoSanguineo.String()
		det.GrupoSanguineo = &g
	}
	for _, a := range s.Alergias {
		det.Alergias = append(det.Alergias, AlergiaDTO{
			Substancia: a.Substancia, Severidade: string(a.Severidade),
			ReaccaoTipica: a.ReaccaoTipica, ConfirmadaEm: a.ConfirmadaEm, Notas: a.Notas,
		})
	}
	for _, a := range s.Antecedentes {
		det.Antecedentes = append(det.Antecedentes, AntecedenteDTO{
			Tipo: string(a.Tipo), Descricao: a.Descricao, CID: a.CID,
			DataInicio: a.DataInicio, Activo: a.Activo, Notas: a.Notas,
		})
	}
	return det
}
