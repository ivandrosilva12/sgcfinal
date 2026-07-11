//go:build integration

package integration_test

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"time"
)

// codigoTOTP calcula o código TOTP (RFC 6238: HMAC-SHA1, 6 dígitos, período 30s)
// a partir do segredo cru guardado pelo Keycloak (secretData.value é usado como
// chave HMAC directa, não base32). Se o Keycloak rejeitar o código, ver a nota no
// spike sobre base32 — algumas versões guardam o segredo noutra codificação.
func codigoTOTP(segredo string, em time.Time) string {
	chave := []byte(segredo)
	contador := uint64(em.Unix()) / 30
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], contador)
	mac := hmac.New(sha1.New, chave)
	_, _ = mac.Write(buf[:])
	soma := mac.Sum(nil)
	deslocamento := soma[len(soma)-1] & 0x0f
	valor := (uint32(soma[deslocamento]&0x7f) << 24) |
		(uint32(soma[deslocamento+1]) << 16) |
		(uint32(soma[deslocamento+2]) << 8) |
		uint32(soma[deslocamento+3])
	return fmt.Sprintf("%06d", valor%1000000)
}
