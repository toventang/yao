package telegram

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

// TestE2E_Webhook requires a local telegram-bot-api server (TELEGRAM_TEST_HOST)
// because the official Telegram API only accepts HTTPS webhooks on public IPs.
// A local Bot API server can deliver webhooks to http://127.0.0.1.
//
// Flow:
//  1. Start a local HTTP server on a random port
//  2. SetWebhook to http://127.0.0.1:{port}/webhook with a secret token
//  3. Seed a message via MTProto
//  4. Wait for the webhook to deliver the Update
//  5. Verify: ParseWebhookPayload succeeds, secret header correct, Update fields valid
//  6. DeleteWebhook to restore polling mode
func TestE2E_Webhook(t *testing.T) {
	skipIfNoToken(t)

	host := os.Getenv("TELEGRAM_TEST_HOST")
	if host == "" {
		t.Skip("TELEGRAM_TEST_HOST not set — need local bot-api server for webhook test")
	}

	const secret = "e2e-webhook-test-secret"
	b := testBot(WithAPIBase(host))
	bWithSecret := NewBot(testBotToken, secret, WithAPIBase(host))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// --- 1. Start local webhook receiver ---
	var (
		mu          sync.Mutex
		received    []webhookHit
		wrongSecret int
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		headerSec := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		cm, err := bWithSecret.ParseWebhookPayload(r.Context(), r, nil)

		mu.Lock()
		defer mu.Unlock()

		if err != nil {
			wrongSecret++
			t.Logf("webhook: rejected request (secret=%q err=%v)", headerSec, err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		received = append(received, webhookHit{
			secret: headerSec,
			cm:     cm,
		})
		if cm != nil {
			t.Logf("webhook: accepted update_id=%d secret=%q", cm.UpdateID, headerSec)
		} else {
			t.Logf("webhook: accepted (no processable message) secret=%q", headerSec)
		}
		w.WriteHeader(http.StatusOK)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{Handler: mux}
	go server.Serve(listener)
	defer server.Close()
	t.Logf("webhook server listening on 127.0.0.1:%d", port)

	// --- 2. SetWebhook ---
	webhookURL := fmt.Sprintf("http://127.0.0.1:%d/webhook", port)
	if err := bWithSecret.SetWebhook(ctx, webhookURL, []string{"message"}); err != nil {
		t.Fatalf("SetWebhook: %v", err)
	}
	t.Logf("SetWebhook -> %s", webhookURL)

	defer func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		if err := b.DeleteWebhook(cleanCtx, true); err != nil {
			t.Logf("warning: DeleteWebhook failed: %v", err)
		} else {
			t.Log("DeleteWebhook -> ok (polling restored)")
		}
	}()

	// --- 3. Seed a message ---
	seedBotMessages(t)

	// --- 4. Wait for webhook delivery ---
	deadline := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			mu.Lock()
			count := len(received)
			mu.Unlock()
			if count == 0 {
				t.Fatal("timeout: no webhook updates received within 30s")
			}
			goto verify
		case <-ticker.C:
			mu.Lock()
			count := len(received)
			mu.Unlock()
			if count >= 2 {
				time.Sleep(2 * time.Second)
				goto verify
			}
		}
	}

verify:
	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("no webhook updates received")
	}
	t.Logf("total webhook hits: %d (rejected: %d)", len(received), wrongSecret)

	// --- 5. Validate received updates ---
	for i, hit := range received {
		t.Run(fmt.Sprintf("update_%d", i), func(t *testing.T) {
			if hit.secret != secret {
				t.Errorf("secret header = %q, want %q", hit.secret, secret)
			}

			cm := hit.cm
			if cm == nil {
				t.Log("webhook hit has no processable message (cm nil)")
				return
			}

			if cm.UpdateID == 0 {
				t.Error("ConvertedMessage.UpdateID should not be 0")
			}
			if cm.MessageID == 0 {
				t.Error("ConvertedMessage.MessageID should not be 0")
			}
			if cm.ChatID == 0 {
				t.Error("ConvertedMessage.ChatID should not be 0")
			}
			if cm.Date == 0 {
				t.Error("ConvertedMessage.Date should not be 0")
			}
			if cm.SenderID == 0 {
				t.Error("ConvertedMessage.SenderID should not be 0")
			}
			if cm.SenderName == "" {
				t.Error("ConvertedMessage.SenderName should not be empty")
			}
			if !cm.HasText() && !cm.HasMedia() {
				t.Error("message has neither text nor media")
			}

			for j, mi := range cm.MediaItems {
				if mi.FileID == "" {
					t.Errorf("media[%d].FileID should not be empty", j)
				}
				if mi.FileUniqueID == "" {
					t.Errorf("media[%d].FileUniqueID should not be empty", j)
				}
				if mi.MimeType == "" {
					t.Errorf("media[%d].MimeType should not be empty", j)
				}
				if mi.Type == "" {
					t.Errorf("media[%d].Type should not be empty", j)
				}
			}

			t.Logf("webhook update[%d] id=%d msg=%d chat=%d sender=%q text=%q media=%d",
				i, cm.UpdateID, cm.MessageID, cm.ChatID, cm.SenderName,
				truncate(cm.Text, 40), len(cm.MediaItems))
		})
	}

	// --- 6. Compare with GetUpdates ---
	// Delete webhook first, then seed + poll to verify ConvertUpdate consistency.
	t.Run("convert_consistency", func(t *testing.T) {
		delCtx, delCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer delCancel()
		if err := b.DeleteWebhook(delCtx, true); err != nil {
			t.Fatalf("DeleteWebhook for polling: %v", err)
		}
		time.Sleep(time.Second)

		seedBotMessages(t)
		time.Sleep(2 * time.Second)

		pollCtx, pollCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer pollCancel()
		polled := fetchUpdates(t, b, pollCtx)
		if len(polled) == 0 {
			t.Skip("no polled updates to compare")
		}

		polledCM := polled[0]
		if polledCM == nil {
			t.Skip("polled update has no message")
		}

		var webhookCM *ConvertedMessage
		for _, hit := range received {
			if hit.cm != nil {
				webhookCM = hit.cm
				break
			}
		}
		if webhookCM == nil {
			t.Skip("no webhook update with message")
		}

		if webhookCM.ChatID == 0 || polledCM.ChatID == 0 {
			t.Error("both sources should have non-zero ChatID")
		}
		if webhookCM.SenderID == 0 || polledCM.SenderID == 0 {
			t.Error("both sources should have non-zero SenderID")
		}
		if webhookCM.Date == 0 || polledCM.Date == 0 {
			t.Error("both sources should have non-zero Date")
		}
		if webhookCM.ChatType != polledCM.ChatType {
			t.Errorf("ChatType mismatch: webhook=%q polled=%q", webhookCM.ChatType, polledCM.ChatType)
		}

		webhookHasContent := webhookCM.HasText() || webhookCM.HasMedia()
		polledHasContent := polledCM.HasText() || polledCM.HasMedia()
		if !webhookHasContent {
			t.Error("webhook ConvertUpdate produced no content")
		}
		if !polledHasContent {
			t.Error("polled ConvertUpdate produced no content")
		}

		t.Logf("consistency OK: webhook(text=%v media=%d) polled(text=%v media=%d) chat_type=%s",
			webhookCM.HasText(), len(webhookCM.MediaItems),
			polledCM.HasText(), len(polledCM.MediaItems),
			polledCM.ChatType)
	})

	// Verify ParseWebhookPayload rejects wrong secrets
	t.Run("wrong_secret_rejected", func(t *testing.T) {
		fakeReq, _ := http.NewRequest("POST", "/webhook", nil)
		fakeReq.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong-secret")
		_, err := bWithSecret.ParseWebhookPayload(fakeReq.Context(), fakeReq, nil)
		if err == nil {
			t.Error("expected error for wrong secret, got nil")
		}
	})
}

type webhookHit struct {
	secret string
	cm     *ConvertedMessage
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
