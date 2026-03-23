package discord

import (
	"testing"
)

// TestE2E_01_BotUser verifies the Discord bot token by fetching bot user info.
func TestE2E_01_BotUser(t *testing.T) {
	skipIfNoToken(t)
	bot := testBot(t)

	user, err := bot.BotUser()
	if err != nil {
		t.Fatalf("BotUser: %v", err)
	}
	if user.ID == "" {
		t.Error("user.ID should not be empty")
	}
	if user.Username == "" {
		t.Error("user.Username should not be empty")
	}
	if !user.Bot {
		t.Error("user.Bot should be true")
	}
	t.Logf("OK  id=%s username=%s discriminator=%s bot=%v",
		user.ID, user.Username, user.Discriminator, user.Bot)
}
