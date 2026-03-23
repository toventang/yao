package dingtalk

import (
	"testing"
)

func TestConvertStreamData_Text(t *testing.T) {
	data := &StreamCallbackData{
		MsgID:            "msg_001",
		ConversationID:   "cid_001",
		ConversationType: "1",
		SenderID:         "user_001",
		SenderNick:       "Test User",
		SessionWebhook:   "https://oapi.dingtalk.com/robot/sendBySession/xxx",
		Msgtype:          "text",
		Text:             &TextContent{Content: " Hello World "},
	}

	cm := ConvertStreamData(data)
	if cm == nil {
		t.Fatal("expected non-nil ConvertedMessage")
	}
	if cm.MessageID != "msg_001" {
		t.Errorf("expected msg_001, got %s", cm.MessageID)
	}
	if cm.Text != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", cm.Text)
	}
	if cm.ConversationID != "cid_001" {
		t.Errorf("expected cid_001, got %s", cm.ConversationID)
	}
	if cm.SenderNick != "Test User" {
		t.Errorf("expected 'Test User', got %s", cm.SenderNick)
	}
	if cm.SessionWebhook == "" {
		t.Error("expected non-empty SessionWebhook")
	}
}

func TestConvertStreamData_Nil(t *testing.T) {
	cm := ConvertStreamData(nil)
	if cm != nil {
		t.Error("nil input should return nil")
	}
}

func TestConvertedMessage_HasText(t *testing.T) {
	cm := &ConvertedMessage{}
	if cm.HasText() {
		t.Error("empty should not have text")
	}
	cm.Text = "hello"
	if !cm.HasText() {
		t.Error("should have text")
	}
}

func TestConvertedMessage_HasMedia(t *testing.T) {
	cm := &ConvertedMessage{}
	if cm.HasMedia() {
		t.Error("empty should not have media")
	}
	cm.MediaItems = append(cm.MediaItems, MediaItem{Type: MediaImage, URL: "http://example.com/img.jpg"})
	if !cm.HasMedia() {
		t.Error("should have media")
	}
}
