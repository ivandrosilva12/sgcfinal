package identidade

// Papel é um papel RBAC do SGC Angola. Os 11 valores canónicos provêm do
// DDM-001 v2.0 (ver docs/ERRATA-001-papeis.md) e são semeados em
// seeds/papeis.sql. Correspondem aos realm roles do Keycloak.
type Papel string

const (
	PapelMedico             Papel = "Medico"
	PapelEnfermeiro         Papel = "Enfermeiro"
	PapelAdministrativo     Papel = "Administrativo"
	PapelFarmaceutico       Papel = "Farmaceutico"
	PapelFarmaceuticoSenior Papel = "FarmaceuticoSenior"
	PapelTecnicoLab         Papel = "TecnicoLab"
	PapelPatologista        Papel = "Patologista"
	PapelDirector           Papel = "Director"
	PapelAdmin              Papel = "Admin"
	PapelDPO                Papel = "DPO"
	PapelAuditor            Papel = "Auditor"
)

// papeisValidos é o conjunto canónico dos 11 papéis.
var papeisValidos = map[Papel]bool{
	PapelMedico:             true,
	PapelEnfermeiro:         true,
	PapelAdministrativo:     true,
	PapelFarmaceutico:       true,
	PapelFarmaceuticoSenior: true,
	PapelTecnicoLab:         true,
	PapelPatologista:        true,
	PapelDirector:           true,
	PapelAdmin:              true,
	PapelDPO:                true,
	PapelAuditor:            true,
}

// papeisSensiveis exigem MFA (Sprint 3). Alinhado com sensivel=true em
// seeds/papeis.sql: Director, Admin, DPO, Auditor.
var papeisSensiveis = map[Papel]bool{
	PapelDirector: true,
	PapelAdmin:    true,
	PapelDPO:      true,
	PapelAuditor:  true,
}

// PapelValido indica se o código corresponde a um dos 11 papéis canónicos.
func PapelValido(codigo string) bool {
	return papeisValidos[Papel(codigo)]
}

// EhSensivel indica se o papel exige protecção reforçada (MFA em Sprint 3).
func EhSensivel(p Papel) bool {
	return papeisSensiveis[p]
}

// PapeisDe converte uma lista de códigos (ex.: os realm roles de um token) na
// lista de Papel válidos, ignorando silenciosamente códigos desconhecidos
// (roles internos do Keycloak como "offline_access" ou "default-roles-*").
func PapeisDe(codigos []string) []Papel {
	papeis := make([]Papel, 0, len(codigos))
	for _, c := range codigos {
		if PapelValido(c) {
			papeis = append(papeis, Papel(c))
		}
	}
	return papeis
}
