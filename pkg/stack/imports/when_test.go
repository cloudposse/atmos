package imports

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: a rename of the field the tests set fails the build.
var _ = schema.StackImport{When: schema.Condition{}}

func TestDecodeObject(t *testing.T) {
	t.Run("decodes path and every scalar option", func(t *testing.T) {
		imp, err := DecodeObject(map[string]any{
			"path":                           "mixins/ci-source-tags",
			"skip_if_missing":                true,
			"skip_templates_processing":      true,
			"ignore_missing_template_values": true,
			"nested_imports":                 "remote",
			"ttl":                            "1h",
		})
		require.NoError(t, err)
		assert.Equal(t, "mixins/ci-source-tags", imp.Path)
		assert.True(t, imp.SkipIfMissing)
		assert.True(t, imp.SkipTemplatesProcessing)
		assert.True(t, imp.IgnoreMissingTemplateValues)
		assert.Equal(t, "remote", imp.NestedImports)
		assert.Equal(t, "1h", imp.TTL)
		assert.True(t, imp.When.IsZero(), "an import without `when` has no condition")
	})

	for _, tc := range []struct {
		name string
		when any
	}{
		{"predicate", "ci"},
		{"cel expression", `!cel env.DEPLOY_ENV == "prod"`},
		{"list of conditions", []any{"ci", "always"}},
		{"object form", map[string]any{"any": []any{"ci", "never"}}},
	} {
		t.Run("decodes a "+tc.name, func(t *testing.T) {
			imp, err := DecodeObject(map[string]any{"path": "x", "when": tc.when})
			require.NoError(t, err)
			assert.False(t, imp.When.IsZero(), "the condition must be set")
		})
	}

	t.Run("rejects an unknown predicate", func(t *testing.T) {
		_, err := DecodeObject(map[string]any{"path": "x", "when": "sometimes"})
		require.Error(t, err)
	})

	t.Run("rejects a value that is not an object", func(t *testing.T) {
		_, err := DecodeObject("just-a-string")
		require.Error(t, err)
	})
}

func TestIsActive(t *testing.T) {
	cond := func(value any) *schema.StackImport {
		return &schema.StackImport{Path: "p", When: schema.MustCondition(value)}
	}

	for _, tc := range []struct {
		name string
		imp  *schema.StackImport
		ctx  *condition.Context
		want bool
	}{
		{"no condition is always active", &schema.StackImport{Path: "p"}, &condition.Context{}, true},
		{"always", cond("always"), &condition.Context{}, true},
		{"never", cond("never"), &condition.Context{}, false},
		{"ci in CI", cond("ci"), &condition.Context{CI: true}, true},
		{"ci off CI", cond("ci"), &condition.Context{CI: false}, false},
		{"local off CI", cond("local"), &condition.Context{CI: false}, true},
		{"local in CI", cond("local"), &condition.Context{CI: true}, false},
		{"cel env match", cond(`!cel env.DEPLOY_ENV == "prod"`), &condition.Context{Env: map[string]string{"DEPLOY_ENV": "prod"}}, true},
		{"cel env mismatch", cond(`!cel env.DEPLOY_ENV == "prod"`), &condition.Context{Env: map[string]string{"DEPLOY_ENV": "dev"}}, false},
		{"all of ci and cel", cond([]any{"ci", `!cel env.DEPLOY_ENV == "prod"`}), &condition.Context{CI: true, Env: map[string]string{"DEPLOY_ENV": "dev"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsActive(tc.imp, tc.ctx))
		})
	}

	// A CEL expression over an unset variable raises "no such key". That is the common case, so it
	// must skip the import rather than fail, matching `import:` entries in atmos.yaml.
	t.Run("an expression over an unset variable does not match", func(t *testing.T) {
		assert.False(t, IsActive(cond(`!cel env.MISSING == "x"`), &condition.Context{Env: map[string]string{}}))
	})
}

func TestWhenContext(t *testing.T) {
	t.Setenv("ATMOS_TEST_STACK_IMPORT_WHEN", "present")

	t.Run("reports CI when a CI provider is detected", func(t *testing.T) {
		t.Cleanup(ci.SwapRegistryForTest())
		ci.Register(github.NewProvider())
		t.Setenv("GITHUB_ACTIONS", "true")

		ctx := WhenContext()
		assert.True(t, ctx.CI)
		assert.Equal(t, "present", ctx.Env["ATMOS_TEST_STACK_IMPORT_WHEN"])
	})

	t.Run("reports local when no CI provider is detected", func(t *testing.T) {
		t.Cleanup(ci.SwapRegistryForTest())

		assert.False(t, WhenContext().CI)
	})
}
