package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

func TestEmitPlanWarningAnnotations(t *testing.T) {
	t.Run("no warnings emits nothing", func(t *testing.T) {
		mp := newMockProvider()
		p := &Plugin{}
		ctx := &plugin.HookContext{Provider: mp, Command: "plan"}
		result := &plugin.OutputResult{Data: &plugin.TerraformOutputData{}}

		p.emitPlanWarningAnnotations(ctx, result)

		assert.Empty(t, mp.annotateCalls)
	})

	t.Run("warning with a source locator carries file and line", func(t *testing.T) {
		mp := newMockProvider()
		p := &Plugin{}
		ctx := &plugin.HookContext{Provider: mp, Command: "plan"}
		block := "Warning: Argument is deprecated\n\n  with aws_s3_bucket.this,\n  on main.tf line 12, in resource \"aws_s3_bucket\" \"this\":\n  12:   acl = \"private\"\n\nUse the replacement attribute instead."
		result := &plugin.OutputResult{
			Data: &plugin.TerraformOutputData{Warnings: []string{block}},
		}

		p.emitPlanWarningAnnotations(ctx, result)

		require.Len(t, mp.annotateCalls, 1)
		require.Len(t, mp.annotateCalls[0], 1)
		ann := mp.annotateCalls[0][0]
		assert.Equal(t, provider.AnnotationWarning, ann.Level)
		assert.Equal(t, "main.tf", ann.Path)
		assert.Equal(t, 12, ann.StartLine)
		assert.Equal(t, "terraform plan: warning", ann.Title)
		assert.Equal(t, block, ann.Message)
	})

	t.Run("warning without a source locator falls back to a file-level annotation", func(t *testing.T) {
		mp := newMockProvider()
		p := &Plugin{}
		ctx := &plugin.HookContext{Provider: mp, Command: "apply"}
		result := &plugin.OutputResult{
			Data: &plugin.TerraformOutputData{
				Warnings: []string{"Warning: Provider development overrides are in effect"},
			},
		}

		p.emitPlanWarningAnnotations(ctx, result)

		require.Len(t, mp.annotateCalls, 1)
		require.Len(t, mp.annotateCalls[0], 1)
		ann := mp.annotateCalls[0][0]
		assert.Empty(t, ann.Path)
		assert.Zero(t, ann.StartLine)
		assert.Equal(t, "terraform apply: warning", ann.Title)
	})

	t.Run("multiple warning blocks each become their own annotation", func(t *testing.T) {
		mp := newMockProvider()
		p := &Plugin{}
		ctx := &plugin.HookContext{Provider: mp, Command: "plan"}
		result := &plugin.OutputResult{
			Data: &plugin.TerraformOutputData{
				Warnings: []string{
					"Warning: first\n\n  on main.tf line 1, in resource \"a\" \"b\":\n  1: resource \"a\" \"b\" {",
					"Warning: second",
				},
			},
		}

		p.emitPlanWarningAnnotations(ctx, result)

		require.Len(t, mp.annotateCalls, 1)
		require.Len(t, mp.annotateCalls[0], 2)
		assert.Equal(t, "main.tf", mp.annotateCalls[0][0].Path)
		assert.Equal(t, 1, mp.annotateCalls[0][0].StartLine)
		assert.Empty(t, mp.annotateCalls[0][1].Path)
	})

	t.Run("non-terraform-plan data is a no-op", func(t *testing.T) {
		mp := newMockProvider()
		p := &Plugin{}
		ctx := &plugin.HookContext{Provider: mp, Command: "test"}
		result := &plugin.OutputResult{Data: &plugin.TerraformTestOutputData{}}

		p.emitPlanWarningAnnotations(ctx, result)

		assert.Empty(t, mp.annotateCalls)
	})
}
