package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-telegram/bot/models"
)

// GetUpdates fetches new updates via long polling and returns them as
// ConvertedMessages ready for consumption. Non-message updates (e.g.
// callback_query without a message) are silently skipped.
// When groups is non-nil, media attachments are automatically downloaded
// and stored via DownloadAndStore, filling each MediaItem.Wrapper.
func (b *Bot) GetUpdates(ctx context.Context, offset int64, timeout int, groups []string) ([]*ConvertedMessage, error) {
	raw, err := b.GetRawUpdates(ctx, offset, timeout)
	if err != nil {
		return nil, err
	}
	var msgs []*ConvertedMessage
	for i := range raw {
		if cm := ConvertUpdate(&raw[i]); cm != nil {
			if groups != nil && cm.HasMedia() {
				b.ResolveMedia(ctx, cm, groups)
			}
			msgs = append(msgs, cm)
		}
	}
	return msgs, nil
}

// GetRawUpdates fetches raw Telegram updates without conversion.
// Use this when you need access to the original models.Update (e.g. for
// offset tracking). Uses raw HTTP because the SDK keeps getUpdates private.
func (b *Bot) GetRawUpdates(ctx context.Context, offset int64, timeout int) ([]models.Update, error) {
	params := map[string]interface{}{
		"offset":  offset,
		"timeout": timeout,
		"limit":   100,
	}
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", b.botURL()+"/getUpdates", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram API error status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		OK     bool            `json:"ok"`
		Result []models.Update `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram API returned ok=false")
	}
	return result.Result, nil
}
