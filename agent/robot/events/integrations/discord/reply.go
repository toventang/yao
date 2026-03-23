package discord

import (
	"context"
	"fmt"
	"strings"

	agentcontext "github.com/yaoapp/yao/agent/context"
	events "github.com/yaoapp/yao/agent/robot/events"
	dcapi "github.com/yaoapp/yao/integrations/discord"
)

// Reply sends the assistant message back to the originating Discord channel.
func (a *Adapter) Reply(ctx context.Context, msg *agentcontext.Message, metadata *events.MessageMetadata) error {
	if msg == nil || metadata == nil {
		return fmt.Errorf("nil message or metadata")
	}

	entry := a.resolveByChat(metadata)
	if entry == nil {
		return fmt.Errorf("no bot registered for discord metadata (appID=%s)", metadata.AppID)
	}

	var replyToID string
	if metadata.Extra != nil {
		if v, ok := metadata.Extra["discord_message_id"]; ok {
			if s, ok := v.(string); ok {
				replyToID = s
			}
		}
	}

	return a.sendContent(ctx, entry, metadata.ChatID, replyToID, msg.Content)
}

func (a *Adapter) sendContent(ctx context.Context, entry *botEntry, channelID, replyToID string, content interface{}) error {
	switch c := content.(type) {
	case string:
		if strings.TrimSpace(c) == "" {
			return nil
		}
		formatted := dcapi.FormatDiscordMarkdown(c)
		if replyToID != "" {
			_, err := entry.bot.SendMessageReply(channelID, formatted, replyToID)
			return err
		}
		_, err := entry.bot.SendMessage(channelID, formatted)
		return err

	case []interface{}:
		return a.sendParts(ctx, entry, channelID, replyToID, c)

	default:
		parts, ok := toContentParts(content)
		if ok {
			return a.sendPartsTyped(ctx, entry, channelID, replyToID, parts)
		}
		_, err := entry.bot.SendMessage(channelID, fmt.Sprintf("%v", content))
		return err
	}
}

func (a *Adapter) sendParts(ctx context.Context, entry *botEntry, channelID, replyToID string, parts []interface{}) error {
	var textBuf strings.Builder
	for _, part := range parts {
		m, ok := part.(map[string]interface{})
		if !ok {
			continue
		}
		partType, _ := m["type"].(string)
		switch partType {
		case "text":
			if text, ok := m["text"].(string); ok {
				textBuf.WriteString(text)
			}
		case "image_url":
			if err := a.flushText(entry, channelID, replyToID, &textBuf); err != nil {
				return err
			}
			if imgMap, ok := m["image_url"].(map[string]interface{}); ok {
				if url, ok := imgMap["url"].(string); ok {
					if err := sendFileOrWrapper(entry, channelID, url, ""); err != nil {
						log.Error("discord reply: send image: %v", err)
					}
				}
			}
		case "file":
			if err := a.flushText(entry, channelID, replyToID, &textBuf); err != nil {
				return err
			}
			fileURL, _ := m["file_url"].(string)
			if fileURL == "" {
				if fileMap, ok := m["file"].(map[string]interface{}); ok {
					fileURL, _ = fileMap["url"].(string)
				}
			}
			if fileURL != "" {
				if err := sendFileOrWrapper(entry, channelID, fileURL, ""); err != nil {
					log.Error("discord reply: send file: %v", err)
				}
			}
		}
	}
	return a.flushText(entry, channelID, replyToID, &textBuf)
}

func (a *Adapter) sendPartsTyped(ctx context.Context, entry *botEntry, channelID, replyToID string, parts []agentcontext.ContentPart) error {
	var textBuf strings.Builder
	for _, part := range parts {
		switch part.Type {
		case agentcontext.ContentText:
			textBuf.WriteString(part.Text)
		case agentcontext.ContentImageURL:
			if err := a.flushText(entry, channelID, replyToID, &textBuf); err != nil {
				return err
			}
			if part.ImageURL != nil {
				if err := sendFileOrWrapper(entry, channelID, part.ImageURL.URL, ""); err != nil {
					log.Error("discord reply: send image: %v", err)
				}
			}
		case agentcontext.ContentFile:
			if err := a.flushText(entry, channelID, replyToID, &textBuf); err != nil {
				return err
			}
			if part.File != nil {
				if err := sendFileOrWrapper(entry, channelID, part.File.URL, part.File.Filename); err != nil {
					log.Error("discord reply: send file: %v", err)
				}
			}
		}
	}
	return a.flushText(entry, channelID, replyToID, &textBuf)
}

func (a *Adapter) flushText(entry *botEntry, channelID, replyToID string, buf *strings.Builder) error {
	if buf.Len() == 0 {
		return nil
	}
	text := dcapi.FormatDiscordMarkdown(buf.String())
	buf.Reset()

	if replyToID != "" {
		_, err := entry.bot.SendMessageReply(channelID, text, replyToID)
		return err
	}
	_, err := entry.bot.SendMessage(channelID, text)
	return err
}

func sendFileOrWrapper(entry *botEntry, channelID, url, caption string) error {
	if strings.Contains(url, "://") && !strings.HasPrefix(url, "http") {
		return entry.bot.SendMediaFromWrapper(channelID, url, caption)
	}
	if strings.HasPrefix(url, "http") {
		_, err := entry.bot.SendMessage(channelID, url)
		return err
	}
	return fmt.Errorf("unsupported file URL scheme: %s", url)
}

func toContentParts(content interface{}) ([]agentcontext.ContentPart, bool) {
	parts, ok := content.([]agentcontext.ContentPart)
	return parts, ok
}

func (a *Adapter) resolveByChat(metadata *events.MessageMetadata) *botEntry {
	if metadata.AppID != "" {
		if entry, ok := a.resolveByAppID(metadata.AppID); ok {
			return entry
		}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, entry := range a.bots {
		return entry
	}
	return nil
}
