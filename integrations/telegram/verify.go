package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-telegram/bot/models"
)

// GetMe calls the getMe endpoint to verify the bot token is valid.
// Uses raw HTTP for compatibility with both official and local Bot API servers.
func (b *Bot) GetMe(ctx context.Context) (*models.User, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", b.botURL()+"/getMe", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result struct {
		OK     bool        `json:"ok"`
		Result models.User `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("getMe: API returned ok=false")
	}
	return &result.Result, nil
}

// VerifyWebhook checks the X-Telegram-Bot-Api-Secret-Token header value
// against the bot's configured secret_token using constant-time comparison.
// Returns true if the secret matches or if no secret was configured.
func (b *Bot) VerifyWebhook(headerSecret string) bool {
	if b.secretToken == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(b.secretToken), []byte(headerSecret)) == 1
}
