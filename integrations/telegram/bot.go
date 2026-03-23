package telegram

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-telegram/bot"
)

const defaultAPIBase = "https://api.telegram.org"

// Bot represents a single Telegram bot instance bound to a specific token.
// Each registered robot gets its own Bot. All API methods live on Bot so
// callers never need to pass the token around.
type Bot struct {
	token       string
	secretToken string // for webhook X-Telegram-Bot-Api-Secret-Token verification
	apiBase     string // e.g. "https://api.telegram.org" or "http://localhost:3001"
	httpClient  *http.Client
}

// BotOption configures optional Bot parameters.
type BotOption func(*Bot)

// WithAPIBase sets a custom Bot API server URL (e.g. a local telegram-bot-api instance).
func WithAPIBase(url string) BotOption {
	return func(b *Bot) {
		b.apiBase = strings.TrimRight(url, "/")
	}
}

// NewBot creates a Bot bound to the given token.
// secretToken is optional — when set it is sent with SetWebhook and used by
// VerifyWebhook to authenticate incoming requests.
func NewBot(token string, secretToken string, opts ...BotOption) *Bot {
	b := &Bot{
		token:       token,
		secretToken: secretToken,
		apiBase:     defaultAPIBase,
		httpClient:  &http.Client{Timeout: 90 * time.Second},
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

// Token returns the raw bot token.
func (b *Bot) Token() string { return b.token }

// SecretToken returns the webhook secret (may be empty).
func (b *Bot) SecretToken() string { return b.secretToken }

// APIBase returns the API server base URL.
func (b *Bot) APIBase() string { return b.apiBase }

// botURL builds the bot-method base URL: {apiBase}/bot{token}
func (b *Bot) botURL() string {
	return b.apiBase + "/bot" + b.token
}

// fileURL builds the file download URL: {apiBase}/file/bot{token}/{path}
func (b *Bot) fileURL(filePath string) string {
	return b.apiBase + "/file/bot" + b.token + "/" + filePath
}

// sdk returns a go-telegram/bot.Bot instance for typed method calls.
func (b *Bot) sdk() (*bot.Bot, error) {
	opts := []bot.Option{
		bot.WithSkipGetMe(),
		bot.WithHTTPClient(90*time.Second, b.httpClient),
	}
	if b.apiBase != defaultAPIBase {
		opts = append(opts, bot.WithServerURL(b.apiBase))
	}
	sdkBot, err := bot.New(b.token, opts...)
	if err != nil {
		return nil, fmt.Errorf("create bot sdk: %w", err)
	}
	return sdkBot, nil
}
