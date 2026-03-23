package feishu

import (
	"context"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	fsapi "github.com/yaoapp/yao/integrations/feishu"
)

const reconnectDelay = 5 * time.Second

// eventLoop starts the Feishu WebSocket event subscription for a single bot.
// It automatically reconnects on failure.
func (a *Adapter) eventLoop(ctx context.Context, entry *botEntry) {
	log.Info("feishu eventLoop started robot=%s app=%s", entry.robotID, entry.appID)

	for {
		select {
		case <-ctx.Done():
			log.Info("feishu eventLoop stopped robot=%s", entry.robotID)
			return
		case <-a.stopCh:
			return
		default:
		}

		err := a.runWSClient(ctx, entry)
		if err != nil {
			log.Error("feishu ws disconnected robot=%s: %v, reconnecting in %s", entry.robotID, err, reconnectDelay)
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

func (a *Adapter) runWSClient(ctx context.Context, entry *botEntry) error {
	eventHandler := dispatcher.NewEventDispatcher("", "")
	eventHandler.OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
		return a.onMessageReceive(ctx, entry, event)
	})

	cli := larkws.NewClient(entry.bot.AppID(), entry.bot.AppSecret(),
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelWarn),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- cli.Start(ctx)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.stopCh:
		return nil
	case err := <-errCh:
		return err
	}
}

func (a *Adapter) onMessageReceive(ctx context.Context, entry *botEntry, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return nil
	}

	msg := event.Event.Message
	sender := event.Event.Sender

	msgType := derefStr(msg.MessageType)
	content := derefStr(msg.Content)
	messageID := derefStr(msg.MessageId)
	chatID := derefStr(msg.ChatId)
	chatType := derefStr(msg.ChatType)

	text, media := fsapi.ParseMessageContent(msgType, content)

	cm := &fsapi.ConvertedMessage{
		MessageID:    messageID,
		ChatID:       chatID,
		ChatType:     chatType,
		Text:         text,
		MediaItems:   media,
		EventID:      event.EventV2Base.Header.EventID,
		LanguageCode: "zh",
	}

	if sender != nil && sender.SenderId != nil {
		cm.SenderID = derefStr(sender.SenderId.OpenId)
	}

	if cm.HasMedia() {
		groups := []string{"feishu", entry.robotID}
		entry.bot.ResolveMedia(ctx, cm, groups)
	}

	a.handleMessages(ctx, entry, []*fsapi.ConvertedMessage{cm})
	return nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
