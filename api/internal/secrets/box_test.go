package secrets

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

func newTestBox(t *testing.T, fill byte) *Box {
	t.Helper()
	b, err := NewBox(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBox(t *testing.T) {
	b := newTestBox(t, 1)
	secret := []byte("0123456789ABCDEF0123456789ABCDEF")

	sealed, err := b.Seal(secret, []byte("user:1"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, secret) {
		t.Fatal("sealed value contains the plaintext")
	}
	again, _ := b.Seal(secret, []byte("user:1"))
	if bytes.Equal(sealed, again) {
		t.Fatal("nonce must be random: same input sealed twice gave the same output")
	}

	got, err := b.Open(sealed, []byte("user:1"))
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("open: %q, %v", got, err)
	}

	t.Run("bound to aad", func(t *testing.T) {
		if _, err := b.Open(sealed, []byte("user:2")); !errors.Is(err, ErrDecrypt) {
			t.Fatalf("err = %v, want ErrDecrypt", err)
		}
	})
	t.Run("tampered", func(t *testing.T) {
		bad := bytes.Clone(sealed)
		bad[len(bad)-1] ^= 1
		if _, err := b.Open(bad, []byte("user:1")); !errors.Is(err, ErrDecrypt) {
			t.Fatalf("err = %v, want ErrDecrypt", err)
		}
	})
	t.Run("other master key", func(t *testing.T) {
		if _, err := newTestBox(t, 2).Open(sealed, []byte("user:1")); !errors.Is(err, ErrDecrypt) {
			t.Fatalf("err = %v, want ErrDecrypt", err)
		}
	})
	t.Run("short input", func(t *testing.T) {
		if _, err := b.Open([]byte{1, 2}, nil); !errors.Is(err, ErrDecrypt) {
			t.Fatalf("err = %v, want ErrDecrypt", err)
		}
	})
}

func TestNewBoxRejectsBadKeys(t *testing.T) {
	for _, k := range []string{"", "not base64!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := NewBox(k); err == nil {
			t.Errorf("NewBox(%q) accepted a bad key", k)
		}
	}
}
