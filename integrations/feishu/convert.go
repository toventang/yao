package feishu

import (
	"encoding/json"
)

// ConvertedMessage is the unified output after parsing a Feishu event message.
type ConvertedMessage struct {
	MessageID    string      `json:"message_id"`
	ChatID       string      `json:"chat_id"`
	ChatType     string      `json:"chat_type"` // p2p, group
	SenderID     string      `json:"sender_id"`
	SenderName   string      `json:"sender_name,omitempty"`
	Text         string      `json:"text,omitempty"`
	MediaItems   []MediaItem `json:"media,omitempty"`
	MentionBot   bool        `json:"mention_bot,omitempty"`
	EventID      string      `json:"event_id,omitempty"`
	LanguageCode string      `json:"language_code,omitempty"`
}

// MediaItem describes a single media attachment from the message.
type MediaItem struct {
	Type     MediaType `json:"type"`
	Key      string    `json:"key"`
	MimeType string    `json:"mime_type,omitempty"`
	FileName string    `json:"file_name,omitempty"`
	FileSize int64     `json:"file_size,omitempty"`
	Wrapper  string    `json:"wrapper,omitempty"`
}

// MediaType indicates the attachment type.
type MediaType string

const (
	MediaImage MediaType = "image"
	MediaFile  MediaType = "file"
	MediaAudio MediaType = "audio"
	MediaVideo MediaType = "video"
	MediaMedia MediaType = "media"
)

// HasMedia returns true if the message contains any media.
func (cm *ConvertedMessage) HasMedia() bool { return len(cm.MediaItems) > 0 }

// HasText returns true if the message contains text.
func (cm *ConvertedMessage) HasText() bool { return cm.Text != "" }

// feishuTextContent is the JSON structure of a text-type message body.
type feishuTextContent struct {
	Text string `json:"text"`
}

// feishuImageContent is the JSON structure of an image-type message body.
type feishuImageContent struct {
	ImageKey string `json:"image_key"`
}

// feishuFileContent is the JSON structure of a file-type message body.
type feishuFileContent struct {
	FileKey  string `json:"file_key"`
	FileName string `json:"file_name"`
}

// feishuAudioContent is the JSON structure of an audio-type message body.
type feishuAudioContent struct {
	FileKey  string `json:"file_key"`
	Duration int    `json:"duration"`
}

// feishuMediaContent is the JSON structure of a media-type message body.
type feishuMediaContent struct {
	FileKey  string `json:"file_key"`
	FileName string `json:"file_name"`
	ImageKey string `json:"image_key"`
}

// ParseMessageContent parses a Feishu message body (JSON string) based on its type.
func ParseMessageContent(msgType, content string) (text string, media []MediaItem) {
	switch msgType {
	case "text":
		var tc feishuTextContent
		if err := json.Unmarshal([]byte(content), &tc); err == nil {
			text = tc.Text
		}
	case "image":
		var ic feishuImageContent
		if err := json.Unmarshal([]byte(content), &ic); err == nil && ic.ImageKey != "" {
			media = append(media, MediaItem{Type: MediaImage, Key: ic.ImageKey, MimeType: "image/jpeg"})
		}
	case "file":
		var fc feishuFileContent
		if err := json.Unmarshal([]byte(content), &fc); err == nil && fc.FileKey != "" {
			media = append(media, MediaItem{Type: MediaFile, Key: fc.FileKey, FileName: fc.FileName})
		}
	case "audio":
		var ac feishuAudioContent
		if err := json.Unmarshal([]byte(content), &ac); err == nil && ac.FileKey != "" {
			media = append(media, MediaItem{Type: MediaAudio, Key: ac.FileKey, MimeType: "audio/ogg"})
		}
	case "media":
		var mc feishuMediaContent
		if err := json.Unmarshal([]byte(content), &mc); err == nil && mc.FileKey != "" {
			media = append(media, MediaItem{Type: MediaVideo, Key: mc.FileKey, FileName: mc.FileName})
		}
	case "post":
		var post map[string]interface{}
		if err := json.Unmarshal([]byte(content), &post); err == nil {
			text = extractPostText(post)
		}
	}
	return
}

// extractPostText extracts plain text from a rich-text (post) message.
func extractPostText(post map[string]interface{}) string {
	for _, langContent := range post {
		lc, ok := langContent.(map[string]interface{})
		if !ok {
			continue
		}
		contentArr, ok := lc["content"].([]interface{})
		if !ok {
			continue
		}
		var result string
		for _, para := range contentArr {
			paraArr, ok := para.([]interface{})
			if !ok {
				continue
			}
			for _, elem := range paraArr {
				elemMap, ok := elem.(map[string]interface{})
				if !ok {
					continue
				}
				tag, _ := elemMap["tag"].(string)
				if tag == "text" {
					if t, ok := elemMap["text"].(string); ok {
						result += t
					}
				} else if tag == "a" {
					if t, ok := elemMap["text"].(string); ok {
						result += t
					}
				}
			}
			result += "\n"
		}
		if result != "" {
			return result
		}
	}
	return ""
}
