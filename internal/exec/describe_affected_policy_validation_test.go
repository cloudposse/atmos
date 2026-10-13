package exec

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
)

func TestAffectedPolicyInventoryRejectsMalformedStructure(t *testing.T) {
	for name, value := range map[string]any{
		"stack":      "not an object",
		"components": map[string]any{"components": []any{}},
		"instances":  map[string]any{"components": map[string]any{"helm": []any{}}},
	} {
		t.Run(name, func(t *testing.T) {
			count, err := affectedPolicyInstanceCount(map[string]any{"prod": value})
			require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
			require.Zero(t, count)
		})
	}
	count, err := affectedPolicyInstanceCount(map[string]any{"empty": map[string]any{}})
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestAffectedPolicySnapshotValidation(t *testing.T) {
	for name, value := range map[string]any{
		"unencodable": map[string]any{"value": make(chan int)},
		"malformed":   map[string]any{"components": "not an object"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := affectedPolicySnapshot(map[string]any{"prod": value})
			require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
		})
	}
	snapshot, err := affectedPolicySnapshot(map[string]any{"empty": map[string]any{}})
	require.NoError(t, err)
	require.Equal(t, map[string]map[string]json.RawMessage{}, snapshot.Stacks["empty"])
}

func TestAffectedPolicyComponentSourceBoundaries(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "components", "helm")
	for _, tc := range []struct {
		name, directory, data, file string
		changed                     bool
		wantErr                     bool
	}{
		{name: "no source capability", data: `{}`},
		{name: "invalid component", directory: directory, data: `{"component":42}`, wantErr: true},
		{name: "outside repository", directory: t.TempDir(), data: `{}`, wantErr: true},
		{name: "path traversal", directory: directory, data: `{"component":"../../../escape"}`, wantErr: true},
		{name: "instance fallback", directory: directory, data: `{}`, file: "components/helm/app/Chart.yaml", changed: true},
		{name: "base component", directory: directory, data: `{"component":"chart"}`, file: "components/helm/chart/Chart.yaml", changed: true},
		{name: "prefix collision", directory: directory, data: `{}`, file: "components/helm/application/Chart.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed, err := affectedPolicyComponentFiles(root, tc.directory, "app", json.RawMessage(tc.data), []policy.FileChange{{Path: tc.file}})
			if tc.wantErr {
				require.Error(t, err)
				require.False(t, changed)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.changed, changed)
		})
	}
}

func TestAffectedPolicySourceChangesRejectInvalidInstances(t *testing.T) {
	root := t.TempDir()
	snapshot := policy.InventorySnapshot{Stacks: map[string]map[string]map[string]json.RawMessage{
		"prod": {"helm": {"app": json.RawMessage(`{"component":42}`)}},
	}}
	identities, err := affectedPolicySourceChanges(affectedPolicyTestConfig(root), root, snapshot, snapshot, nil)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
	require.Nil(t, identities)
}

func TestAffectedPolicyInventoryRejectsExternalDiscovery(t *testing.T) {
	root := t.TempDir()
	config := affectedPolicyTestConfig(root)
	config.StacksBaseAbsolutePath = t.TempDir()
	snapshot, err := collectAffectedPolicyInventory(config, root, t.TempDir())
	require.Error(t, err)
	require.Nil(t, snapshot.Stacks)
}

func TestAffectedPolicyInventoryRejectsIdentityCollisions(t *testing.T) {
	root, _ := affectedPolicyRepo(t, map[string]string{
		"stacks/one.yaml": affectedTestManifest,
		"stacks/two.yaml": affectedTestManifest,
	})
	snapshot, err := collectAffectedPolicyInventory(affectedPolicyTestConfig(root), root, root)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.ErrorContains(t, err, "identity collisions or omitted instances")
	require.Nil(t, snapshot.Stacks)
}
