package telegram

import (
	"testing"
)

func TestNewBot(t *testing.T) {
	b := NewBot("123:ABC", "my-secret")
	if b.Token() != "123:ABC" {
		t.Fatalf("expected token 123:ABC, got %s", b.Token())
	}
	if b.SecretToken() != "my-secret" {
		t.Fatalf("expected secret my-secret, got %s", b.SecretToken())
	}
}

func TestNewBot_EmptySecret(t *testing.T) {
	b := NewBot("123:ABC", "")
	if b.SecretToken() != "" {
		t.Fatalf("expected empty secret, got %s", b.SecretToken())
	}
}
