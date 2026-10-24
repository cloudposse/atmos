package policy

import (
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ChangeAction distinguishes additions, modifications, and removals. Collectors
// represent ambiguous moves conservatively as removal/addition, with Ambiguous set.
type ChangeAction string

const (
	Added    ChangeAction = "add"
	Modified ChangeAction = "modify"
	Removed  ChangeAction = "remove"
)

// ObjectIdentity distinguishes shared components from configured instances.
// An instance uses Stack + ComponentType + Name; a shared component omits Stack.
type ObjectIdentity struct {
	Name          string `json:"name"`
	ComponentType string `json:"component_type,omitempty"`
	Stack         string `json:"stack,omitempty"`
}

// FileChange records both paths for detected renames without assuming object identity.
type FileChange struct {
	Path         string       `json:"path"`
	PreviousPath string       `json:"previous_path,omitempty"`
	Action       ChangeAction `json:"action"`
	Ambiguous    bool         `json:"ambiguous"`
}

// ObjectChange holds separate before/after objects; selectors must not merge them.
// Absent before/after fields differ from explicit JSON null values.
type ObjectChange struct {
	Identity  ObjectIdentity  `json:"identity"`
	Action    ChangeAction    `json:"action"`
	Before    json.RawMessage `json:"before,omitempty"`
	After     json.RawMessage `json:"after,omitempty"`
	Reasons   []string        `json:"reasons"`
	Ambiguous bool            `json:"ambiguous"`
}

// FieldChange uses a JSON Pointer path and explicit presence to distinguish a
// missing field from a field whose value is null. Collectors populate both flags.
type FieldChange struct {
	Identity      ObjectIdentity  `json:"identity"`
	Path          string          `json:"path"`
	Action        ChangeAction    `json:"action"`
	BeforePresent bool            `json:"before_present"`
	AfterPresent  bool            `json:"after_present"`
	Before        json.RawMessage `json:"before,omitempty"`
	After         json.RawMessage `json:"after,omitempty"`
}

// AffectedInput is a materialized analysis payload for Request.Input. The command
// collector compares unfiltered static inventories under active discovery settings;
// it does not reuse the filtered describe-affected presentation as its inventory.
// Empty, complete lists must be encoded as [] rather than unavailable/null lists.
// Evaluating this payload never computes inventories or schedules component work.
type AffectedInput struct {
	Complete   bool           `json:"complete"`
	Reasons    []string       `json:"reasons"`
	Files      []FileChange   `json:"files"`
	Components []ObjectChange `json:"components"`
	Instances  []ObjectChange `json:"instances"`
	Stacks     []ObjectChange `json:"stacks"`
	Changes    []FieldChange  `json:"changes"`
}

// validateAffectedInput requires an explicit complete inventory before any rule
// can turn a false condition into a non-blocking decision. Actual completeness
// is the trusted collector's responsibility; flags alone do not prove it.
func validateAffectedInput(data json.RawMessage) error {
	var affected AffectedInput
	if err := json.Unmarshal(data, &affected); err != nil {
		return fmt.Errorf("%w: affected: %w", errUtils.ErrPolicyInputInvalid, err)
	}
	if !affected.Complete || affected.Files == nil || affected.Components == nil ||
		affected.Instances == nil || affected.Stacks == nil || affected.Changes == nil {
		return fmt.Errorf("%w: affected requires complete files, components, instances, stacks, and changes", errUtils.ErrPolicyInputUnavailable)
	}
	return nil
}
