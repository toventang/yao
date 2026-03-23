package discord

import (
	"context"
	"time"

	"github.com/bwmarrin/discordgo"
	dcapi "github.com/yaoapp/yao/integrations/discord"
)

const reconnectDelay = 5 * time.Second

// gatewayLoop starts the Discord WebSocket Gateway for a single bot.
// It automatically reconnects on failure.
func (a *Adapter) gatewayLoop(ctx context.Context, entry *botEntry) {
	log.Info("discord gatewayLoop started robot=%s app=%s", entry.robotID, entry.appID)

	for {
		select {
		case <-ctx.Done():
			log.Info("discord gatewayLoop stopped robot=%s", entry.robotID)
			return
		case <-a.stopCh:
			return
		default:
		}

		err := a.runGateway(ctx, entry)
		if err != nil {
			log.Error("discord gateway disconnected robot=%s: %v, reconnecting in %s", entry.robotID, err, reconnectDelay)
		}

		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-time.After(reconnectDelay):
		}
	}
}

func (a *Adapter) runGateway(ctx context.Context, entry *botEntry) error {
	session := entry.bot.Session()

	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		a.onMessageCreate(ctx, entry, m)
	})

	if err := session.Open(); err != nil {
		return err
	}

	// Block until context is cancelled or stop signal
	select {
	case <-ctx.Done():
	case <-a.stopCh:
	}

	return session.Close()
}

func (a *Adapter) onMessageCreate(ctx context.Context, entry *botEntry, m *discordgo.MessageCreate) {
	if m == nil || m.Author == nil {
		return
	}

	// Ignore bot's own messages
	if m.Author.Bot {
		return
	}

	cm := dcapi.ConvertMessageCreate(m)
	if cm == nil {
		return
	}

	if cm.HasMedia() {
		groups := []string{"discord", entry.robotID}
		dcapi.ResolveMedia(ctx, cm, groups)
	}

	a.handleMessages(ctx, entry, []*dcapi.ConvertedMessage{cm})
}
