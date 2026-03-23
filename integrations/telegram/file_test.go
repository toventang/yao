package telegram

import (
	"testing"
)

func TestFingerprintFileID(t *testing.T) {
	id1 := fingerprintFileID("unique1", []string{"telegram", "bot123"})
	id2 := fingerprintFileID("unique1", []string{"telegram", "bot123"})
	if id1 != id2 {
		t.Fatalf("same input should produce same fingerprint, got %s vs %s", id1, id2)
	}

	id3 := fingerprintFileID("unique2", []string{"telegram", "bot123"})
	if id1 == id3 {
		t.Fatalf("different file_unique_id should produce different fingerprint")
	}

	id4 := fingerprintFileID("unique1", []string{"telegram", "bot456"})
	if id1 == id4 {
		t.Fatalf("different groups should produce different fingerprint")
	}

	id5 := fingerprintFileID("unique1", nil)
	if id5 == "" {
		t.Fatal("nil groups should still produce a valid fingerprint")
	}
	if id5 == id1 {
		t.Fatal("nil groups vs non-nil groups should differ")
	}

	if len(id1) != 32 {
		t.Fatalf("fingerprint should be 32 hex chars (md5), got length %d", len(id1))
	}
}
