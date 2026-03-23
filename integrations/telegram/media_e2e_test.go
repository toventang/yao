package telegram

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaoapp/yao/attachment"
)

type mediaTestCase struct {
	name      string
	file      string // relative to testdata/
	mimeType  string
	mediaType MediaType
}

var mediaTestCases = []mediaTestCase{
	{"jpg", "test.jpg", "image/jpeg", MediaPhoto},
	{"png", "test.png", "image/png", MediaPhoto},
	{"gif", "test.gif", "image/gif", MediaAnimation},
	{"webp", "test.webp", "image/webp", MediaSticker},
	{"mp3", "test.mp3", "audio/mpeg", MediaAudio},
	{"ogg", "test.ogg", "audio/ogg", MediaVoice},
	{"mp4", "test.mp4", "video/mp4", MediaVideo},
	{"pdf", "test.pdf", "application/pdf", MediaDocument},
	{"docx", "test.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", MediaDocument},
	{"pptx", "test.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", MediaDocument},
}

func readTestFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../testdata/" + name)
	if err != nil {
		t.Fatalf("read ../testdata/%s: %v", name, err)
	}
	if len(data) == 0 {
		t.Fatalf("../testdata/%s is empty", name)
	}
	return data
}

func TestE2E_08_SendMediaByReader_MultiType(t *testing.T) {
	skipIfNoToken(t)
	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	chatID := findChatID(t, b, ctx)
	if chatID == 0 {
		t.Fatal("no chat_id")
	}

	for _, tc := range mediaTestCases {
		t.Run(tc.name, func(t *testing.T) {
			data := readTestFile(t, tc.file)

			detected := DetectMediaType(tc.mimeType)
			if detected != tc.mediaType {
				t.Errorf("DetectMediaType(%q) = %q, want %q", tc.mimeType, detected, tc.mediaType)
			}

			err := b.SendMediaByReader(ctx, chatID, tc.mediaType, tc.file, bytes.NewReader(data), fmt.Sprintf("E2E %s %d bytes", tc.name, len(data)), 0)
			if err != nil {
				t.Fatalf("SendMediaByReader(%s) failed: %v", tc.name, err)
			}
			t.Logf("OK  %s %s %d bytes -> chat=%d", tc.name, tc.mimeType, len(data), chatID)
		})
	}
}

func TestE2E_09_SendMedia_Wrapper_MultiType(t *testing.T) {
	skipIfNoToken(t)
	prepare(t)
	defer cleanup()

	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	chatID := findChatID(t, b, ctx)
	if chatID == 0 {
		t.Fatal("no chat_id")
	}

	manager, exists := attachment.Managers[defaultUploader]
	if !exists {
		t.Fatal("attachment manager not found")
	}

	for _, tc := range mediaTestCases {
		t.Run(tc.name, func(t *testing.T) {
			data := readTestFile(t, tc.file)

			header := &attachment.FileHeader{
				FileHeader: &multipart.FileHeader{
					Filename: tc.file,
					Size:     int64(len(data)),
					Header:   make(textproto.MIMEHeader),
				},
			}
			header.Header.Set("Content-Type", tc.mimeType)

			uploaded, err := manager.Upload(ctx, header, bytes.NewReader(data), attachment.UploadOption{
				OriginalFilename: tc.file,
				Groups:           []string{"telegram", "e2e-media"},
			})
			if err != nil {
				t.Fatalf("upload %s: %v", tc.name, err)
			}

			wrapper := fmt.Sprintf("%s://%s", defaultUploader, uploaded.ID)

			err = b.SendMedia(ctx, chatID, wrapper, fmt.Sprintf("E2E wrapper %s", tc.name), 0)
			if err != nil {
				t.Fatalf("SendMedia(%s) failed: %v", tc.name, err)
			}
			t.Logf("OK  %s -> %s -> chat=%d", tc.name, wrapper, chatID)
		})
	}
}

// TestE2E_10_Receive_DownloadAndStore_Dedup pulls updates from the bot,
// finds media messages (seeded by TestE2E_00_Seed), and for each one:
//  1. DownloadAndStore -> verify wrapper format + stored bytes > 0
//  2. Read back from attachment manager -> verify content non-empty
//  3. Call DownloadAndStore again -> verify same wrapper (fingerprint dedup)
func TestE2E_10_Receive_DownloadAndStore_Dedup(t *testing.T) {
	skipIfNoToken(t)
	prepare(t)
	defer cleanup()

	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	updates := fetchUpdates(t, b, ctx)
	if len(updates) == 0 {
		t.Fatal("no updates")
	}

	groups := []string{"telegram", "e2e-recv"}
	manager := attachment.Managers[defaultUploader]
	if manager == nil {
		t.Fatal("attachment manager not found")
	}

	type mediaHit struct {
		kind         string
		fileID       string
		fileUniqueID string
		mimeType     string
		filename     string
	}

	var hits []mediaHit
	for _, cm := range updates {
		if cm == nil || !cm.HasMedia() {
			continue
		}
		for _, m := range cm.MediaItems {
			mime := m.MimeType
			if mime == "" {
				mime = "application/octet-stream"
			}
			name := m.FileName
			if name == "" {
				name = string(m.Type)
			}
			hits = append(hits, mediaHit{string(m.Type), m.FileID, m.FileUniqueID, mime, name})
		}
	}

	if len(hits) == 0 {
		t.Fatal("no media messages found in updates")
	}
	t.Logf("found %d media items in updates", len(hits))

	for i, h := range hits {
		t.Run(fmt.Sprintf("%s_%d", h.kind, i), func(t *testing.T) {
			// 1. DownloadAndStore
			result, err := b.DownloadAndStore(ctx, h.fileID, h.fileUniqueID, h.mimeType, h.filename, groups)
			if err != nil {
				t.Fatalf("DownloadAndStore: %v", err)
			}
			if result.Wrapper == "" {
				t.Fatal("wrapper is empty")
			}
			if !strings.HasPrefix(result.Wrapper, defaultUploader+"://") {
				t.Errorf("wrapper format: %s", result.Wrapper)
			}
			if result.FileName == "" {
				t.Error("filename is empty")
			}

			// 2. Read back from attachment
			_, fileID, ok := attachment.Parse(result.Wrapper)
			if !ok {
				t.Fatalf("failed to parse wrapper: %s", result.Wrapper)
			}
			resp, err := manager.Download(ctx, fileID)
			if err != nil {
				t.Fatalf("attachment Download: %v", err)
			}
			stored, err := io.ReadAll(resp.Reader)
			resp.Reader.Close()
			if err != nil {
				t.Fatalf("read stored: %v", err)
			}
			if len(stored) == 0 {
				t.Fatal("stored file is 0 bytes")
			}

			// 3. Dedup: same file_unique_id -> same wrapper
			result2, err := b.DownloadAndStore(ctx, h.fileID, h.fileUniqueID, h.mimeType, h.filename, groups)
			if err != nil {
				t.Fatalf("DownloadAndStore dedup: %v", err)
			}
			if result2.Wrapper != result.Wrapper {
				t.Errorf("dedup failed: %s vs %s", result.Wrapper, result2.Wrapper)
			}

			t.Logf("OK  %s unique=%s wrapper=%s stored=%d dedup=ok",
				h.kind, h.fileUniqueID, result.Wrapper, len(stored))
		})
	}
}

// TestE2E_99_Offset_Confirm runs last (highest number, file sorted after e2e_test.go).
// It validates offset-based acknowledgement and confirm semantics:
//  1. offset=last_id returns from last_id onwards (confirms ids < last_id)
//  2. offset=last_id+1 confirms all, subsequent offset=0 returns nothing old
func TestE2E_99_Offset_Confirm(t *testing.T) {
	skipIfNoToken(t)
	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	updates, err := b.GetRawUpdates(ctx, 0, 1)
	if err != nil {
		t.Fatalf("initial pull: %v", err)
	}
	if len(updates) == 0 {
		t.Skip("no pending updates to test offset confirm")
	}
	firstID := int64(updates[0].ID)
	lastID := int64(updates[len(updates)-1].ID)
	t.Logf("pending updates: count=%d first_id=%d last_id=%d", len(updates), firstID, lastID)

	// offset = last_id: confirms everything with id < last_id, returns from last_id
	partial, err := b.GetRawUpdates(ctx, lastID, 1)
	if err != nil {
		t.Fatalf("GetUpdates(offset=last_id=%d): %v", lastID, err)
	}
	if len(partial) == 0 {
		t.Error("offset=last_id returned 0 updates, expected at least 1")
	}
	if len(partial) > 0 && int64(partial[0].ID) != lastID {
		t.Errorf("offset=last_id: first_id=%d, want %d", partial[0].ID, lastID)
	}
	for _, p := range partial {
		if int64(p.ID) < lastID {
			t.Errorf("offset=last_id: got update_id=%d < %d", p.ID, lastID)
		}
	}
	t.Logf("offset=last_id(%d): returned=%d, first_id=%d", lastID, len(partial), partial[0].ID)

	// offset = last_id+1: confirms all remaining updates
	confirmOffset := lastID + 1
	afterConfirm, err := b.GetRawUpdates(ctx, confirmOffset, 1)
	if err != nil {
		t.Fatalf("GetUpdates(offset=%d): %v", confirmOffset, err)
	}
	for _, ac := range afterConfirm {
		if int64(ac.ID) <= lastID {
			t.Errorf("post-confirm: got update_id=%d <= %d", ac.ID, lastID)
		}
	}
	t.Logf("confirm offset=%d: returned=%d new", confirmOffset, len(afterConfirm))

	// Re-pull offset=0: old updates should be purged
	repull, err := b.GetRawUpdates(ctx, 0, 1)
	if err != nil {
		t.Fatalf("GetUpdates(offset=0 post-confirm): %v", err)
	}
	for _, r := range repull {
		if int64(r.ID) <= lastID {
			t.Errorf("post-confirm offset=0: stale update_id=%d (expected > %d)", r.ID, lastID)
		}
	}
	t.Logf("post-confirm offset=0: returned=%d (old updates purged)", len(repull))
}
