package dingtalk

import (
	"testing"
)

func TestNewBot(t *testing.T) {
	b := NewBot("client_id", "client_secret")
	if b.ClientID() != "client_id" {
		t.Fatalf("expected ClientID client_id, got %s", b.ClientID())
	}
	if b.ClientSecret() != "client_secret" {
		t.Fatalf("expected ClientSecret client_secret, got %s", b.ClientSecret())
	}
}
