package keycloak

import "testing"

func TestDividirIssuer(t *testing.T) {
	casos := []struct {
		issuer    string
		wantBase  string
		wantRealm string
		wantOK    bool
	}{
		{"http://localhost:8081/realms/sgc", "http://localhost:8081", "sgc", true},
		{"https://kc.exemplo.ao/auth/realms/sgc", "https://kc.exemplo.ao/auth", "sgc", true},
		{"http://localhost:8081/realms/", "", "", false},
		{"http://localhost:8081/sem-realm", "", "", false},
		{"", "", "", false},
	}
	for _, c := range casos {
		base, realm, ok := dividirIssuer(c.issuer)
		if base != c.wantBase || realm != c.wantRealm || ok != c.wantOK {
			t.Fatalf("dividirIssuer(%q) = (%q,%q,%v); quer (%q,%q,%v)",
				c.issuer, base, realm, ok, c.wantBase, c.wantRealm, c.wantOK)
		}
	}
}

func TestNovoAdmin_IssuerInvalido(t *testing.T) {
	if _, err := NovoAdmin("http://x/sem-realm", "sgc-admin", "segredo"); err == nil {
		t.Fatal("esperava erro para issuer sem /realms/")
	}
}
