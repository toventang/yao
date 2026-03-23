package feishu

import (
	"testing"
)

func TestNewBot(t *testing.T) {
	b := NewBot("cli_xxx", "secret_yyy")
	if b.AppID() != "cli_xxx" {
		t.Fatalf("expected AppID cli_xxx, got %s", b.AppID())
	}
	if b.AppSecret() != "secret_yyy" {
		t.Fatalf("expected AppSecret secret_yyy, got %s", b.AppSecret())
	}
	if b.Client() == nil {
		t.Fatal("Client() should not be nil")
	}
}
