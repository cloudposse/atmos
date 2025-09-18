//go:build exec_test

package exec

import (
	"os"
	"path/filepath"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// toggle allowing tests to inject a fake FindStacksMap
var useFakeFindStacksMap bool
var fakeStacksMap map[string]any

// fakeFindStacksMap mimics FindStacksMap signature for tests.
func fakeFindStacksMap(_ *schema.AtmosConfiguration, _ bool) (map[string]any, []string, error) {
	if !useFakeFindStacksMap {
		return FindStacksMap(&schema.AtmosConfiguration{}, false)
	}
	return fakeStacksMap, nil, nil
}

// Test helper to build a minimal AtmosConfiguration with temporary base/component paths.
func newMinimalAtmosConfigForTest(t *testing.T, baseDir string) *schema.AtmosConfiguration {
	t.Helper()
	return &schema.AtmosConfiguration{
		BasePath: baseDir,
		Components: schema.Components{
			Terraform: schema.ComponentsTerraform{
				BasePath: "components/terraform",
			},
		},
		Stacks: schema.Stacks{
			NameTemplate: "",
		},
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Enabled: true,
			},
		},
	}
}

// prepareTerraformComponentDir creates a temp component folder to which backend files will be written.
func prepareTerraformComponentDir(t *testing.T, baseDir, componentRel string) string {
	t.Helper()
	dir := filepath.Join(baseDir, "components", "terraform", componentRel)
	if err := ensureDir(dir); err != nil {
		t.Fatalf("failed to create component dir: %v", err)
	}
	return dir
}
func ensureDir(p string) error {
	return os.MkdirAll(p, 0o755)
}