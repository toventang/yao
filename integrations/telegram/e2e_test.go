package telegram

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaoapp/yao/attachment"
)

// TestE2E_00_Seed must run first (go test runs in lexical order).
// It sends a text + photo to the bot via MTProto so later tests have data.
func TestE2E_00_Seed(t *testing.T) {
	skipIfNoToken(t)
	seedBotMessages(t)
}

func TestE2E_01_GetMe(t *testing.T) {
	skipIfNoToken(t)
	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user, err := b.GetMe(ctx)
	if err != nil {
		t.Fatalf("GetMe failed: %v", err)
	}

	if user.ID == 0 {
		t.Error("user.ID should not be 0")
	}
	if !user.IsBot {
		t.Error("user.IsBot should be true")
	}
	if user.FirstName == "" {
		t.Error("user.FirstName should not be empty")
	}
	if user.Username == "" {
		t.Error("user.Username should not be empty")
	}

	expected := os.Getenv("TG_TEST_BOT_USERNAME")
	if expected != "" && user.Username != expected {
		t.Errorf("username mismatch: got %q, want %q", user.Username, expected)
	}

	if !user.CanJoinGroups {
		t.Log("warning: bot cannot join groups (CanJoinGroups=false)")
	}
	t.Logf("OK  id=%d username=%s first_name=%s can_read_all=%v",
		user.ID, user.Username, user.FirstName, user.CanReadAllGroupMessages)
}

func TestE2E_02_GetUpdates(t *testing.T) {
	skipIfNoToken(t)
	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	updates, err := b.GetRawUpdates(ctx, 0, 1)
	if err != nil {
		t.Fatalf("GetUpdates(offset=0): %v", err)
	}
	if len(updates) == 0 {
		t.Fatal("expected at least 1 update after seed, got 0")
	}

	var prevID int64
	for i, u := range updates {
		if u.ID == 0 {
			t.Errorf("updates[%d].ID should not be 0", i)
		}
		if u.ID < 0 {
			t.Errorf("updates[%d].ID=%d is negative", i, u.ID)
		}
		if i > 0 && int64(u.ID) <= prevID {
			t.Errorf("update_id not increasing: updates[%d].ID=%d <= prev=%d", i, u.ID, prevID)
		}
		prevID = int64(u.ID)

		if u.Message != nil {
			msg := u.Message
			if msg.ID == 0 {
				t.Errorf("updates[%d].Message.ID should not be 0", i)
			}
			if msg.Date == 0 {
				t.Errorf("updates[%d].Message.Date should not be 0", i)
			}
			if msg.Chat.ID == 0 {
				t.Errorf("updates[%d].Message.Chat.ID should not be 0", i)
			}
			if msg.Chat.Type == "" {
				t.Errorf("updates[%d].Message.Chat.Type should not be empty", i)
			}
			if msg.From != nil && msg.From.ID == 0 {
				t.Errorf("updates[%d].Message.From.ID should not be 0", i)
			}

			t.Logf("  update[%d] id=%d msg_id=%d chat=%d from=%d text=%q has_photo=%v has_doc=%v",
				i, u.ID, msg.ID, msg.Chat.ID, safeUserID(msg.From), truncate(msg.Text, 40),
				len(msg.Photo) > 0, msg.Document != nil)
		}
	}

	firstID := int64(updates[0].ID)
	lastID := int64(updates[len(updates)-1].ID)

	// offset = first_id: non-destructive, returns from first_id onwards
	updates2, err := b.GetRawUpdates(ctx, firstID, 1)
	if err != nil {
		t.Fatalf("GetUpdates(offset=first_id=%d): %v", firstID, err)
	}
	if len(updates2) == 0 {
		t.Error("offset=first_id returned 0 updates, expected >= 1")
	}
	if len(updates2) > 0 && int64(updates2[0].ID) != firstID {
		t.Errorf("offset=first_id: first returned update_id=%d, want %d", updates2[0].ID, firstID)
	}
	for _, u2 := range updates2 {
		if int64(u2.ID) < firstID {
			t.Errorf("offset=first_id: got update_id=%d < offset=%d", u2.ID, firstID)
		}
	}

	t.Logf("OK  total=%d first_id=%d last_id=%d monotonic=true offset_filter=ok",
		len(updates), firstID, lastID)
}

func TestE2E_03_GetFile_Download(t *testing.T) {
	skipIfNoToken(t)
	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var fileID string
	for i := 0; i < 3 && fileID == ""; i++ {
		if i > 0 {
			time.Sleep(time.Second)
		}
		fileID = findFileID(t, b, ctx)
	}
	if fileID == "" {
		t.Fatal("expected a photo/file from seed, got none")
	}

	fileMeta, err := b.GetFile(ctx, fileID)
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}
	if fileMeta.FileID == "" {
		t.Error("fileMeta.FileID should not be empty")
	}
	if fileMeta.FileID != fileID {
		t.Errorf("fileMeta.FileID=%q should match requested %q", fileMeta.FileID, fileID)
	}
	if fileMeta.FilePath == "" {
		t.Fatal("fileMeta.FilePath should not be empty")
	}
	if fileMeta.FileSize <= 0 {
		t.Error("fileMeta.FileSize should be > 0")
	}

	body, contentType, size, err := b.DownloadFile(ctx, fileMeta.FilePath)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("downloaded file has 0 bytes")
	}
	if contentType == "" {
		t.Error("content-type should not be empty")
	}

	t.Logf("OK  file_id=%s path=%s api_size=%d content_type=%s downloaded=%d",
		fileMeta.FileID, fileMeta.FilePath, size, contentType, len(data))
}

func TestE2E_04_DownloadAndStore(t *testing.T) {
	skipIfNoToken(t)
	prepare(t)
	defer cleanup()

	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	photoFileID, photoUniqueID := findPhotoIDs(t, b, ctx)
	if photoFileID == "" {
		t.Fatal("expected a photo from seed, got none")
	}

	groups := []string{"telegram", "e2e-test"}

	result, err := b.DownloadAndStore(ctx, photoFileID, photoUniqueID, "image/jpeg", "test_photo.jpg", groups)
	if err != nil {
		t.Fatalf("DownloadAndStore failed: %v", err)
	}
	if result.Wrapper == "" {
		t.Fatal("wrapper should not be empty")
	}
	if !strings.HasPrefix(result.Wrapper, defaultUploader+"://") {
		t.Errorf("wrapper should start with %s://, got %s", defaultUploader, result.Wrapper)
	}
	if result.MimeType == "" {
		t.Error("mime_type should not be empty")
	}
	if result.FileName == "" {
		t.Error("file_name should not be empty")
	}

	manager := attachment.Managers[defaultUploader]
	_, fileID, _ := attachment.Parse(result.Wrapper)
	resp, err := manager.Download(ctx, fileID)
	if err != nil {
		t.Fatalf("attachment.Download failed: %v", err)
	}
	defer resp.Reader.Close()
	stored, err := io.ReadAll(resp.Reader)
	if err != nil {
		t.Fatalf("read stored: %v", err)
	}
	if len(stored) == 0 {
		t.Fatal("stored file has 0 bytes")
	}
	t.Logf("OK  wrapper=%s stored_bytes=%d content_type=%s", result.Wrapper, len(stored), resp.ContentType)

	result2, err := b.DownloadAndStore(ctx, photoFileID, photoUniqueID, "image/jpeg", "test_photo.jpg", groups)
	if err != nil {
		t.Fatalf("DownloadAndStore (dedup) failed: %v", err)
	}
	if result2.Wrapper != result.Wrapper {
		t.Errorf("dedup failed: first=%s second=%s", result.Wrapper, result2.Wrapper)
	}
	t.Log("OK  dedup verified")
}

func TestE2E_05_SendMessage(t *testing.T) {
	skipIfNoToken(t)
	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	chatID := findChatID(t, b, ctx)
	if chatID == 0 {
		t.Fatal("expected chat_id from seed, got 0")
	}

	err := b.SendMessage(ctx, chatID, fmt.Sprintf("*E2E test* at `%s`", time.Now().Format(time.RFC3339)), 0)
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	t.Logf("OK  sent text message to chat=%d", chatID)
}

func TestE2E_06_SendMedia_Wrapper(t *testing.T) {
	skipIfNoToken(t)
	prepare(t)
	defer cleanup()

	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chatID := findChatID(t, b, ctx)
	if chatID == 0 {
		t.Fatal("expected chat_id from seed, got 0")
	}

	manager, exists := attachment.Managers[defaultUploader]
	if !exists {
		t.Fatal("attachment manager not found")
	}

	imgData := generateTestPNG()
	header := &attachment.FileHeader{
		FileHeader: &multipart.FileHeader{
			Filename: "e2e_test.png",
			Size:     int64(len(imgData)),
			Header:   make(textproto.MIMEHeader),
		},
	}
	header.Header.Set("Content-Type", "image/png")

	uploaded, err := manager.Upload(ctx, header, bytes.NewReader(imgData), attachment.UploadOption{
		OriginalFilename: "e2e_test.png",
		Groups:           []string{"telegram", "e2e-test"},
	})
	if err != nil {
		t.Fatalf("attachment upload failed: %v", err)
	}
	wrapper := fmt.Sprintf("%s://%s", defaultUploader, uploaded.ID)
	if uploaded.Bytes <= 0 {
		t.Error("uploaded.Bytes should be > 0")
	}
	t.Logf("uploaded wrapper=%s bytes=%d", wrapper, uploaded.Bytes)

	err = b.SendMedia(ctx, chatID, wrapper, "E2E attachment test", 0)
	if err != nil {
		t.Fatalf("SendMedia(wrapper) failed: %v", err)
	}
	t.Logf("OK  sent media via wrapper to chat=%d", chatID)
}

func TestE2E_07_SendMediaByReader(t *testing.T) {
	skipIfNoToken(t)
	b := testBot()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	chatID := findChatID(t, b, ctx)
	if chatID == 0 {
		t.Fatal("expected chat_id from seed, got 0")
	}

	imgData := generateTestPNG()
	err := b.SendMediaByReader(ctx, chatID, MediaPhoto, "e2e_test.png", bytes.NewReader(imgData), "E2E reader test", 0)
	if err != nil {
		t.Fatalf("SendMediaByReader failed: %v", err)
	}
	t.Logf("OK  sent photo via reader to chat=%d", chatID)
}

// --------------- helpers ---------------

func fetchUpdates(t *testing.T, b *Bot, ctx context.Context) []*ConvertedMessage {
	t.Helper()
	updates, err := b.GetUpdates(ctx, 0, 5, nil)
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	return updates
}

func findChatID(t *testing.T, b *Bot, ctx context.Context) int64 {
	t.Helper()
	for _, cm := range fetchUpdates(t, b, ctx) {
		if cm != nil && cm.ChatID != 0 {
			return cm.ChatID
		}
	}
	return 0
}

func findFileID(t *testing.T, b *Bot, ctx context.Context) string {
	t.Helper()
	for _, cm := range fetchUpdates(t, b, ctx) {
		if cm == nil {
			continue
		}
		for _, m := range cm.MediaItems {
			if m.Type == MediaPhoto || m.Type == MediaDocument {
				return m.FileID
			}
		}
	}
	return ""
}

func findPhotoIDs(t *testing.T, b *Bot, ctx context.Context) (fileID, uniqueID string) {
	t.Helper()
	for _, cm := range fetchUpdates(t, b, ctx) {
		if cm == nil {
			continue
		}
		for _, m := range cm.MediaItems {
			if m.Type == MediaPhoto {
				return m.FileID, m.FileUniqueID
			}
		}
	}
	return "", ""
}

func safeUserID(u *User) int64 {
	if u == nil {
		return 0
	}
	return u.ID
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// generateTestPNG produces a 100x100 red PNG that Telegram will accept.
func generateTestPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	red := color.RGBA{R: 255, A: 255}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, red)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
