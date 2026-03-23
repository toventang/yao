package telegram

import (
	"testing"
)

func TestVerifyWebhook_NoSecret(t *testing.T) {
	b := NewBot("token", "")
	if !b.VerifyWebhook("anything") {
		t.Fatal("should pass when no secret configured")
	}
	if !b.VerifyWebhook("") {
		t.Fatal("should pass with empty header when no secret configured")
	}
}

func TestVerifyWebhook_CorrectSecret(t *testing.T) {
	b := NewBot("token", "s3cr3t-t0ken")
	if !b.VerifyWebhook("s3cr3t-t0ken") {
		t.Fatal("should pass with matching secret")
	}
}

func TestVerifyWebhook_WrongSecret(t *testing.T) {
	b := NewBot("token", "s3cr3t-t0ken")
	if b.VerifyWebhook("wrong") {
		t.Fatal("should reject mismatched secret")
	}
	if b.VerifyWebhook("") {
		t.Fatal("should reject empty header when secret configured")
	}
}
