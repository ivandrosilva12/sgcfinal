package identidade_test

import (
	"context"
	"errors"
	"testing"

	appident "github.com/ivandrosilva12/sgcfinal/internal/application/identidade"
)

func TestResetPassword_GeraEAudita(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoResetPassword(admin, aud, &fakeNotificador{})

	out, err := caso.Executar(context.Background(), "actor-1", "alvo-1")
	if err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if out.SenhaTemporaria == "" {
		t.Fatal("esperava senha temporária não vazia")
	}
	if admin.passwordDefinida["alvo-1"] != out.SenhaTemporaria {
		t.Fatalf("senha passada ao adaptador != devolvida: %v", admin.passwordDefinida)
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.password.reposta" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
}

func TestResetPassword_PropagaErro(t *testing.T) {
	admin := &fakeAdmin{err: errors.New("kc down")}
	caso := appident.NovoCasoResetPassword(admin, &fakeAuditor{}, &fakeNotificador{})
	if _, err := caso.Executar(context.Background(), "actor-1", "alvo-1"); err == nil {
		t.Fatal("esperava erro propagado")
	}
}

func TestResetPassword_FalhaEmailNaoFalha(t *testing.T) {
	admin := &fakeAdmin{}
	notif := &fakeNotificador{err: errors.New("smtp em baixo")}
	caso := appident.NovoCasoResetPassword(admin, &fakeAuditor{}, notif)
	out, err := caso.Executar(context.Background(), "actor-1", "alvo-1")
	if err != nil {
		t.Fatalf("falha de email não deve falhar o reset, obtive %v", err)
	}
	if out.SenhaTemporaria == "" {
		t.Fatal("esperava senha temporária devolvida na mesma")
	}
	if notif.resets != 1 {
		t.Fatalf("esperava 1 tentativa de notificação, obtive %d", notif.resets)
	}
}

func TestResetOTP_Audita(t *testing.T) {
	admin := &fakeAdmin{}
	aud := &fakeAuditor{}
	caso := appident.NovoCasoResetOTP(admin, aud)
	if err := caso.Executar(context.Background(), "actor-1", "alvo-1"); err != nil {
		t.Fatalf("esperava sucesso, obtive %v", err)
	}
	if !admin.otpReposto["alvo-1"] {
		t.Fatal("esperava reset de OTP delegado")
	}
	if len(aud.registos) != 1 || aud.registos[0].Accao != "identidade.otp.reposto" {
		t.Fatalf("auditoria em falta: %v", aud.registos)
	}
}
