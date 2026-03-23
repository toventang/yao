package telegram

import (
	"testing"
)

func TestDetectMediaType(t *testing.T) {
	cases := []struct {
		mime     string
		expected MediaType
	}{
		{"image/jpeg", MediaPhoto},
		{"image/png", MediaPhoto},
		{"IMAGE/PNG", MediaPhoto},
		{"image/gif", MediaAnimation},
		{"image/webp", MediaSticker},
		{"video/mp4", MediaVideo},
		{"video/webm", MediaVideo},
		{"audio/mpeg", MediaAudio},
		{"audio/mp3", MediaAudio},
		{"audio/ogg", MediaVoice},
		{"audio/ogg; codecs=opus", MediaVoice},
		{"application/pdf", MediaDocument},
		{"application/octet-stream", MediaDocument},
		{"text/plain", MediaDocument},
		{"", MediaDocument},
	}
	for _, tc := range cases {
		got := DetectMediaType(tc.mime)
		if got != tc.expected {
			t.Errorf("DetectMediaType(%q) = %q, want %q", tc.mime, got, tc.expected)
		}
	}
}

func TestParseWrapper(t *testing.T) {
	cases := []struct {
		input   string
		manager string
		fileID  string
		wantErr bool
	}{
		{"__yao.attachment://abc123", "__yao.attachment", "abc123", false},
		{"__custom.uploader://xyz", "__custom.uploader", "xyz", false},
		{"no-separator", "", "", true},
		{"://empty-manager", "", "empty-manager", false},
	}
	for _, tc := range cases {
		manager, fileID, err := parseWrapper(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseWrapper(%q) expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseWrapper(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if manager != tc.manager || fileID != tc.fileID {
			t.Errorf("parseWrapper(%q) = (%q, %q), want (%q, %q)", tc.input, manager, fileID, tc.manager, tc.fileID)
		}
	}
}
