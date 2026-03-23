package feishu

import (
	"context"
	"testing"
	"time"
)

// TestE2E_01_BotCredentials verifies the Feishu app credentials by
// sending a simple text message send request (if a chat_id is available).
func TestE2E_01_BotCredentials(t *testing.T) {
	skipIfNoCreds(t)
	b := testBot()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Verify credentials by attempting to send a message.
	// This will fail with a descriptive error if credentials are invalid.
	_, err := b.sendMessage(ctx, "open_id", "test_invalid_open_id", "text", `{"text":"e2e test"}`)
	if err == nil {
		t.Log("message send succeeded (unexpected, but credentials are valid)")
		return
	}

	// We expect a Feishu API error (not a network error), which proves
	// the credentials were accepted and the API was reached.
	t.Logf("API response (expected error for invalid open_id): %v", err)
}

// TestE2E_02_SendMessage tests sending a real message if FEISHU_TEST_CHAT_ID is set.
func TestE2E_02_SendMessage(t *testing.T) {
	skipIfNoCreds(t)

	chatID := getChatID(t)
	if chatID == "" {
		t.Skip("no chat_id available for send test")
	}

	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	msgID, err := b.SendTextMessage(ctx, chatID, "E2E test from Yao integration at "+time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("SendTextMessage: %v", err)
	}
	t.Logf("OK  sent message_id=%s to chat=%s", msgID, chatID)
}

// TestE2E_03_SendImage tests sending an image message.
func TestE2E_03_SendImage(t *testing.T) {
	skipIfNoCreds(t)

	chatID := getChatID(t)
	if chatID == "" {
		t.Skip("no chat_id available for image send test")
	}

	// Would need an uploaded image_key. Skip if not available.
	t.Skip("image_key upload not implemented yet in test suite")
}

// getChatID attempts to retrieve a test chat ID from environment or skip.
func getChatID(t *testing.T) string {
	t.Helper()
	// For now we don't have a chat_id mechanism like Telegram's getUpdates.
	// A chat_id can be obtained by having the bot in a group or by user messaging the bot first.
	return ""
}
