package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// InventorySnapshot is a complete, unfiltered resolved stack inventory. Callers
// must establish collection completeness before constructing a policy input.
type InventorySnapshot struct {
	Stacks map[string]map[string]map[string]json.RawMessage
}

type resolvedInventory struct {
	snapshot  InventorySnapshot
	instances map[ObjectIdentity]json.RawMessage
}

// BuildAffectedInput compares independently collected snapshots. SourceAffected
// identifies instances whose component files changed without a stack-value change.
func BuildAffectedInput(before, after InventorySnapshot, files []FileChange, sourceAffected []ObjectIdentity) (AffectedInput, error) {
	defer perf.Track(nil, "policy.BuildAffectedInput")()

	input := AffectedInput{
		Reasons: []string{}, Files: append([]FileChange{}, files...),
		Instances: []ObjectChange{}, Components: []ObjectChange{}, Stacks: []ObjectChange{}, Changes: []FieldChange{},
	}
	old, err := resolveInventory(before)
	if err != nil {
		return input, err
	}
	current, err := resolveInventory(after)
	if err != nil {
		return input, err
	}
	forced := map[ObjectIdentity]bool{}
	for _, identity := range sourceAffected {
		forced[identity] = true
	}
	for _, identity := range inventoryIdentities(old.instances, current.instances) {
		change, changed := inventoryChange(identity, old.instances[identity], current.instances[identity], forced[identity])
		if !changed {
			continue
		}
		input.Instances = append(input.Instances, change)
		fields, err := changedFields(identity, change.Before, change.After)
		if err != nil {
			return input, err
		}
		input.Changes = append(input.Changes, fields...)
	}
	input.Stacks, err = changedInventoryGroups(old, current, forced, true)
	if err != nil {
		return input, err
	}
	input.Components, err = changedInventoryGroups(old, current, forced, false)
	if err != nil {
		return input, err
	}
	input.Complete = true
	return input, nil
}

func resolveInventory(snapshot InventorySnapshot) (resolvedInventory, error) {
	if snapshot.Stacks == nil {
		return resolvedInventory{}, fmt.Errorf("%w: missing stack inventory", errUtils.ErrPolicyInputUnavailable)
	}
	instances, err := flattenInventory(snapshot)
	return resolvedInventory{snapshot: snapshot, instances: instances}, err
}

func flattenInventory(snapshot InventorySnapshot) (map[ObjectIdentity]json.RawMessage, error) {
	result := map[ObjectIdentity]json.RawMessage{}
	for stack, kinds := range snapshot.Stacks {
		for kind, instances := range kinds {
			for name, instance := range instances {
				if stack == "" || kind == "" || name == "" {
					return nil, fmt.Errorf("%w: incomplete instance identity", errUtils.ErrPolicyInputInvalid)
				}
				data, err := decodeObject(instance)
				if err != nil {
					return nil, err
				}
				normalized, err := json.Marshal(data)
				if err != nil {
					return nil, fmt.Errorf("%w: inventory JSON: %w", errUtils.ErrPolicyInputInvalid, err)
				}
				result[ObjectIdentity{Name: name, Stack: stack, ComponentType: kind}] = normalized
			}
		}
	}
	return result, nil
}

func inventoryIdentities(before, after map[ObjectIdentity]json.RawMessage) []ObjectIdentity {
	keys := map[ObjectIdentity]bool{}
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	result := slices.Collect(maps.Keys(keys))
	slices.SortFunc(result, func(a, b ObjectIdentity) int {
		return strings.Compare(a.Stack+"\x00"+a.ComponentType+"\x00"+a.Name, b.Stack+"\x00"+b.ComponentType+"\x00"+b.Name)
	})
	return result
}

func inventoryChange(identity ObjectIdentity, before, after json.RawMessage, forced bool) (ObjectChange, bool) {
	change := ObjectChange{Identity: identity, Before: before, After: after, Action: Modified, Reasons: []string{"configuration"}}
	switch {
	case len(before) == 0:
		change.Action = Added
		change.Ambiguous = true
	case len(after) == 0:
		change.Action = Removed
		change.Ambiguous = true
	case bytes.Equal(before, after):
		if !forced {
			return change, false
		}
		change.Reasons = []string{"component_files"}
	}
	return change, true
}

// Groups expose affected stacks and component names with their contributing instances.
// No cross-stack metadata is merged into a synthetic component configuration.
func changedInventoryGroups(before, after resolvedInventory, forced map[ObjectIdentity]bool, byStack bool) ([]ObjectChange, error) {
	oldGroups, err := inventoryGroups(before.snapshot, before.instances, byStack)
	if err != nil {
		return nil, err
	}
	newGroups, err := inventoryGroups(after.snapshot, after.instances, byStack)
	if err != nil {
		return nil, err
	}
	forcedGroups := map[ObjectIdentity]bool{}
	for identity := range forced {
		for _, data := range []json.RawMessage{before.instances[identity], after.instances[identity]} {
			if len(data) > 0 {
				forcedGroups[inventoryGroupIdentity(identity, data, byStack)] = true
			}
		}
	}
	result := []ObjectChange{}
	for _, identity := range inventoryIdentities(oldGroups, newGroups) {
		if change, changed := inventoryChange(identity, oldGroups[identity], newGroups[identity], forcedGroups[identity]); changed {
			result = append(result, change)
		}
	}
	return result, nil
}

func inventoryGroupIdentity(identity ObjectIdentity, data json.RawMessage, byStack bool) ObjectIdentity {
	if byStack {
		return ObjectIdentity{Name: identity.Stack}
	}
	var object map[string]any
	// Entries have already been decoded and validated by flattenInventory.
	_ = json.Unmarshal(data, &object)
	name, _ := object["component"].(string)
	if name == "" {
		name = identity.Name
	}
	return ObjectIdentity{Name: name, ComponentType: identity.ComponentType}
}

func inventoryGroups(snapshot InventorySnapshot, instances map[ObjectIdentity]json.RawMessage, byStack bool) (map[ObjectIdentity]json.RawMessage, error) {
	groups := map[ObjectIdentity]map[string]json.RawMessage{}
	if byStack {
		for stack := range snapshot.Stacks {
			groups[ObjectIdentity{Name: stack}] = map[string]json.RawMessage{}
		}
	}
	for identity, data := range instances {
		group := inventoryGroupIdentity(identity, data, byStack)
		if groups[group] == nil {
			groups[group] = map[string]json.RawMessage{}
		}
		// JSON-encoded tuples avoid collisions in names containing slashes.
		key, _ := json.Marshal(identity)
		groups[group][string(key)] = data
	}
	result := map[ObjectIdentity]json.RawMessage{}
	for identity, members := range groups {
		data, err := json.Marshal(map[string]any{"instances": members})
		if err != nil {
			return nil, fmt.Errorf("%w: inventory group: %w", errUtils.ErrPolicyInputInvalid, err)
		}
		result[identity] = data
	}
	return result, nil
}

func changedFields(identity ObjectIdentity, before, after json.RawMessage) ([]FieldChange, error) {
	var old, current any
	if len(before) > 0 {
		var err error
		old, err = decodeObject(before)
		if err != nil {
			return nil, err
		}
	}
	if len(after) > 0 {
		var err error
		current, err = decodeObject(after)
		if err != nil {
			return nil, err
		}
	}
	result := []FieldChange{}
	walkFieldChanges(&result, identity, "", fieldSnapshot{old, len(before) > 0}, fieldSnapshot{current, len(after) > 0})
	return result, nil
}

type fieldSnapshot struct {
	value   any
	present bool
}

func walkFieldChanges(result *[]FieldChange, identity ObjectIdentity, path string, before, after fieldSnapshot) {
	oldMap, oldObject := before.value.(map[string]any)
	newMap, newObject := after.value.(map[string]any)
	if oldObject && newObject {
		keys := map[string]bool{}
		for key := range oldMap {
			keys[key] = true
		}
		for key := range newMap {
			keys[key] = true
		}
		for _, key := range slices.Sorted(maps.Keys(keys)) {
			oldValue, oldExists := oldMap[key]
			newValue, newExists := newMap[key]
			pointer := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			walkFieldChanges(result, identity, path+"/"+pointer, fieldSnapshot{oldValue, oldExists}, fieldSnapshot{newValue, newExists})
		}
		return
	}
	oldJSON, _ := json.Marshal(before.value)
	newJSON, _ := json.Marshal(after.value)
	if before.present == after.present && bytes.Equal(oldJSON, newJSON) {
		return
	}
	change := FieldChange{Identity: identity, Path: path, Action: Modified, BeforePresent: before.present, AfterPresent: after.present}
	if before.present {
		change.Before = oldJSON
	} else {
		change.Action = Added
	}
	if after.present {
		change.After = newJSON
	} else {
		change.Action = Removed
	}
	*result = append(*result, change)
}
