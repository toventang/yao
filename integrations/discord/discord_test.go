package discord

import (
	"os"
	"testing"
)

var (
	testBotToken string
	testAppID    string
)

func TestMain(m *testing.M) {
	testBotToken = os.Getenv("DISCORD_TEST_BOT_TOKEN")
	testAppID = os.Getenv("DISCORD_TEST_APP_ID")
	os.Exit(m.Run())
}

func skipIfNoToken(t *testing.T) {
	t.Helper()
	if testBotToken == "" {
		t.Skip("DISCORD_TEST_BOT_TOKEN not set")
	}
}

func testBot(t *testing.T) *Bot {
	t.Helper()
	bot, err := NewBot(testBotToken, testAppID)
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	return bot
}
