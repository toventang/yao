package feishu

import (
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// Bot represents a single Feishu bot instance bound to an app.
type Bot struct {
	appID     string
	appSecret string
	client    *lark.Client
}

// NewBot creates a Bot bound to the given Feishu app credentials.
func NewBot(appID, appSecret string) *Bot {
	client := lark.NewClient(appID, appSecret,
		lark.WithLogLevel(larkcore.LogLevelWarn),
	)
	return &Bot{
		appID:     appID,
		appSecret: appSecret,
		client:    client,
	}
}

// AppID returns the app ID.
func (b *Bot) AppID() string { return b.appID }

// AppSecret returns the app secret (needed for WS client).
func (b *Bot) AppSecret() string { return b.appSecret }

// Client returns the underlying Lark SDK client.
func (b *Bot) Client() *lark.Client { return b.client }

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
