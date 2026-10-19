package policy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func inventoryFixture(data string) InventorySnapshot {
	return InventorySnapshot{Stacks: map[string]map[string]map[string]json.RawMessage{
		"prod": {"helm": {"app": json.RawMessage(data)}},
	}}
}

func TestBuildAffectedInput(t *testing.T) {
	before := inventoryFixture(`{"component":"chart","metadata":{"labels":{"owner":"platform","a/b~":null}}}`)
	after := inventoryFixture(`{"component":"chart","metadata":{"labels":{"new":"label"}}}`)
	input, err := BuildAffectedInput(before, after, nil, nil)
	require.NoError(t, err)
	require.True(t, input.Complete)
	require.NotNil(t, input.Files)
	require.Len(t, input.Instances, 1)
	require.Len(t, input.Components, 1)
	require.Equal(t, "chart", input.Components[0].Identity.Name)
	require.Len(t, input.Stacks, 1)
	require.Len(t, input.Changes, 3)
	require.Equal(t, "/metadata/labels/a~1b~0", input.Changes[0].Path)
	require.True(t, input.Changes[0].BeforePresent)
	require.False(t, input.Changes[0].AfterPresent)
	require.JSONEq(t, "null", string(input.Changes[0].Before))
	require.Equal(t, Added, input.Changes[1].Action)
	require.Equal(t, Removed, input.Changes[2].Action)
	before.Stacks["prod"]["helm"]["app"][2] = 'X'
	require.JSONEq(t, `{"component":"chart","metadata":{"labels":{"owner":"platform","a/b~":null}}}`, string(input.Instances[0].Before))
}

func TestBuildAffectedInputGroupsAndSourceChanges(t *testing.T) {
	before := inventoryFixture(`{"component":"chart"}`)
	before.Stacks["dev"] = map[string]map[string]json.RawMessage{"helm": {"app": json.RawMessage(`{"component":"chart"}`)}}
	after := InventorySnapshot{Stacks: map[string]map[string]map[string]json.RawMessage{"dev": before.Stacks["dev"]}}
	input, err := BuildAffectedInput(before, after, nil, nil)
	require.NoError(t, err)
	require.Len(t, input.Instances, 1)
	require.Equal(t, Removed, input.Instances[0].Action)
	require.Equal(t, Removed, input.Stacks[0].Action)
	require.Equal(t, Modified, input.Components[0].Action, "shared chart still exists in dev")
	require.Contains(t, string(input.Components[0].After), "dev")

	input, err = BuildAffectedInput(after, after, nil, []ObjectIdentity{{Name: "app", Stack: "dev", ComponentType: "helm"}})
	require.NoError(t, err)
	require.Len(t, input.Instances, 1)
	require.Equal(t, []string{"component_files"}, input.Instances[0].Reasons)
	require.Len(t, input.Stacks, 1)
	require.Len(t, input.Components, 1)
	require.Empty(t, input.Changes)

	input, err = BuildAffectedInput(after, after, nil, nil)
	require.NoError(t, err)
	require.Empty(t, input.Instances)
	require.Empty(t, input.Stacks)
	require.Empty(t, input.Components)
}

func TestBuildAffectedInputValidation(t *testing.T) {
	empty := InventorySnapshot{Stacks: map[string]map[string]map[string]json.RawMessage{}}
	incomplete, err := BuildAffectedInput(InventorySnapshot{}, empty, nil, nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.False(t, incomplete.Complete)
	_, err = BuildAffectedInput(inventoryFixture(`null`), empty, nil, nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
	input, err := BuildAffectedInput(empty, inventoryFixture(`{}`), nil, nil)
	require.NoError(t, err)
	require.Equal(t, Added, input.Instances[0].Action)
	require.True(t, input.Instances[0].Ambiguous)
	require.Equal(t, Added, input.Stacks[0].Action)
	require.Equal(t, Added, input.Components[0].Action)
	input, err = BuildAffectedInput(inventoryFixture(`{}`), empty, nil, nil)
	require.NoError(t, err)
	require.Equal(t, Removed, input.Components[0].Action)
}

func TestBuildAffectedInputRejectsIncompleteIdentities(t *testing.T) {
	for _, identity := range []ObjectIdentity{
		{Name: "app", ComponentType: "helm"},
		{Name: "app", Stack: "prod"},
		{Stack: "prod", ComponentType: "helm"},
	} {
		t.Run(identity.Stack+"/"+identity.ComponentType+"/"+identity.Name, func(t *testing.T) {
			snapshot := InventorySnapshot{Stacks: map[string]map[string]map[string]json.RawMessage{
				identity.Stack: {identity.ComponentType: {identity.Name: json.RawMessage(`{}`)}},
			}}
			input, err := BuildAffectedInput(inventoryFixture(`{}`), snapshot, nil, nil)
			require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
			require.False(t, input.Complete)
		})
	}
	input, err := BuildAffectedInput(inventoryFixture(`{}`), InventorySnapshot{}, nil, nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.False(t, input.Complete)
}

func TestChangedFieldsRejectsInvalidSnapshots(t *testing.T) {
	for name, values := range map[string][2]string{
		"before": {`null`, `{}`},
		"after":  {`{}`, `[]`},
	} {
		t.Run(name, func(t *testing.T) {
			fields, err := changedFields(ObjectIdentity{Name: "app"}, json.RawMessage(values[0]), json.RawMessage(values[1]))
			require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
			require.Nil(t, fields)
		})
	}
}
