package telegram

import (
	"context"
	"encoding/json"

	"github.com/go-telegram/bot/models"
	"github.com/yaoapp/yao/event"
	eventtypes "github.com/yaoapp/yao/event/types"
	tgapi "github.com/yaoapp/yao/integrations/telegram"
	webhooktypes "github.com/yaoapp/yao/openapi/integrations"
)

// StartWebhookSubscription subscribes to integration.webhook.telegram events.
// Call once after event.Start().
func (a *Adapter) StartWebhookSubscription() {
	ch := make(chan *eventtypes.Event, 128)
	a.webhSub = event.Subscribe("integration.webhook.telegram", ch)
	go a.handleWebhooks(ch)
	log.Info("telegram adapter: webhook subscription started")
}

// StopWebhookSubscription unsubscribes from webhook events.
func (a *Adapter) StopWebhookSubscription() {
	if a.webhSub != "" {
		event.Unsubscribe(a.webhSub)
		a.webhSub = ""
	}
}

func (a *Adapter) handleWebhooks(ch <-chan *eventtypes.Event) {
	for ev := range ch {
		var payload webhooktypes.WebhookPayload
		if err := ev.Should(&payload); err != nil {
			log.Error("telegram adapter: invalid webhook event: %v", err)
			continue
		}

		entry, ok := a.resolveByAppID(payload.AppID)
		if !ok {
			log.Warn("telegram adapter: unknown app_id=%s", payload.AppID)
			continue
		}

		headerSecret := payload.Headers["X-Telegram-Bot-Api-Secret-Token"]
		if !entry.bot.VerifyWebhook(headerSecret) {
			log.Warn("telegram adapter: webhook secret mismatch app_id=%s", payload.AppID)
			continue
		}

		var update models.Update
		if err := json.Unmarshal(payload.Body, &update); err != nil {
			log.Error("telegram adapter: webhook unmarshal failed: %v", err)
			continue
		}

		cm := tgapi.ConvertUpdate(&update)
		if cm != nil && cm.HasMedia() {
			groups := []string{"telegram", entry.robotID}
			ctx := context.Background()
			entry.bot.ResolveMedia(ctx, cm, groups)
		}
		a.handleMessages(context.Background(), entry, []*tgapi.ConvertedMessage{cm})
	}
}
