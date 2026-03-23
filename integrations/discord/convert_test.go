package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestConvertMessage_Text(t *testing.T) {
	m := &discordgo.Message{
		ID:        "msg_001",
		ChannelID: "ch_001",
		GuildID:   "guild_001",
		Content:   "Hello World",
		Author: &discordgo.User{
			ID:       "user_001",
			Username: "TestUser",
			Bot:      false,
		},
	}
	cm := ConvertMessage(m)
	if cm == nil {
		t.Fatal("expected non-nil ConvertedMessage")
	}
	if cm.MessageID != "msg_001" {
		t.Errorf("expected msg_001, got %s", cm.MessageID)
	}
	if cm.Text != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", cm.Text)
	}
	if cm.AuthorID != "user_001" {
		t.Errorf("expected user_001, got %s", cm.AuthorID)
	}
	if cm.AuthorName != "TestUser" {
		t.Errorf("expected TestUser, got %s", cm.AuthorName)
	}
	if cm.IsBot {
		t.Error("expected IsBot=false")
	}
	if cm.IsDM {
		t.Error("expected IsDM=false for guild message")
	}
	if !cm.HasText() {
		t.Error("expected HasText=true")
	}
	if cm.HasMedia() {
		t.Error("expected HasMedia=false")
	}
}

func TestConvertMessage_DM(t *testing.T) {
	m := &discordgo.Message{
		ID:        "msg_002",
		ChannelID: "ch_dm",
		Content:   "DM message",
		Author: &discordgo.User{
			ID:       "user_002",
			Username: "DMUser",
		},
	}
	cm := ConvertMessage(m)
	if cm == nil {
		t.Fatal("expected non-nil")
	}
	if !cm.IsDM {
		t.Error("expected IsDM=true for message without GuildID")
	}
}

func TestConvertMessage_WithAttachments(t *testing.T) {
	m := &discordgo.Message{
		ID:        "msg_003",
		ChannelID: "ch_003",
		Content:   "Check this out",
		Author: &discordgo.User{
			ID:       "user_003",
			Username: "FileUser",
		},
		Attachments: []*discordgo.MessageAttachment{
			{
				ID:          "att_001",
				URL:         "https://cdn.discordapp.com/attachments/test.png",
				ProxyURL:    "https://media.discordapp.net/attachments/test.png",
				Filename:    "test.png",
				ContentType: "image/png",
				Size:        1024,
			},
			{
				ID:          "att_002",
				URL:         "https://cdn.discordapp.com/attachments/report.pdf",
				Filename:    "report.pdf",
				ContentType: "application/pdf",
				Size:        2048,
			},
		},
	}
	cm := ConvertMessage(m)
	if cm == nil {
		t.Fatal("expected non-nil")
	}
	if !cm.HasText() {
		t.Error("expected HasText=true")
	}
	if !cm.HasMedia() {
		t.Error("expected HasMedia=true")
	}
	if len(cm.MediaItems) != 2 {
		t.Fatalf("expected 2 media items, got %d", len(cm.MediaItems))
	}
	if cm.MediaItems[0].Type != MediaImage {
		t.Errorf("expected image type, got %s", cm.MediaItems[0].Type)
	}
	if cm.MediaItems[0].FileName != "test.png" {
		t.Errorf("expected test.png, got %s", cm.MediaItems[0].FileName)
	}
	if cm.MediaItems[1].Type != MediaDocument {
		t.Errorf("expected document type, got %s", cm.MediaItems[1].Type)
	}
}

func TestConvertMessage_WithReply(t *testing.T) {
	m := &discordgo.Message{
		ID:        "msg_004",
		ChannelID: "ch_004",
		Content:   "Replying",
		Author:    &discordgo.User{ID: "user_004"},
		MessageReference: &discordgo.MessageReference{
			MessageID: "msg_original",
			ChannelID: "ch_004",
		},
	}
	cm := ConvertMessage(m)
	if cm == nil {
		t.Fatal("expected non-nil")
	}
	if cm.ReplyTo != "msg_original" {
		t.Errorf("expected ReplyTo=msg_original, got %s", cm.ReplyTo)
	}
}

func TestConvertMessage_Nil(t *testing.T) {
	cm := ConvertMessage(nil)
	if cm != nil {
		t.Error("nil input should return nil")
	}
}

func TestConvertMessageCreate_Nil(t *testing.T) {
	cm := ConvertMessageCreate(nil)
	if cm != nil {
		t.Error("nil input should return nil")
	}
}

func TestDetectMediaType(t *testing.T) {
	cases := []struct {
		input    string
		expected MediaType
	}{
		{"image/png", MediaImage},
		{"image/jpeg", MediaImage},
		{"video/mp4", MediaVideo},
		{"audio/mpeg", MediaAudio},
		{"application/pdf", MediaDocument},
		{"", MediaDocument},
	}
	for _, tc := range cases {
		got := detectMediaType(tc.input)
		if got != tc.expected {
			t.Errorf("detectMediaType(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
