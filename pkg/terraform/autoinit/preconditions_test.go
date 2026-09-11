package autoinit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validLockFile = `provider "registry.terraform.io/hashicorp/null" {
  version     = "3.2.1"
  constraints = "3.2.1"
  hashes = [
    "h1:abc123",
  ]
}
`

const emptyLockFile = `# no providers
`

func TestProvidersMissing(t *testing.T) {
	tests := []struct {
		name        string
		lockContent string
		makeDirs    []string
		want        bool
	}{
		{
			name:        "lock has providers, neither dir exists",
			lockContent: validLockFile,
			makeDirs:    nil,
			want:        true,
		},
		{
			name:        "lock has providers, providers dir exists",
			lockContent: validLockFile,
			makeDirs:    []string{"providers"},
			want:        false,
		},
		{
			name:        "lock has providers, plugins dir exists",
			lockContent: validLockFile,
			makeDirs:    []string{"plugins"},
			want:        false,
		},
		{
			name:        "lock has zero providers",
			lockContent: emptyLockFile,
			makeDirs:    nil,
			want:        false,
		},
		{
			name:        "no lock file",
			lockContent: "",
			makeDirs:    nil,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentPath := t.TempDir()
			if tt.lockContent != "" {
				require.NoError(t, os.WriteFile(filepath.Join(componentPath, ".terraform.lock.hcl"), []byte(tt.lockContent), 0o644))
			}

			dataDir := filepath.Join(componentPath, ".terraform")
			for _, d := range tt.makeDirs {
				require.NoError(t, os.MkdirAll(filepath.Join(dataDir, d), 0o755))
			}

			assert.Equal(t, tt.want, providersMissing(dataDir, componentPath))
		})
	}
}

func TestModulesMissing(t *testing.T) {
	tests := []struct {
		name            string
		configFile      string
		configContent   string
		manifestPresent bool
		want            bool
	}{
		{
			name:            "hcl module block, no manifest",
			configFile:      "main.tf",
			configContent:   `module "vpc" { source = "./vpc" }`,
			manifestPresent: false,
			want:            true,
		},
		{
			name:            "hcl module block, manifest present",
			configFile:      "main.tf",
			configContent:   `module "vpc" { source = "./vpc" }`,
			manifestPresent: true,
			want:            false,
		},
		{
			name:            "json module key, no manifest",
			configFile:      "main.tf.json",
			configContent:   `{"module":{"vpc":{"source":"./vpc"}}}`,
			manifestPresent: false,
			want:            true,
		},
		{
			name:            "no module declared, no manifest",
			configFile:      "main.tf",
			configContent:   `resource "null_resource" "x" {}`,
			manifestPresent: false,
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentPath := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(componentPath, tt.configFile), []byte(tt.configContent), 0o644))

			dataDir := filepath.Join(componentPath, ".terraform")
			if tt.manifestPresent {
				modulesDir := filepath.Join(dataDir, "modules")
				require.NoError(t, os.MkdirAll(modulesDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(modulesDir, "modules.json"), []byte("{}"), 0o644))
			}

			assert.Equal(t, tt.want, modulesMissing(dataDir, componentPath))
		})
	}
}

func TestBackendStateMissing(t *testing.T) {
	tests := []struct {
		name          string
		configFile    string
		configContent string
		statePresent  bool
		want          bool
	}{
		{
			name:          "backend.tf.json present, no state",
			configFile:    "backend.tf.json",
			configContent: `{"terraform":{"backend":{"s3":{}}}}`,
			statePresent:  false,
			want:          true,
		},
		{
			name:          "hcl backend block, no state",
			configFile:    "main.tf",
			configContent: "terraform {\n  backend \"s3\" {}\n}",
			statePresent:  false,
			want:          true,
		},
		{
			name:          "hcl backend block, state present",
			configFile:    "main.tf",
			configContent: "terraform {\n  backend \"s3\" {}\n}",
			statePresent:  true,
			want:          false,
		},
		{
			name:          "no backend declared",
			configFile:    "main.tf",
			configContent: `resource "null_resource" "x" {}`,
			statePresent:  false,
			want:          false,
		},
		{
			name:          "cloud block, no state",
			configFile:    "main.tf",
			configContent: "terraform {\n  cloud {\n    organization = \"acme\"\n  }\n}",
			statePresent:  false,
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentPath := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(componentPath, tt.configFile), []byte(tt.configContent), 0o644))

			dataDir := filepath.Join(componentPath, ".terraform")
			if tt.statePresent {
				require.NoError(t, os.MkdirAll(dataDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dataDir, "terraform.tfstate"), []byte("{}"), 0o644))
			}

			assert.Equal(t, tt.want, backendStateMissing(dataDir, componentPath))
		})
	}
}
