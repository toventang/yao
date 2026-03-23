package dingtalk

import (
	"context"
	"fmt"
	"strings"

	agentcontext "github.com/yaoapp/yao/agent/context"
	events "github.com/yaoapp/yao/agent/robot/events"
	dtapi "github.com/yaoapp/yao/integrations/dingtalk"
)

// Reply sends the assistant message back to the originating DingTalk conversation.
func (a *Adapter) Reply(ctx context.Context, msg *agentcontext.Message, metadata *events.MessageMetadata) error {
	if msg == nil || metadata == nil {
		return fmt.Errorf("nil message or metadata")
	}

	var sessionWebhook string
	if metadata.Extra != nil {
		if v, ok := metadata.Extra["session_webhook"]; ok {
			if s, ok := v.(string); ok {
				sessionWebhook = s
			}
		}
	}

	if sessionWebhook == "" {
		return fmt.Errorf("no session_webhook in metadata for dingtalk reply")
	}

	return sendContent(ctx, sessionWebhook, msg.Content)
}

func sendContent(ctx context.Context, sessionWebhook string, content interface{}) error {
	switch c := content.(type) {
	case string:
		if strings.TrimSpace(c) == "" {
			return nil
		}
		return dtapi.SendMarkdownMessage(ctx, sessionWebhook, "Reply", dtapi.FormatDingTalkMarkdown(c))

	case []interface{}:
		return sendParts(ctx, sessionWebhook, c)

	default:
		parts, ok := toContentParts(content)
		if ok {
			return sendPartsTyped(ctx, sessionWebhook, parts)
		}
		return dtapi.SendTextMessage(ctx, sessionWebhook, fmt.Sprintf("%v", content))
	}
}

func sendParts(ctx context.Context, sessionWebhook string, parts []interface{}) error {
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
			if err := flushText(ctx, sessionWebhook, &textBuf); err != nil {
				return err
			}
			if imgMap, ok := m["image_url"].(map[string]interface{}); ok {
				if url, ok := imgMap["url"].(string); ok {
					if strings.HasPrefix(url, "http") {
						textBuf.WriteString(fmt.Sprintf("\n![image](%s)\n", url))
					}
				}
			}
		case "file":
			if err := flushText(ctx, sessionWebhook, &textBuf); err != nil {
				return err
			}
			fileURL, _ := m["file_url"].(string)
			fileName, _ := m["file_name"].(string)
			if fileURL == "" {
				if fileMap, ok := m["file"].(map[string]interface{}); ok {
					fileURL, _ = fileMap["url"].(string)
					if fn, ok := fileMap["filename"].(string); ok && fn != "" {
						fileName = fn
					}
				}
			}
			if fileURL != "" && strings.HasPrefix(fileURL, "http") {
				label := fileName
				if label == "" {
					label = "file"
				}
				textBuf.WriteString(fmt.Sprintf("\n[%s](%s)\n", label, fileURL))
			}
		}
	}
	return flushText(ctx, sessionWebhook, &textBuf)
}

func sendPartsTyped(ctx context.Context, sessionWebhook string, parts []agentcontext.ContentPart) error {
	var textBuf strings.Builder
	for _, part := range parts {
		switch part.Type {
		case agentcontext.ContentText:
			textBuf.WriteString(part.Text)
		case agentcontext.ContentImageURL:
			if err := flushText(ctx, sessionWebhook, &textBuf); err != nil {
				return err
			}
			if part.ImageURL != nil && strings.HasPrefix(part.ImageURL.URL, "http") {
				textBuf.WriteString(fmt.Sprintf("\n![image](%s)\n", part.ImageURL.URL))
			}
		case agentcontext.ContentFile:
			if err := flushText(ctx, sessionWebhook, &textBuf); err != nil {
				return err
			}
			if part.File != nil && part.File.URL != "" && strings.HasPrefix(part.File.URL, "http") {
				label := part.File.Filename
				if label == "" {
					label = "file"
				}
				textBuf.WriteString(fmt.Sprintf("\n[%s](%s)\n", label, part.File.URL))
			}
		}
	}
	return flushText(ctx, sessionWebhook, &textBuf)
}

func flushText(ctx context.Context, sessionWebhook string, buf *strings.Builder) error {
	if buf.Len() == 0 {
		return nil
	}
	text := buf.String()
	buf.Reset()
	return dtapi.SendMarkdownMessage(ctx, sessionWebhook, "Reply", dtapi.FormatDingTalkMarkdown(text))
}

func toContentParts(content interface{}) ([]agentcontext.ContentPart, bool) {
	parts, ok := content.([]agentcontext.ContentPart)
	return parts, ok
}
