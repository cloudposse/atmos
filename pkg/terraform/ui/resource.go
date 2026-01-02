package ui

import (
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ResourceTracker manages all resource operations.
type ResourceTracker struct {
	mu            sync.RWMutex
	resources     map[string]*ResourceOperation // keyed by address
	order         []string                      // maintains insertion order
	phase         Phase                         // current operation phase
	changeSummary *ChangeSummaryMessage         // plan/apply summary
	diagnostics   []*DiagnosticMessage          // warnings and errors
	version       *VersionMessage               // terraform version info
	outputs       *OutputsMessage               // output values after apply
}

// NewResourceTracker creates a new resource tracker.
func NewResourceTracker() *ResourceTracker {
	return &ResourceTracker{
		resources:   make(map[string]*ResourceOperation),
		order:       make([]string, 0),
		phase:       PhaseInitializing,
		diagnostics: make([]*DiagnosticMessage, 0),
	}
}

// HandleMessage processes a Terraform JSON message and updates state.
func (rt *ResourceTracker) HandleMessage(msg any) {
	defer perf.Track(nil, "terraform.ui.ResourceTracker.HandleMessage")()

	rt.mu.Lock()
	defer rt.mu.Unlock()

	switch m := msg.(type) {
	case *VersionMessage:
		rt.version = m

	case *PlannedChangeMessage:
		rt.handlePlannedChange(m)

	case *ApplyStartMessage:
		rt.handleApplyStart(m)

	case *ApplyProgressMessage:
		rt.handleApplyProgress(m)

	case *ApplyCompleteMessage:
		rt.handleApplyComplete(m)

	case *ApplyErroredMessage:
		rt.handleApplyErrored(m)

	case *RefreshStartMessage:
		rt.handleRefreshStart(m)

	case *RefreshCompleteMessage:
		rt.handleRefreshComplete(m)

	case *DiagnosticMessage:
		rt.diagnostics = append(rt.diagnostics, m)
		if m.Diagnostic.Severity == "error" {
			rt.phase = PhaseError
		}

	case *ChangeSummaryMessage:
		rt.changeSummary = m
		// Mark complete for both plan and apply operations.
		if (m.Changes.Operation == "apply" || m.Changes.Operation == "plan") && rt.phase != PhaseError {
			rt.phase = PhaseComplete
		}

	case *OutputsMessage:
		rt.outputs = m
	}
}

func (rt *ResourceTracker) handlePlannedChange(m *PlannedChangeMessage) {
	rt.phase = PhasePlanning
	addr := m.Change.Resource.Addr
	if _, exists := rt.resources[addr]; !exists {
		rt.order = append(rt.order, addr)
	}
	rt.resources[addr] = &ResourceOperation{
		Address:      addr,
		Module:       m.Change.Resource.Module,
		ResourceType: m.Change.Resource.ResourceType,
		ResourceName: m.Change.Resource.ResourceName,
		Action:       m.Change.Action,
		State:        ResourceStatePending,
	}
}

func (rt *ResourceTracker) handleApplyStart(m *ApplyStartMessage) {
	rt.phase = PhaseApplying
	addr := m.Hook.Resource.Addr
	if op, exists := rt.resources[addr]; exists {
		op.State = ResourceStateInProgress
		op.StartTime = time.Now()
		op.IDKey = m.Hook.IDKey
		op.IDValue = m.Hook.IDValue
	} else {
		// Resource wasn't in plan, add it now.
		rt.order = append(rt.order, addr)
		rt.resources[addr] = &ResourceOperation{
			Address:      addr,
			Module:       m.Hook.Resource.Module,
			ResourceType: m.Hook.Resource.ResourceType,
			ResourceName: m.Hook.Resource.ResourceName,
			Action:       m.Hook.Action,
			State:        ResourceStateInProgress,
			StartTime:    time.Now(),
			IDKey:        m.Hook.IDKey,
			IDValue:      m.Hook.IDValue,
		}
	}
}

func (rt *ResourceTracker) handleApplyProgress(m *ApplyProgressMessage) {
	addr := m.Hook.Resource.Addr
	if op, exists := rt.resources[addr]; exists {
		op.ElapsedSecs = m.Hook.ElapsedSecs
		op.LastUpdate = time.Now()
	}
}

func (rt *ResourceTracker) handleApplyComplete(m *ApplyCompleteMessage) {
	addr := m.Hook.Resource.Addr
	if op, exists := rt.resources[addr]; exists {
		op.State = ResourceStateComplete
		op.EndTime = time.Now()
		op.ElapsedSecs = m.Hook.ElapsedSecs
		op.IDKey = m.Hook.IDKey
		op.IDValue = m.Hook.IDValue
	}
}

func (rt *ResourceTracker) handleApplyErrored(m *ApplyErroredMessage) {
	addr := m.Hook.Resource.Addr
	if op, exists := rt.resources[addr]; exists {
		op.State = ResourceStateError
		op.EndTime = time.Now()
		op.ElapsedSecs = m.Hook.ElapsedSecs
		op.Error = m.Message
	}
	rt.phase = PhaseError
}

func (rt *ResourceTracker) handleRefreshStart(m *RefreshStartMessage) {
	rt.phase = PhaseRefreshing
	addr := m.Hook.Resource.Addr
	if op, exists := rt.resources[addr]; exists {
		op.State = ResourceStateRefreshing
		op.StartTime = time.Now()
	} else {
		rt.order = append(rt.order, addr)
		rt.resources[addr] = &ResourceOperation{
			Address:      addr,
			Module:       m.Hook.Resource.Module,
			ResourceType: m.Hook.Resource.ResourceType,
			ResourceName: m.Hook.Resource.ResourceName,
			Action:       "read",
			State:        ResourceStateRefreshing,
			StartTime:    time.Now(),
			IDKey:        m.Hook.IDKey,
			IDValue:      m.Hook.IDValue,
		}
	}
}

func (rt *ResourceTracker) handleRefreshComplete(m *RefreshCompleteMessage) {
	addr := m.Hook.Resource.Addr
	if op, exists := rt.resources[addr]; exists {
		op.State = ResourceStateComplete
		op.EndTime = time.Now()
	}
}

// GetResources returns a snapshot of all resources in order.
func (rt *ResourceTracker) GetResources() []*ResourceOperation {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	result := make([]*ResourceOperation, 0, len(rt.order))
	for _, addr := range rt.order {
		if op, ok := rt.resources[addr]; ok {
			// Return copy to avoid race conditions.
			opCopy := *op
			result = append(result, &opCopy)
		}
	}
	return result
}

// GetPhase returns the current operation phase.
func (rt *ResourceTracker) GetPhase() Phase {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.phase
}

// GetChangeSummary returns the change summary if available.
func (rt *ResourceTracker) GetChangeSummary() *ChangeSummaryMessage {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.changeSummary
}

// GetDiagnostics returns all diagnostic messages.
func (rt *ResourceTracker) GetDiagnostics() []*DiagnosticMessage {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	result := make([]*DiagnosticMessage, len(rt.diagnostics))
	copy(result, rt.diagnostics)
	return result
}

// GetActiveCount returns the number of resources currently in progress.
func (rt *ResourceTracker) GetActiveCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	count := 0
	for _, op := range rt.resources {
		if op.State == ResourceStateInProgress || op.State == ResourceStateRefreshing {
			count++
		}
	}
	return count
}

// GetCompletedCount returns the number of completed resources.
func (rt *ResourceTracker) GetCompletedCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	count := 0
	for _, op := range rt.resources {
		if op.State == ResourceStateComplete || op.State == ResourceStateError {
			count++
		}
	}
	return count
}

// GetTotalCount returns the total number of resources.
func (rt *ResourceTracker) GetTotalCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.resources)
}

// GetErrorCount returns the number of failed resources.
func (rt *ResourceTracker) GetErrorCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	count := 0
	for _, op := range rt.resources {
		if op.State == ResourceStateError {
			count++
		}
	}
	return count
}

// GetCurrentActivity returns the first in-progress or refreshing resource, if any.
// Returns nil if no resource is currently active.
func (rt *ResourceTracker) GetCurrentActivity() *ResourceOperation {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	for _, addr := range rt.order {
		if op, ok := rt.resources[addr]; ok {
			if op.State == ResourceStateInProgress || op.State == ResourceStateRefreshing {
				opCopy := *op
				return &opCopy
			}
		}
	}
	return nil
}

// HasErrors returns true if any resources failed.
func (rt *ResourceTracker) HasErrors() bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	for _, op := range rt.resources {
		if op.State == ResourceStateError {
			return true
		}
	}
	// Also check for error diagnostics.
	for _, d := range rt.diagnostics {
		if d.Diagnostic.Severity == "error" {
			return true
		}
	}
	return false
}

// GetOutputs returns the captured output values.
func (rt *ResourceTracker) GetOutputs() *OutputsMessage {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.outputs
}
