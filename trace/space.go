package trace

import (
	"context"

	"github.com/yaoapp/yao/trace/types"
)

// space implements the Space interface for custom space operations.
// Uses context.Background() for driver calls to decouple from caller context
// (fixes the context-fork bug where parent cancellation breaks child ops).
type space struct {
	traceID string
	data    *types.TraceSpace
	driver  types.Driver
}

// NewSpace creates a new space instance
func NewSpace(traceID string, data *types.TraceSpace, driver types.Driver) types.Space {
	return &space{
		traceID: traceID,
		data:    data,
		driver:  driver,
	}
}

// ID returns the space identifier
func (s *space) ID() string {
	return s.data.ID
}

// Set stores a value by key
func (s *space) Set(key string, value any) error {
	return s.driver.SetSpaceKey(context.Background(), s.traceID, s.data.ID, key, value)
}

// Get retrieves a value by key
func (s *space) Get(key string) (any, error) {
	return s.driver.GetSpaceKey(context.Background(), s.traceID, s.data.ID, key)
}

// Has checks if a key exists
func (s *space) Has(key string) bool {
	return s.driver.HasSpaceKey(context.Background(), s.traceID, s.data.ID, key)
}

// Delete removes a key-value pair
func (s *space) Delete(key string) error {
	return s.driver.DeleteSpaceKey(context.Background(), s.traceID, s.data.ID, key)
}

// Clear removes all key-value pairs
func (s *space) Clear() error {
	return s.driver.ClearSpaceKeys(context.Background(), s.traceID, s.data.ID)
}

// Keys returns all keys in the space
func (s *space) Keys() []string {
	keys, err := s.driver.ListSpaceKeys(context.Background(), s.traceID, s.data.ID)
	if err != nil {
		return nil
	}
	return keys
}
