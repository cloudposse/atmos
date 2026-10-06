package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// parseDocNode unmarshals input into a *yaml.Node whose Kind is
// yaml.DocumentNode, exactly what yaml.Unmarshal(data, &doc) produces for
// pkg/manifest's resolveIncludeTags -- the one real caller that passes
// WalkYAMLTags a raw, un-unwrapped document node (processCustomTags, this
// package's other caller, always unwraps the document node itself first).
func parseDocNode(t *testing.T, input string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(input), &doc))
	require.Equal(t, yaml.DocumentNode, doc.Kind, "sanity: yaml.Unmarshal into a Node always yields a DocumentNode")
	return &doc
}

// TestWalkYAMLTags_RawDocumentNode proves WalkYAMLTags unwraps a raw
// DocumentNode (the shape yaml.Unmarshal(data, &doc) produces) before
// dispatching tags on its real root -- the exact input shape
// pkg/manifest.resolveIncludeTags passes it, as opposed to the already-
// unwrapped node processCustomTags passes via getStackManifestTagPolicy.
func TestWalkYAMLTags_RawDocumentNode(t *testing.T) {
	t.Setenv("ATMOS_SCAFFOLD_WALKER_TEST_VAR", "hello-from-env")

	doc := parseDocNode(t, "value: !env ATMOS_SCAFFOLD_WALKER_TEST_VAR\n")

	err := WalkYAMLTags(&schema.AtmosConfiguration{}, doc, "test.yaml", ScaffoldTagPolicy(nil))
	require.NoError(t, err)

	root := doc.Content[0]
	require.Equal(t, yaml.MappingNode, root.Kind)
	valueNode := root.Content[1]
	assert.Equal(t, "hello-from-env", valueNode.Value)
	assert.NotEqual(t, AtmosYamlFuncEnv, valueNode.Tag, "a resolved !env node must have its custom tag replaced")
}

// TestWalkYAMLTags_ScaffoldPolicy_SimpleTagHandlerError proves a resolver
// error inside a ScaffoldTagPolicy simple-tag handler (here, !env given an
// arity ParseEnv rejects) propagates out of WalkYAMLTags rather than being
// swallowed or leaving the node half-resolved.
func TestWalkYAMLTags_ScaffoldPolicy_SimpleTagHandlerError(t *testing.T) {
	doc := parseDocNode(t, "value: !env ONE TWO THREE\n")

	err := WalkYAMLTags(&schema.AtmosConfiguration{}, doc, "test.yaml", ScaffoldTagPolicy(nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env function accepts 1 or 2 arguments")
}

// TestWalkYAMLTags_ScaffoldPolicy_IncludeCallback proves ScaffoldTagPolicy's
// onInclude callback fires with the tag's raw, pre-resolution path argument
// for both !include and !include.raw, and that the tag itself still resolves
// normally afterward.
func TestWalkYAMLTags_ScaffoldPolicy_IncludeCallback(t *testing.T) {
	dir := t.TempDir()
	writeIncludeFixtureUtils(t, dir, "greeting.yaml", "hello: world\n")

	doc := parseDocNode(t, "values: !include ./greeting.yaml\n")

	var seen []string
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	err := WalkYAMLTags(atmosConfig, doc, filepath.Join(dir, "manifest.yaml"), ScaffoldTagPolicy(func(path string) {
		seen = append(seen, path)
	}))
	require.NoError(t, err)
	assert.Equal(t, []string{"./greeting.yaml"}, seen)
}

// TestWalkYAMLTags_UnhandledTagRejectedUnderNonDeferPolicy proves a tag that
// is globally valid (fntag.IsValidYAML accepts it) but absent from
// ScaffoldTagPolicy's own handler map is a hard error naming the tag and
// listing the supported set -- never silently deferred, since Defer is false
// for this policy (unlike the stack-manifest policy).
func TestWalkYAMLTags_UnhandledTagRejectedUnderNonDeferPolicy(t *testing.T) {
	doc := parseDocNode(t, "value: !unset\n")

	err := WalkYAMLTags(&schema.AtmosConfiguration{}, doc, "test.yaml", ScaffoldTagPolicy(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
	// The hint lists the handler map's own keys (sorted) -- confirm it
	// actually names a real handler rather than coming back empty, proving
	// sortedHandlerTags ran against ScaffoldTagPolicy's real handler set.
	assert.True(t, errUtils.HasHint(err, "!literal"), "expected the rejection hint to list supported tags, got: %v", err)
}

// TestWalkYAMLTags_ScaffoldPolicy_IncludeRawCallback proves the
// !include.raw branch of ScaffoldTagPolicy's onInclude wiring (a distinct
// closure from !include's own) also fires with the raw path argument.
func TestWalkYAMLTags_ScaffoldPolicy_IncludeRawCallback(t *testing.T) {
	dir := t.TempDir()
	writeIncludeFixtureUtils(t, dir, "raw.txt", "hello")

	doc := parseDocNode(t, "value: !include.raw ./raw.txt\n")

	var seen []string
	atmosConfig := &schema.AtmosConfiguration{BasePath: dir, BasePathAbsolute: dir}
	err := WalkYAMLTags(atmosConfig, doc, filepath.Join(dir, "manifest.yaml"), ScaffoldTagPolicy(func(path string) {
		seen = append(seen, path)
	}))
	require.NoError(t, err)
	assert.Equal(t, []string{"./raw.txt"}, seen)

	root := doc.Content[0]
	assert.Equal(t, "hello", root.Content[1].Value)
}

// TestWalkYAMLTags_ScaffoldPolicy_SimpleHandlersResolve proves every
// context-free simple-tag handler ScaffoldTagPolicy wires up (!random, !cwd,
// and the full !git.*/!repo-root family) actually invokes its own real
// resolver function -- not just that the map entry exists -- by resolving
// each tag through WalkYAMLTags and checking a real postcondition on the
// result. Each git-family case supplies a trailing default value per that
// tag's own documented fallback-on-failure contract (see
// pkg/git/yaml_tags.go), so the assertion holds whether or not the test
// happens to run inside a real git worktree.
func TestWalkYAMLTags_ScaffoldPolicy_SimpleHandlersResolve(t *testing.T) {
	tests := []struct {
		name string
		tag  string
	}{
		{name: "random", tag: "!random 1 2"},
		{name: "cwd", tag: "!cwd"},
		{name: "repo-root", tag: "!repo-root fallback-root"},
		{name: "git.root", tag: "!git.root fallback-root"},
		{name: "git.sha", tag: "!git.sha fallback-sha"},
		{name: "git.ref", tag: "!git.ref fallback-sha"},
		{name: "git.branch", tag: "!git.branch fallback-branch"},
		{name: "git.repository", tag: "!git.repository fallback-repo"},
		{name: "git.owner", tag: "!git.owner fallback-owner"},
		{name: "git.name", tag: "!git.name fallback-name"},
		{name: "git.host", tag: "!git.host fallback-host"},
		{name: "git.url", tag: "!git.url fallback-url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := parseDocNode(t, "value: "+tt.tag+"\n")

			err := WalkYAMLTags(&schema.AtmosConfiguration{}, doc, "test.yaml", ScaffoldTagPolicy(nil))
			require.NoError(t, err)

			root := doc.Content[0]
			assert.NotEmpty(t, root.Content[1].Value, "expected %s to resolve to a non-empty value", tt.tag)
		})
	}
}

// TestHandleAppendTag_NestedInvalidTagPropagatesError proves an !append
// sequence's own ctx.Walk (which resolves nested tags inside the sequence
// before wrapping it) propagates an error from a genuinely invalid nested
// tag, rather than silently wrapping a half-processed sequence.
func TestHandleAppendTag_NestedInvalidTagPropagatesError(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	input := "items: !append\n  - !totally-bogus-tag nope\n  - plain\n"

	_, err := UnmarshalYAMLFromFile[map[string]any](cfg, input, "stack.yaml")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedYamlTag)
	assert.Contains(t, err.Error(), "!totally-bogus-tag")
}

// writeIncludeFixtureUtils writes a local file a test's !include tag points
// at (pkg/utils-local counterpart of pkg/manifest's writeIncludeFixture
// helper -- kept separate since the two packages' test files don't share
// non-exported helpers).
func writeIncludeFixtureUtils(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}
