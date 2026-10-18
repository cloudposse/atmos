package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestRenderImportPath_ComponentGuard verifies import paths reject Component calls rather than
// deferring them to literal templates: import paths must resolve while manifests are loading.
func TestRenderImportPath_ComponentGuard(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		ignoreMissing bool
	}{
		{name: "direct call", path: `{{ atmos.Component "base" "dev" }}`},
		{name: "component field", path: `catalog/{{ (atmos.Component "base" "dev").vars.path }}`},
		{name: "ignore missing values retains guard", path: `{{ atmos.Component "base" "dev" }}`, ignoreMissing: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderImportPath(templatedImportContextConfig(), "deploy/dev.yaml", tt.path,
				map[string]any{}, schema.StackImport{IgnoreMissingTemplateValues: tt.ignoreMissing})

			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrComponentFuncDuringManifestLoad)
			assert.ErrorIs(t, err, errUtils.ErrImportPathTemplate)
			assert.Contains(t, err.Error(), "deploy/dev.yaml")
			assert.Empty(t, got)
		})
	}
}
