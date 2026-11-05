package doctor

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/validation"
)

// vendorManifest declares imports in a vendor manifest.
func vendorManifest(imports ...string) string {
	content := "apiVersion: atmos/v1\nkind: AtmosVendorConfig\nspec:\n  imports:\n"
	for _, item := range imports {
		content += "    - " + item + "\n"
	}
	return content
}

func TestVendorImportsAreDiscovered(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		loaded []string
	}{
		{
			name: "extensionless import resolves to yaml",
			files: map[string]string{
				"vendor.yaml":            vendorManifest("vendor/networking"),
				"vendor/networking.yaml": ecBroken,
			},
			loaded: []string{"vendor/networking.yaml"},
		},
		{
			name: "extensionless import resolves to yml",
			files: map[string]string{
				"vendor.yaml":           vendorManifest("vendor/networking"),
				"vendor/networking.yml": ecBroken,
			},
			loaded: []string{"vendor/networking.yml"},
		},
		{
			name: "explicit extension",
			files: map[string]string{
				"vendor.yaml":            vendorManifest("vendor/networking.yaml"),
				"vendor/networking.yaml": ecBroken,
			},
			loaded: []string{"vendor/networking.yaml"},
		},
		{
			name: "imports chain through imported manifests",
			files: map[string]string{
				"vendor.yaml":       vendorManifest("vendor/a"),
				"vendor/a.yaml":     vendorManifest("vendor/b"),
				"vendor/b.yaml":     ecBroken,
				"vendor/c.yaml":     ecBroken,
				"not-imported.yaml": ecBroken,
			},
			loaded: []string{"vendor/b.yaml"},
		},
		{
			name: "an import is read relative to the importing manifest",
			files: map[string]string{
				"deep/vendor.yaml":  vendorManifest("sibling"),
				"deep/sibling.yaml": ecBroken,
			},
			loaded: []string{"deep/sibling.yaml"},
		},
		{
			name: "an import cycle terminates",
			files: map[string]string{
				"vendor.yaml":   vendorManifest("vendor/a"),
				"vendor/a.yaml": vendorManifest("vendor/a", "vendor.yaml"),
			},
			loaded: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			for name, content := range tt.files {
				projectFile(t, root, name, content)
			}

			r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})

			assert.Equal(t, tt.loaded, ecLoaded(&r))
		})
	}
}

func TestVendorImportProblemsAreReported(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "vendor.yaml", vendorManifest("vendor/missing", "'{{ .name }}'", "https://example.com/vendor.yaml", "vendor/rendered"))
	projectFile(t, root, "vendor/rendered.yaml.tmpl", "spec: {}\n")

	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleImports}})

	found := ecFindings(&r, ruleImports)
	require.Len(t, found, 4)
	reasons := map[string]int{}
	for _, f := range found {
		if f.Message == "Vendor import does not resolve: vendor/missing" {
			assert.Equal(t, validation.SeverityError, f.Severity)
			assert.Equal(t, statusManual, f.Status)
			assert.Equal(t, 5, f.Line)
			continue
		}
		assert.Equal(t, statusVerificationIncomplete, f.Status)
		reasons[f.Reason]++
	}
	assert.Equal(t, map[string]int{
		"Dynamic or remote import was not evaluated":               2,
		"Template import not evaluated: vendor/rendered.yaml.tmpl": 1,
	}, reasons)
	assert.Empty(t, ecLoaded(&r))
}

func TestVendorImportOutsideTheProjectIsNotRead(t *testing.T) {
	root := newProject(t)
	outside := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, outside, "shared.yaml", ecBroken)
	projectFile(t, root, "vendor.yaml", vendorManifest(filepath.Join(outside, "shared")))

	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleSchema, ruleImports}})

	assert.Empty(t, ecLoaded(&r))
}

// A template vendor manifest imported by several manifests is one notice, as for stack imports.
func TestVendorTemplateImportNoticeIsReportedOncePerTemplate(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "vendor/rendered.yaml.tmpl", "spec: {}\n")
	projectFile(t, root, "vendor.yaml", vendorManifest("vendor/rendered"))
	projectFile(t, root, "sub/vendor.yaml", vendorManifest("vendor/rendered"))

	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleImports}})

	notices := templateImportNotices(&r)
	require.Len(t, notices, 1, "%+v", notices)
	assert.Equal(t, "sub/vendor.yaml", notices[0].File)
	assert.Equal(t, "Template import not evaluated: vendor/rendered.yaml.tmpl (imported by 2 files: sub/vendor.yaml, vendor.yaml)", notices[0].Reason)
	assert.Equal(t, 5, notices[0].Line)
}
