package dingtalk

import (
	"context"
	"strings"
	"time"

	dingstream "github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	dingclient "github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	dtapi "github.com/yaoapp/yao/integrations/dingtalk"
)

const reconnectDelay = 5 * time.Second

// streamLoop starts the DingTalk Stream client for a single bot.
// It automatically reconnects on failure.
func (a *Adapter) streamLoop(ctx context.Context, entry *botEntry) {
	log.Info("dingtalk streamLoop started robot=%s client=%s", entry.robotID, entry.clientID)

	for {
		select {
		case <-ctx.Done():
			log.Info("dingtalk streamLoop stopped robot=%s", entry.robotID)
			return
		case <-a.stopCh:
			return
		default:
		}

		err := a.runStreamClient(ctx, entry)
		if err != nil {
			log.Error("dingtalk stream disconnected robot=%s: %v, reconnecting in %s", entry.robotID, err, reconnectDelay)
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

func (a *Adapter) runStreamClient(ctx context.Context, entry *botEntry) error {
	cli := dingclient.NewStreamClient(
		dingclient.WithAppCredential(dingclient.NewAppCredentialConfig(entry.clientID, entry.bot.ClientSecret())),
	)

	cli.RegisterChatBotCallbackRouter(func(c context.Context, data *dingstream.BotCallbackDataModel) ([]byte, error) {
		return a.onBotCallback(c, entry, data)
	})

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

func (a *Adapter) onBotCallback(ctx context.Context, entry *botEntry, data *dingstream.BotCallbackDataModel) ([]byte, error) {
	if data == nil {
		return nil, nil
	}

	cm := &dtapi.ConvertedMessage{
		MessageID:        data.MsgId,
		ConversationID:   data.ConversationId,
		ConversationType: data.ConversationType,
		SenderID:         data.SenderId,
		SenderNick:       data.SenderNick,
		SenderStaffID:    data.SenderStaffId,
		ChatbotUserID:    data.ChatbotUserId,
		IsInAtList:       data.IsInAtList,
		SessionWebhook:   data.SessionWebhook,
	}

	switch data.Msgtype {
	case "text":
		cm.Text = strings.TrimSpace(data.Text.Content)
	}

	if cm.HasMedia() {
		groups := []string{"dingtalk", entry.robotID}
		dtapi.ResolveMedia(ctx, cm, groups)
	}

	a.handleMessages(ctx, entry, []*dtapi.ConvertedMessage{cm})
	return nil, nil
}
