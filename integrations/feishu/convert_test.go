package feishu

import (
	"testing"
)

func TestParseMessageContent_Text(t *testing.T) {
	text, media := ParseMessageContent("text", `{"text":"Hello World"}`)
	if text != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", text)
	}
	if len(media) != 0 {
		t.Errorf("expected 0 media, got %d", len(media))
	}
}

func TestParseMessageContent_Image(t *testing.T) {
	text, media := ParseMessageContent("image", `{"image_key":"img_abc123"}`)
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
	if len(media) != 1 {
		t.Fatalf("expected 1 media, got %d", len(media))
	}
	if media[0].Type != MediaImage {
		t.Errorf("expected type %s, got %s", MediaImage, media[0].Type)
	}
	if media[0].Key != "img_abc123" {
		t.Errorf("expected key img_abc123, got %s", media[0].Key)
	}
}

func TestParseMessageContent_File(t *testing.T) {
	text, media := ParseMessageContent("file", `{"file_key":"file_xyz","file_name":"report.pdf"}`)
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
	if len(media) != 1 {
		t.Fatalf("expected 1 media, got %d", len(media))
	}
	if media[0].Type != MediaFile {
		t.Errorf("expected type %s, got %s", MediaFile, media[0].Type)
	}
	if media[0].FileName != "report.pdf" {
		t.Errorf("expected filename report.pdf, got %s", media[0].FileName)
	}
}

func TestParseMessageContent_Audio(t *testing.T) {
	text, media := ParseMessageContent("audio", `{"file_key":"audio_key","duration":30}`)
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
	if len(media) != 1 {
		t.Fatalf("expected 1 media, got %d", len(media))
	}
	if media[0].Type != MediaAudio {
		t.Errorf("expected type %s, got %s", MediaAudio, media[0].Type)
	}
}

func TestParseMessageContent_Media(t *testing.T) {
	text, media := ParseMessageContent("media", `{"file_key":"media_key","file_name":"video.mp4","image_key":"cover"}`)
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
	if len(media) != 1 {
		t.Fatalf("expected 1 media, got %d", len(media))
	}
	if media[0].Type != MediaVideo {
		t.Errorf("expected type %s, got %s", MediaVideo, media[0].Type)
	}
}

func TestParseMessageContent_Post(t *testing.T) {
	content := `{"zh_cn":{"title":"Test","content":[[{"tag":"text","text":"Hello "},{"tag":"a","text":"World","href":"https://example.com"}]]}}`
	text, media := ParseMessageContent("post", content)
	if text == "" {
		t.Error("expected non-empty text from post message")
	}
	if len(media) != 0 {
		t.Errorf("expected 0 media from text-only post, got %d", len(media))
	}
}

func TestParseMessageContent_InvalidJSON(t *testing.T) {
	text, media := ParseMessageContent("text", "not json")
	if text != "" {
		t.Errorf("expected empty text for invalid JSON, got %q", text)
	}
	if len(media) != 0 {
		t.Errorf("expected 0 media for invalid JSON, got %d", len(media))
	}
}

func TestParseMessageContent_UnknownType(t *testing.T) {
	text, media := ParseMessageContent("unknown", `{"text":"Hello"}`)
	if text != "" {
		t.Errorf("expected empty text for unknown type, got %q", text)
	}
	if len(media) != 0 {
		t.Errorf("expected 0 media for unknown type, got %d", len(media))
	}
}

func TestConvertedMessage_HasMedia(t *testing.T) {
	cm := &ConvertedMessage{}
	if cm.HasMedia() {
		t.Error("empty message should not have media")
	}
	cm.MediaItems = append(cm.MediaItems, MediaItem{Type: MediaImage, Key: "test"})
	if !cm.HasMedia() {
		t.Error("message with media should report HasMedia=true")
	}
}

func TestConvertedMessage_HasText(t *testing.T) {
	cm := &ConvertedMessage{}
	if cm.HasText() {
		t.Error("empty message should not have text")
	}
	cm.Text = "hello"
	if !cm.HasText() {
		t.Error("message with text should report HasText=true")
	}
}
