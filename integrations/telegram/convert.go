package telegram

import (
	"sort"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
)

// ConvertedMessage is the unified output of ConvertUpdate, usable by any
// consumer regardless of whether the Update came from GetUpdates or a webhook.
type ConvertedMessage struct {
	UpdateID     int64          `json:"update_id"`
	MessageID    int64          `json:"message_id"`
	ChatID       int64          `json:"chat_id"`
	ChatType     string         `json:"chat_type"`
	SenderID     int64          `json:"sender_id,omitempty"`
	SenderName   string         `json:"sender_name,omitempty"`
	LanguageCode string         `json:"language_code,omitempty"`
	Date         int            `json:"date"`
	Text         string         `json:"text,omitempty"`
	MediaItems   []MediaItem    `json:"media,omitempty"`
	ReplyTo      *ReplyInfo     `json:"reply_to,omitempty"`
	Raw          *models.Update `json:"-"`
}

// MediaItem describes a single media attachment extracted from the message.
type MediaItem struct {
	Type         MediaType `json:"type"`
	FileID       string    `json:"file_id"`
	FileUniqueID string    `json:"file_unique_id"`
	MimeType     string    `json:"mime_type"`
	FileName     string    `json:"file_name,omitempty"`
	FileSize     int64     `json:"file_size,omitempty"`
	Wrapper      string    `json:"wrapper,omitempty"` // __yao.attachment://xxx after ResolveMedia
}

// ReplyInfo holds info about the message being replied to.
type ReplyInfo struct {
	MessageID int64 `json:"message_id"`
	ChatID    int64 `json:"chat_id,omitempty"`
}

// ConvertUpdate transforms a raw Telegram Update into a ConvertedMessage.
// Returns nil if the update contains no processable message.
func ConvertUpdate(u *models.Update) *ConvertedMessage {
	msg := u.Message
	if msg == nil {
		return nil
	}

	cm := &ConvertedMessage{
		UpdateID:  int64(u.ID),
		MessageID: int64(msg.ID),
		ChatID:    msg.Chat.ID,
		ChatType:  string(msg.Chat.Type),
		Date:      msg.Date,
		Raw:       u,
	}

	if msg.From != nil {
		cm.SenderID = msg.From.ID
		cm.SenderName = buildSenderName(msg.From)
		cm.LanguageCode = msg.From.LanguageCode
	}

	if msg.Text != "" {
		cm.Text = ApplyEntities(msg.Text, msg.Entities)
	} else if msg.Caption != "" {
		cm.Text = ApplyEntities(msg.Caption, msg.CaptionEntities)
	}

	cm.MediaItems = extractMedia(msg)

	if msg.ReplyToMessage != nil {
		cm.ReplyTo = &ReplyInfo{
			MessageID: int64(msg.ReplyToMessage.ID),
			ChatID:    msg.ReplyToMessage.Chat.ID,
		}
	}

	return cm
}

// HasMedia returns true if the message contains any media attachments.
func (cm *ConvertedMessage) HasMedia() bool {
	return len(cm.MediaItems) > 0
}

// HasText returns true if the message contains text content.
func (cm *ConvertedMessage) HasText() bool {
	return cm.Text != ""
}

func buildSenderName(u *models.User) string {
	name := u.FirstName
	if u.LastName != "" {
		name += " " + u.LastName
	}
	return name
}

func extractMedia(msg *models.Message) []MediaItem {
	var items []MediaItem

	if len(msg.Photo) > 0 {
		best := pickBestPhoto(msg.Photo)
		items = append(items, MediaItem{
			Type:         MediaPhoto,
			FileID:       best.FileID,
			FileUniqueID: best.FileUniqueID,
			MimeType:     "image/jpeg",
			FileName:     "photo.jpg",
			FileSize:     int64(best.FileSize),
		})
	}

	if msg.Document != nil {
		d := msg.Document
		mime := d.MimeType
		if mime == "" {
			mime = "application/octet-stream"
		}
		name := d.FileName
		if name == "" {
			name = "document"
		}
		items = append(items, MediaItem{
			Type:         MediaDocument,
			FileID:       d.FileID,
			FileUniqueID: d.FileUniqueID,
			MimeType:     mime,
			FileName:     name,
			FileSize:     int64(d.FileSize),
		})
	}

	if msg.Audio != nil {
		a := msg.Audio
		mime := a.MimeType
		if mime == "" {
			mime = "audio/mpeg"
		}
		name := a.FileName
		if name == "" {
			name = "audio.mp3"
		}
		items = append(items, MediaItem{
			Type:         MediaAudio,
			FileID:       a.FileID,
			FileUniqueID: a.FileUniqueID,
			MimeType:     mime,
			FileName:     name,
			FileSize:     int64(a.FileSize),
		})
	}

	if msg.Voice != nil {
		v := msg.Voice
		mime := v.MimeType
		if mime == "" {
			mime = "audio/ogg"
		}
		items = append(items, MediaItem{
			Type:         MediaVoice,
			FileID:       v.FileID,
			FileUniqueID: v.FileUniqueID,
			MimeType:     mime,
			FileName:     "voice.ogg",
			FileSize:     int64(v.FileSize),
		})
	}

	if msg.Video != nil {
		v := msg.Video
		mime := v.MimeType
		if mime == "" {
			mime = "video/mp4"
		}
		name := v.FileName
		if name == "" {
			name = "video.mp4"
		}
		items = append(items, MediaItem{
			Type:         MediaVideo,
			FileID:       v.FileID,
			FileUniqueID: v.FileUniqueID,
			MimeType:     mime,
			FileName:     name,
			FileSize:     int64(v.FileSize),
		})
	}

	if msg.Animation != nil {
		a := msg.Animation
		mime := a.MimeType
		if mime == "" {
			mime = "video/mp4"
		}
		name := a.FileName
		if name == "" {
			name = "animation.mp4"
		}
		items = append(items, MediaItem{
			Type:         MediaAnimation,
			FileID:       a.FileID,
			FileUniqueID: a.FileUniqueID,
			MimeType:     mime,
			FileName:     name,
			FileSize:     int64(a.FileSize),
		})
	}

	if msg.Sticker != nil {
		s := msg.Sticker
		items = append(items, MediaItem{
			Type:         MediaSticker,
			FileID:       s.FileID,
			FileUniqueID: s.FileUniqueID,
			MimeType:     "image/webp",
			FileName:     "sticker.webp",
			FileSize:     int64(s.FileSize),
		})
	}

	return items
}

func pickBestPhoto(photos []models.PhotoSize) models.PhotoSize {
	if len(photos) == 0 {
		return models.PhotoSize{}
	}
	sorted := make([]models.PhotoSize, len(photos))
	copy(sorted, photos)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Width*sorted[i].Height > sorted[j].Width*sorted[j].Height
	})
	return sorted[0]
}

// ApplyEntities converts Telegram MessageEntity formatting to Markdown.
func ApplyEntities(text string, entities []models.MessageEntity) string {
	if len(entities) == 0 {
		return text
	}

	runes := []rune(text)

	sorted := make([]models.MessageEntity, len(entities))
	copy(sorted, entities)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Offset > sorted[j].Offset
	})

	for _, e := range sorted {
		start := e.Offset
		end := e.Offset + e.Length
		if start < 0 || end > len(runes) {
			continue
		}
		segment := string(runes[start:end])

		var replacement string
		switch e.Type {
		case "bold":
			replacement = "**" + segment + "**"
		case "italic":
			replacement = "_" + segment + "_"
		case "underline":
			replacement = "__" + segment + "__"
		case "strikethrough":
			replacement = "~~" + segment + "~~"
		case "code":
			replacement = "`" + segment + "`"
		case "pre":
			lang := ""
			if e.Language != "" {
				lang = e.Language
			}
			replacement = "```" + lang + "\n" + segment + "\n```"
		case "text_link":
			replacement = "[" + segment + "](" + e.URL + ")"
		case "text_mention":
			if e.User != nil {
				replacement = "[" + segment + "](tg://user?id=" + strconv.FormatInt(e.User.ID, 10) + ")"
			} else {
				replacement = segment
			}
		default:
			continue
		}

		runes = append(runes[:start], append([]rune(replacement), runes[end:]...)...)
	}

	return strings.TrimSpace(string(runes))
}
