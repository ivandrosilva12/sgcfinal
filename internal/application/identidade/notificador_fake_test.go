package identidade_test

import "context"

// fakeNotificador captura as chamadas de notificação; `err` simula falha de envio.
type fakeNotificador struct {
	criacoes int
	resets   int
	err      error
}

func (f *fakeNotificador) NotificarCriacao(context.Context, string, string, string) error {
	f.criacoes++
	return f.err
}
func (f *fakeNotificador) NotificarResetPassword(context.Context, string, string, string) error {
	f.resets++
	return f.err
}
