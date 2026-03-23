package telegram

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/yaoapp/yao/attachment"
	"github.com/yaoapp/yao/config"
	"github.com/yaoapp/yao/test"
)

var testBotToken string

func TestMain(m *testing.M) {
	testBotToken = os.Getenv("TELEGRAM_TEST_BOT_TOKEN")
	os.Exit(m.Run())
}

func prepare(t *testing.T) {
	t.Helper()
	test.Prepare(t, config.Conf)
	if err := attachment.Load(config.Conf); err != nil {
		t.Fatalf("load attachment: %v", err)
	}
}

func cleanup() {
	test.Clean()
}

func skipIfNoToken(t *testing.T) {
	t.Helper()
	if testBotToken == "" {
		t.Skip("TELEGRAM_TEST_BOT_TOKEN not set, skipping E2E test")
	}
}

func testBot(opts ...BotOption) *Bot {
	if host := os.Getenv("TELEGRAM_TEST_HOST"); host != "" {
		opts = append([]BotOption{WithAPIBase(host)}, opts...)
	}
	return NewBot(testBotToken, "", opts...)
}

// seedBotMessages uses the persisted MTProto user session to send a text
// message and a small PNG photo to the test bot, so that subsequent
// GetUpdates calls have real data to work with.
// Requires TG_TEST_SESSION and TG_TEST_BOT_USERNAME env vars.
func seedBotMessages(t *testing.T) {
	t.Helper()

	sessionPath := os.Getenv("TG_TEST_SESSION")
	botUsername := os.Getenv("TG_TEST_BOT_USERNAME")
	if sessionPath == "" || botUsername == "" {
		t.Skip("TG_TEST_SESSION or TG_TEST_BOT_USERNAME not set, cannot seed messages")
	}

	if !filepath.IsAbs(sessionPath) {
		if root := os.Getenv("YAO_DEV"); root != "" {
			sessionPath = filepath.Join(root, sessionPath)
		}
	}

	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		t.Skipf("session file %s not found, run tg-login first", sessionPath)
	}
	t.Logf("seed: using session %s, bot @%s", sessionPath, botUsername)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	storage := &session.FileStorage{Path: sessionPath}
	client := telegram.NewClient(17349, "344583e45741c457fe1862106095a5eb", telegram.Options{
		SessionStorage: storage,
	})

	err := client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("auth status: %w", err)
		}
		if !status.Authorized {
			return fmt.Errorf("not authorized — run tg-login first")
		}

		api := client.API()
		resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
			Username: botUsername,
		})
		if err != nil {
			return fmt.Errorf("resolve @%s: %w", botUsername, err)
		}
		if len(resolved.Users) == 0 {
			return fmt.Errorf("bot @%s not found", botUsername)
		}
		u, ok := resolved.Users[0].(*tg.User)
		if !ok {
			return fmt.Errorf("resolved entity is not a user")
		}
		peer := &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}

		tag := time.Now().Format("15:04:05")
		_, err = api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer:     peer,
			Message:  fmt.Sprintf("[e2e-test] hello at %s", tag),
			RandomID: mtpRandID(),
		})
		if err != nil {
			return fmt.Errorf("send text: %w", err)
		}
		t.Log("seed: sent text")

		up := uploader.NewUploader(api)

		seedFiles := []struct {
			path  string
			photo bool
			attrs []tg.DocumentAttributeClass
		}{
			{"../testdata/test.jpg", true, nil},
			{"../testdata/test.mp3", false, []tg.DocumentAttributeClass{
				&tg.DocumentAttributeAudio{Duration: 5, Title: "e2e-test"},
			}},
			{"../testdata/test.pdf", false, []tg.DocumentAttributeClass{
				&tg.DocumentAttributeFilename{FileName: "test.pdf"},
			}},
		}

		for _, sf := range seedFiles {
			f, err := up.FromPath(ctx, sf.path)
			if err != nil {
				return fmt.Errorf("upload %s: %w", sf.path, err)
			}
			var media tg.InputMediaClass
			if sf.photo {
				media = &tg.InputMediaUploadedPhoto{File: f}
			} else {
				media = &tg.InputMediaUploadedDocument{
					File:       f,
					Attributes: sf.attrs,
				}
			}
			_, err = api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
				Peer:     peer,
				Media:    media,
				Message:  fmt.Sprintf("[e2e-test] %s at %s", filepath.Base(sf.path), tag),
				RandomID: mtpRandID(),
			})
			if err != nil {
				return fmt.Errorf("send %s: %w", sf.path, err)
			}
			t.Logf("seed: sent %s", filepath.Base(sf.path))
		}

		time.Sleep(time.Second)
		return nil
	})
	if err != nil {
		t.Fatalf("seedBotMessages: %v", err)
	}
}

func mtpRandID() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return int64(binary.LittleEndian.Uint64(b[:]))
}
