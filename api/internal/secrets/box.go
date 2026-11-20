// Package secrets criptografa dados sensíveis guardados no banco (ex.: a
// API key da Steam do vendedor) com AES-256-GCM.
//
// A chave mestra vem de SECRETS_KEY e nunca fica no banco: quem tiver só um
// dump do Postgres não consegue ler as chaves dos usuários.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// Box é seguro para uso concorrente (cipher.AEAD não guarda estado).
type Box struct {
	aead cipher.AEAD
}

// NewBox recebe a chave mestra em base64 (32 bytes depois de decodificar).
// Gere uma com: openssl rand -base64 32
func NewBox(keyB64 string) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, errors.New("secrets key must be base64")
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secrets key must have 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal criptografa plaintext. aad (dado associado) amarra o resultado a um
// contexto, como o id do usuário: copiar o valor para a linha de outro
// usuário faz Open falhar. Saída: nonce || ciphertext || tag.
func (b *Box) Seal(plaintext, aad []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plaintext, aad), nil
}

var ErrDecrypt = errors.New("cannot decrypt secret")

func (b *Box) Open(sealed, aad []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return nil, ErrDecrypt
	}
	out, err := b.aead.Open(nil, sealed[:n], sealed[n:], aad)
	if err != nil {
		return nil, ErrDecrypt // não repassa detalhes do erro de criptografia
	}
	return out, nil
}
