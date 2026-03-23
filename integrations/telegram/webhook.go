package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// SetWebhook registers a webhook URL with Telegram. The configured
// secret_token (if any) is sent along so Telegram includes it in every
// webhook request header for verification.
func (b *Bot) SetWebhook(ctx context.Context, url string, allowedUpdates []string) error {
	sdk, err := b.sdk()
	if err != nil {
		return err
	}
	params := &bot.SetWebhookParams{
		URL:            url,
		AllowedUpdates: allowedUpdates,
		SecretToken:    b.secretToken,
	}
	if _, err := sdk.SetWebhook(ctx, params); err != nil {
		return fmt.Errorf("setWebhook: %w", err)
	}
	return nil
}

// DeleteWebhook removes the webhook configuration from Telegram.
func (b *Bot) DeleteWebhook(ctx context.Context, dropPending bool) error {
	sdk, err := b.sdk()
	if err != nil {
		return err
	}
	if _, err := sdk.DeleteWebhook(ctx, &bot.DeleteWebhookParams{
		DropPendingUpdates: dropPending,
	}); err != nil {
		return fmt.Errorf("deleteWebhook: %w", err)
	}
	return nil
}

// ParseWebhookPayload reads and parses a Telegram webhook request body,
// verifies the secret header, and returns a ConvertedMessage ready for use.
// Returns nil message (no error) if the update contains no processable message.
// When groups is non-nil, media attachments are automatically resolved.
func (b *Bot) ParseWebhookPayload(ctx context.Context, r *http.Request, groups []string) (*ConvertedMessage, error) {
	update, err := b.ParseRawWebhookPayload(r)
	if err != nil {
		return nil, err
	}
	cm := ConvertUpdate(update)
	if cm != nil && groups != nil && cm.HasMedia() {
		b.ResolveMedia(ctx, cm, groups)
	}
	return cm, nil
}

// ParseRawWebhookPayload reads and parses a Telegram webhook request body
// into a raw models.Update. It also verifies the X-Telegram-Bot-Api-Secret-Token
// header when a secret is configured.
func (b *Bot) ParseRawWebhookPayload(r *http.Request) (*models.Update, error) {
	if !b.VerifyWebhook(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")) {
		return nil, fmt.Errorf("webhook secret mismatch")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	defer r.Body.Close()

	var update models.Update
	if err := json.Unmarshal(body, &update); err != nil {
		return nil, fmt.Errorf("unmarshal update: %w", err)
	}
	return &update, nil
}
