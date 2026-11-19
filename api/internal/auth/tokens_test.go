package auth

import (
	"testing"
	"time"
)

const secret = "test-secret-0123456789abcdef0123456789"

func TestTokensRoundTrip(t *testing.T) {
	tk := NewTokens(secret, time.Hour)
	s, err := tk.Issue(42)
	if err != nil {
		t.Fatal(err)
	}
	id, err := tk.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Fatalf("id = %d, want 42", id)
	}
}

func TestTokensRejectsExpired(t *testing.T) {
	s, err := NewTokens(secret, -time.Minute).Issue(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTokens(secret, time.Hour).Parse(s); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestTokensRejectsOtherSecret(t *testing.T) {
	s, err := NewTokens("other-secret-0123456789abcdef01234567", time.Hour).Issue(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTokens(secret, time.Hour).Parse(s); err == nil {
		t.Fatal("expected error for token signed with another secret")
	}
}
