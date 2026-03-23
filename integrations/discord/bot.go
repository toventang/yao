package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

// Bot represents a single Discord bot instance bound to a token.
type Bot struct {
	token   string
	appID   string
	session *discordgo.Session
}

// NewBot creates a Bot bound to the given Discord bot token.
func NewBot(token, appID string) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentMessageContent
	return &Bot{
		token:   token,
		appID:   appID,
		session: session,
	}, nil
}

// Token returns the raw bot token.
func (b *Bot) Token() string { return b.token }

// AppID returns the application ID.
func (b *Bot) AppID() string { return b.appID }

// Session returns the underlying discordgo session.
func (b *Bot) Session() *discordgo.Session { return b.session }

// BotUser returns the bot's own user information (verifies token).
func (b *Bot) BotUser() (*discordgo.User, error) {
	return b.session.User("@me")
}
