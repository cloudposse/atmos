package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConfigCRUDPreservesTaggedRefAndComments(t *testing.T) {
	dir := realTemp(t)
	file := filepath.Join(dir, "atmos.yaml")
	write(t, file, "# preserved\nai:\n  enabled: true\n")
	config := &schema.AtmosConfiguration{BasePath: dir}
	_, err := Edit(config, EditOptions{Operation: "add", Label: "Example", Value: "example/repo", File: file})
	require.NoError(t, err)
	_, err = Edit(config, EditOptions{Operation: "set", Label: "Example", Field: "ref", Value: "!version tool", File: file})
	require.NoError(t, err)
	_, err = Edit(config, EditOptions{Operation: "set", Label: "Example", Field: "clients", Value: "[claude-code]", File: file})
	require.NoError(t, err)
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(raw), "!version tool")
	require.Contains(t, string(raw), "# preserved")
	require.Contains(t, string(raw), "Example:")
	_, err = Edit(config, EditOptions{Operation: "remove", Label: "Example", File: file, DryRun: true})
	require.NoError(t, err)
	unchanged, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, raw, unchanged)
	_, err = Edit(config, EditOptions{Operation: "remove", Label: "Example", File: file})
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, ".atmos"))
	require.True(t, os.IsNotExist(err))
}

func TestLayeredRemovalAndUnsafeAnchors(t *testing.T) {
	dir := realTemp(t)
	t.Chdir(dir)
	root := filepath.Join(dir, "atmos.yaml")
	fragment := filepath.Join(dir, ".atmos.d", "skills.yaml")
	declaration := "ai:\n  skills:\n    MixedCase:\n      source: org/repo\n      ref: !version dep\n"
	write(t, root, declaration)
	write(t, fragment, declaration)
	config := &schema.AtmosConfiguration{BasePath: dir}
	_, err := Edit(config, EditOptions{Operation: "remove", Label: "MixedCase"})
	require.ErrorContains(t, err, root)
	require.ErrorContains(t, err, fragment)
	message, err := Edit(config, EditOptions{Operation: "remove", Label: "MixedCase", File: root})
	require.NoError(t, err)
	require.Contains(t, message, "other layers still declare MixedCase")
	untouched, err := os.ReadFile(fragment)
	require.NoError(t, err)
	require.Equal(t, declaration, string(untouched))
	_, err = Edit(config, EditOptions{Operation: "add", Label: "MixedCase", Value: "org/other", File: root})
	require.ErrorContains(t, err, "already exists")

	write(t, root, "ai:\n  skills:\n    Shared: &shared\n      source: org/repo\n    Alias: *shared\n")
	_, err = Edit(config, EditOptions{Operation: "set", Label: "Shared", Field: "source", Value: "org/other", File: root})
	require.Error(t, err, "editing an anchor must not silently retarget aliases")
}

func TestTypedSourceEditsRejectInvalidValues(t *testing.T) {
	dir := realTemp(t)
	t.Chdir(dir)
	file := filepath.Join(dir, "atmos.yaml")
	write(t, file, "ai:\n  skills:\n    Test:\n      source: org/repo\n")
	for _, edit := range []EditOptions{
		{Operation: "set", Label: "Test", Field: "clients", Value: "claude-code"},
		{Operation: "set", Label: "Test", Field: "kind", Value: "npm"},
		{Operation: "set", Label: "Test", Field: "scope", Value: "machine"},
		{Operation: "set", Label: "Test", Field: "system_prompt", Value: "inline"},
		{Operation: "set", Label: "Absent", Field: "source", Value: "org/repo"},
	} {
		edit.File = file
		_, err := Edit(&schema.AtmosConfiguration{BasePath: dir}, edit)
		require.Error(t, err)
	}
}

func TestSetLiteralRefClearsDeferredTag(t *testing.T) {
	dir := realTemp(t)
	t.Chdir(dir)
	file := filepath.Join(dir, "atmos.yaml")
	write(t, file, "ai:\n  skills:\n    Test:\n      source: org/repo\n      ref: !version dep\n")
	_, err := Edit(&schema.AtmosConfiguration{BasePath: dir}, EditOptions{Operation: "set", Label: "Test", Field: "ref", Value: "main", File: file})
	require.NoError(t, err)
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "!version")
	require.Contains(t, string(raw), "ref: main")
}
