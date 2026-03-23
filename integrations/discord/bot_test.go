package discord

import (
	"testing"
)

func TestNewBot(t *testing.T) {
	bot, err := NewBot("test-token", "test-app-id")
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	if bot.Token() != "test-token" {
		t.Fatalf("expected token test-token, got %s", bot.Token())
	}
	if bot.AppID() != "test-app-id" {
		t.Fatalf("expected appID test-app-id, got %s", bot.AppID())
	}
	if bot.Session() == nil {
		t.Fatal("Session() should not be nil")
	}
}
