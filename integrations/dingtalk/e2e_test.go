package dingtalk

import (
	"context"
	"testing"
	"time"
)

// TestE2E_01_GetAccessToken verifies the DingTalk credentials by requesting an access token.
func TestE2E_01_GetAccessToken(t *testing.T) {
	skipIfNoCreds(t)
	b := testBotInstance()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	token, err := b.GetAccessToken(ctx)
	if err != nil {
		t.Fatalf("GetAccessToken: %v", err)
	}
	if token == "" {
		t.Fatal("access token should not be empty")
	}
	t.Logf("OK  access_token=%s... (truncated)", token[:min(20, len(token))])

	// Verify token caching
	token2, err := b.GetAccessToken(ctx)
	if err != nil {
		t.Fatalf("GetAccessToken (cached): %v", err)
	}
	if token2 != token {
		t.Error("cached token should be the same")
	}
	t.Log("OK  token caching verified")
}

// TestE2E_02_BotInfo verifies bot credentials via GetBotInfo.
func TestE2E_02_BotInfo(t *testing.T) {
	skipIfNoCreds(t)
	b := testBotInstance()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := b.GetBotInfo(ctx)
	if err != nil {
		t.Fatalf("GetBotInfo: %v", err)
	}
	t.Log("OK  bot credentials verified")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
