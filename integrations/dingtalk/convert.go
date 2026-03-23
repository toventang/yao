package dingtalk

import (
	"encoding/json"
	"strings"
)

// ConvertedMessage is the unified output after parsing a DingTalk message.
type ConvertedMessage struct {
	MessageID        string      `json:"message_id"`
	ConversationID   string      `json:"conversation_id"`
	ConversationType string      `json:"conversation_type"` // "1" = private, "2" = group
	SenderID         string      `json:"sender_id"`
	SenderNick       string      `json:"sender_nick,omitempty"`
	SenderStaffID    string      `json:"sender_staff_id,omitempty"`
	Text             string      `json:"text,omitempty"`
	MediaItems       []MediaItem `json:"media,omitempty"`
	ChatbotUserID    string      `json:"chatbot_user_id,omitempty"`
	IsInAtList       bool        `json:"is_in_at_list,omitempty"`
	SessionWebhook   string      `json:"session_webhook,omitempty"`
}

// MediaItem describes a single attachment in a DingTalk message.
type MediaItem struct {
	Type     MediaType `json:"type"`
	URL      string    `json:"url,omitempty"`
	MimeType string    `json:"mime_type,omitempty"`
	FileName string    `json:"file_name,omitempty"`
	Wrapper  string    `json:"wrapper,omitempty"`
}

// MediaType indicates the attachment type.
type MediaType string

const (
	MediaImage    MediaType = "image"
	MediaFile     MediaType = "file"
	MediaAudio    MediaType = "audio"
	MediaVideo    MediaType = "video"
	MediaRichText MediaType = "richText"
)

// HasMedia returns true if the message contains media.
func (cm *ConvertedMessage) HasMedia() bool { return len(cm.MediaItems) > 0 }

// HasText returns true if the message contains text.
func (cm *ConvertedMessage) HasText() bool { return cm.Text != "" }

// StreamCallbackData is the data structure from DingTalk stream callback.
type StreamCallbackData struct {
	ConversationID            string          `json:"conversationId"`
	ConversationType          string          `json:"conversationType"`
	AtUsers                   []AtUser        `json:"atUsers"`
	ChatbotCorpID             string          `json:"chatbotCorpId"`
	ChatbotUserID             string          `json:"chatbotUserId"`
	MsgID                     string          `json:"msgId"`
	SenderID                  string          `json:"senderId"`
	SenderNick                string          `json:"senderNick"`
	SenderCorpID              string          `json:"senderCorpId"`
	SenderStaffID             string          `json:"senderStaffId"`
	SessionWebhook            string          `json:"sessionWebhook"`
	SessionWebhookExpiredTime int64           `json:"sessionWebhookExpiredTime"`
	IsAdmin                   bool            `json:"isAdmin"`
	IsInAtList                bool            `json:"isInAtList"`
	Text                      *TextContent    `json:"text,omitempty"`
	Msgtype                   string          `json:"msgtype"`
	RichText                  json.RawMessage `json:"richText,omitempty"`
}

// AtUser represents a mentioned user in a DingTalk message.
type AtUser struct {
	DingtalkID string `json:"dingtalkId"`
	StaffID    string `json:"staffId,omitempty"`
}

// TextContent holds plain text content.
type TextContent struct {
	Content string `json:"content"`
}

// ConvertStreamData transforms a DingTalk stream callback into a ConvertedMessage.
func ConvertStreamData(data *StreamCallbackData) *ConvertedMessage {
	if data == nil {
		return nil
	}

	cm := &ConvertedMessage{
		MessageID:        data.MsgID,
		ConversationID:   data.ConversationID,
		ConversationType: data.ConversationType,
		SenderID:         data.SenderID,
		SenderNick:       data.SenderNick,
		SenderStaffID:    data.SenderStaffID,
		ChatbotUserID:    data.ChatbotUserID,
		IsInAtList:       data.IsInAtList,
		SessionWebhook:   data.SessionWebhook,
	}

	switch data.Msgtype {
	case "text":
		if data.Text != nil {
			text := strings.TrimSpace(data.Text.Content)
			cm.Text = text
		}
	case "richText":
		if len(data.RichText) > 0 {
			text, media := parseRichText(data.RichText)
			cm.Text = text
			cm.MediaItems = media
		}
	case "picture":
		cm.MediaItems = append(cm.MediaItems, MediaItem{Type: MediaImage})
	}

	return cm
}

func parseRichText(raw json.RawMessage) (string, []MediaItem) {
	var richText struct {
		RichText []struct {
			Text    string `json:"text,omitempty"`
			PicURL  string `json:"pictureDownloadUrl,omitempty"`
			Type    string `json:"type,omitempty"`
			DownURL string `json:"downloadCode,omitempty"`
		} `json:"richText"`
	}
	if err := json.Unmarshal(raw, &richText); err != nil {
		return "", nil
	}

	var text string
	var media []MediaItem
	for _, item := range richText.RichText {
		if item.Text != "" {
			text += item.Text
		}
		if item.PicURL != "" {
			media = append(media, MediaItem{Type: MediaImage, URL: item.PicURL, MimeType: "image/jpeg"})
		}
	}
	return text, media
}
