package discord

import (
	"testing"
)

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
