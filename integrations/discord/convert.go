package discord

import (
	"github.com/bwmarrin/discordgo"
)

// ConvertedMessage is the unified output after parsing a Discord message event.
type ConvertedMessage struct {
	MessageID  string      `json:"message_id"`
	ChannelID  string      `json:"channel_id"`
	GuildID    string      `json:"guild_id,omitempty"`
	AuthorID   string      `json:"author_id"`
	AuthorName string      `json:"author_name,omitempty"`
	IsBot      bool        `json:"is_bot"`
	Text       string      `json:"text,omitempty"`
	MediaItems []MediaItem `json:"media,omitempty"`
	Locale     string      `json:"locale,omitempty"`
	ReplyTo    string      `json:"reply_to,omitempty"`
	IsDM       bool        `json:"is_dm"`
}

// MediaItem describes a single attachment from a Discord message.
type MediaItem struct {
	Type        MediaType `json:"type"`
	URL         string    `json:"url"`
	ProxyURL    string    `json:"proxy_url,omitempty"`
	FileName    string    `json:"file_name"`
	ContentType string    `json:"content_type,omitempty"`
	Size        int       `json:"size,omitempty"`
	Wrapper     string    `json:"wrapper,omitempty"`
}

// MediaType indicates the attachment type.
type MediaType string

const (
	MediaImage    MediaType = "image"
	MediaVideo    MediaType = "video"
	MediaAudio    MediaType = "audio"
	MediaDocument MediaType = "document"
)

// HasMedia returns true if the message contains media.
func (cm *ConvertedMessage) HasMedia() bool { return len(cm.MediaItems) > 0 }

// HasText returns true if the message contains text.
func (cm *ConvertedMessage) HasText() bool { return cm.Text != "" }

// ConvertMessageCreate transforms a discordgo MessageCreate event into a ConvertedMessage.
func ConvertMessageCreate(m *discordgo.MessageCreate) *ConvertedMessage {
	if m == nil || m.Message == nil {
		return nil
	}
	return ConvertMessage(m.Message)
}

// ConvertMessage transforms a discordgo Message into a ConvertedMessage.
func ConvertMessage(m *discordgo.Message) *ConvertedMessage {
	if m == nil {
		return nil
	}

	cm := &ConvertedMessage{
		MessageID: m.ID,
		ChannelID: m.ChannelID,
		GuildID:   m.GuildID,
		Text:      m.Content,
		IsDM:      m.GuildID == "",
	}

	if m.Author != nil {
		cm.AuthorID = m.Author.ID
		cm.AuthorName = m.Author.Username
		cm.IsBot = m.Author.Bot
		cm.Locale = m.Author.Locale
	}

	if m.MessageReference != nil {
		cm.ReplyTo = m.MessageReference.MessageID
	}

	for _, att := range m.Attachments {
		cm.MediaItems = append(cm.MediaItems, MediaItem{
			Type:        detectMediaType(att.ContentType),
			URL:         att.URL,
			ProxyURL:    att.ProxyURL,
			FileName:    att.Filename,
			ContentType: att.ContentType,
			Size:        att.Size,
		})
	}

	return cm
}

func detectMediaType(contentType string) MediaType {
	if contentType == "" {
		return MediaDocument
	}
	switch {
	case len(contentType) > 6 && contentType[:6] == "image/":
		return MediaImage
	case len(contentType) > 6 && contentType[:6] == "video/":
		return MediaVideo
	case len(contentType) > 6 && contentType[:6] == "audio/":
		return MediaAudio
	default:
		return MediaDocument
	}
}
