package trace

import (
	"fmt"

	"github.com/yaoapp/kun/log"
	"github.com/yaoapp/yao/trace/types"
)

// managerState holds all mutable state for a trace.
// Protected by manager.mu — all access goes through state* methods which acquire the lock.
type managerState struct {
	rootNode     *types.TraceNode
	currentNodes []*types.TraceNode
	spaces       map[string]*types.TraceSpace
	traceStatus  types.TraceStatus
	completed    bool
	updates      []*types.TraceUpdate
}

func (m *manager) stateSetRoot(node *types.TraceNode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.rootNode = node
}

func (m *manager) stateGetRoot() *types.TraceNode {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.rootNode
}

func (m *manager) stateSetCurrentNodes(nodes []*types.TraceNode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.currentNodes = nodes
}

func (m *manager) stateGetCurrentNodes() []*types.TraceNode {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.currentNodes == nil {
		return nil
	}
	nodes := make([]*types.TraceNode, len(m.state.currentNodes))
	copy(nodes, m.state.currentNodes)
	return nodes
}

func (m *manager) stateUpdateRootAndCurrent(root *types.TraceNode, current []*types.TraceNode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.rootNode = root
	m.state.currentNodes = current
}

func (m *manager) stateGetSpace(id string) (*types.TraceSpace, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	space, ok := m.state.spaces[id]
	return space, ok
}

func (m *manager) stateSetSpace(id string, space *types.TraceSpace) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.spaces[id] = space
}

func (m *manager) stateDeleteSpace(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.spaces, id)
}

func (m *manager) stateGetAllSpaces() []*types.TraceSpace {
	m.mu.Lock()
	defer m.mu.Unlock()
	spaces := make([]*types.TraceSpace, 0, len(m.state.spaces))
	for _, space := range m.state.spaces {
		spaces = append(spaces, space)
	}
	return spaces
}

func (m *manager) stateSetTraceStatus(status types.TraceStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.traceStatus = status
}

func (m *manager) stateGetTraceStatus() types.TraceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.traceStatus
}

func (m *manager) stateMarkCompleted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.completed {
		return false
	}
	m.state.completed = true
	return true
}

func (m *manager) stateIsCompleted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.completed
}

func (m *manager) stateAddUpdate(update *types.TraceUpdate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.updates = append(m.state.updates, update)
}

func (m *manager) stateGetUpdates(since int64) []*types.TraceUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	filtered := make([]*types.TraceUpdate, 0)
	for _, update := range m.state.updates {
		if update.Timestamp >= since {
			filtered = append(filtered, update)
		}
	}
	return filtered
}

func (m *manager) stateSetUpdates(updates []*types.TraceUpdate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	log.Trace("[STATE] stateSetUpdates: setting %d updates for trace %s", len(updates), m.traceID)
	m.state.updates = updates
}

// stateExecuteSpaceOp executes a space operation while holding the lock.
func (m *manager) stateExecuteSpaceOp(spaceID string, fn func() error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := fn()
	if err != nil {
		return fmt.Errorf("trace %s: space op failed: %w", m.traceID, err)
	}
	return nil
}
