package telegram

import (
	"sync"
	"time"
)

const (
	dedupTTL           = 24 * time.Hour
	dedupCleanInterval = time.Hour
)

// dedupStore is a lightweight in-memory deduplication store with TTL.
// Used for message-level dedup (same update_id won't be processed twice).
type dedupStore struct {
	m sync.Map // key -> int64 (unix timestamp)
}

func newDedupStore() *dedupStore {
	return &dedupStore{}
}

// markSeen returns true if this is the first time the key is seen.
func (d *dedupStore) markSeen(key string) bool {
	now := time.Now().Unix()
	_, loaded := d.m.LoadOrStore(key, now)
	return !loaded
}

// cleaner periodically removes expired entries. Runs until stopCh is closed.
func (d *dedupStore) cleaner(stopCh <-chan struct{}) {
	ticker := time.NewTicker(dedupCleanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-dedupTTL).Unix()
			d.m.Range(func(key, value any) bool {
				if ts, ok := value.(int64); ok && ts < cutoff {
					d.m.Delete(key)
				}
				return true
			})
		}
	}
}
