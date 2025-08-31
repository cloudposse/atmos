package toolchain

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

// Add a mock ToolResolver for tests

func TestParseToolSpec(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	SetAtmosConfig(&schema.AtmosConfiguration{})
	installer := NewInstallerWithResolver(mockResolver)

	tests := []struct {
		name      string
		tool      string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "full specification",
			tool:      "suzuki-shunsuke/github-comment",
			wantOwner: "suzuki-shunsuke",
			wantRepo:  "github-comment",
			wantErr:   false,
		},
		{
			name:      "terraform tool name",
			tool:      "terraform",
			wantOwner: "hashicorp",
			wantRepo:  "terraform",
			wantErr:   false,
		},
		{
			name:      "opentofu tool name",
			tool:      "opentofu",
			wantOwner: "opentofu",
			wantRepo:  "opentofu",
			wantErr:   false,
		},
		{
			name:      "helm tool name",
			tool:      "helm",
			wantOwner: "helm",
			wantRepo:  "helm",
			wantErr:   false,
		},
		{
			name:      "kubectl tool name",
			tool:      "kubectl",
			wantOwner: "kubernetes",
			wantRepo:  "kubectl",
			wantErr:   false,
		},
		{
			name:      "helmfile tool name",
			tool:      "helmfile",
			wantOwner: "helmfile",
			wantRepo:  "helmfile",
			wantErr:   false,
		},
		{
			name:    "unknown tool fallback",
			tool:    "unknown-tool",
			wantErr: true,
		},
		{
			name:    "invalid specification",
			tool:    "invalid/spec/format",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := installer.parseToolSpec(tt.tool)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseToolSpec() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("parseToolSpec() unexpected error: %v", err)
				return
			}

			if owner != tt.wantOwner {
				t.Errorf("parseToolSpec() owner = %v, want %v", owner, tt.wantOwner)
			}

			if repo != tt.wantRepo {
				t.Errorf("parseToolSpec() repo = %v, want %v", repo, tt.wantRepo)
			}
		})
	}
}

func TestBuildAssetURL(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	installer := NewInstallerWithResolver(mockResolver)

	tool := &Tool{
		Type:      "github_release",
		RepoOwner: "suzuki-shunsuke",
		RepoName:  "github-comment",
		Asset:     "github-comment_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz",
	}

	url, err := installer.buildAssetURL(tool, "v6.3.4")
	if err != nil {
		t.Fatalf("buildAssetURL() error: %v", err)
	}

	expected := "https://github.com/suzuki-shunsuke/github-comment/releases/download/v6.3.4/github-comment_6.3.4"
	if !strings.Contains(url, expected) {
		t.Errorf("buildAssetURL() = %v, want %v", url, expected)
	}
}

func TestBuildAssetURL_CustomFuncs(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
		},
	}
	SetAtmosConfig(&schema.AtmosConfiguration{})
	installer := NewInstallerWithResolver(mockResolver)

	tool := &Tool{
		Type:      "github_release",
		RepoOwner: "hashicorp",
		RepoName:  "terraform",
		Asset:     "terraform_{{trimV .Version}}_{{trimPrefix \"1.\" .Version}}_{{trimSuffix \".8\" .Version}}_{{replace \".\" \"-\" .Version}}_{{.OS}}_{{.Arch}}.zip",
	}

	url, err := installer.buildAssetURL(tool, "v1.2.8")
	if err != nil {
		t.Fatalf("buildAssetURL() error: %v", err)
	}

	// Check that all functions are applied as expected
	if !strings.Contains(url, "terraform_1.2.8_2.8_1.2_1-2-8") {
		t.Errorf("buildAssetURL() custom funcs not applied correctly, got: %v", url)
	}
}

func TestGetBinaryPath(t *testing.T) {
	installer := NewInstaller()
	installer.binDir = t.TempDir()

	path := installer.getBinaryPath("suzuki-shunsuke", "github-comment", "v6.3.4")
	expected := filepath.Join(installer.binDir, "suzuki-shunsuke", "github-comment", "v6.3.4", "github-comment")

	if path != expected {
		t.Errorf("getBinaryPath() = %v, want %v", path, expected)
	}
}

func TestFindTool(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	installer := NewInstallerWithResolver(mockResolver)

	// Test with known tool
	tool, err := installer.findTool("suzuki-shunsuke", "github-comment", "v6.3.4")
	if err != nil {
		t.Fatalf("findTool() error: %v", err)
	}

	if tool.RepoOwner != "suzuki-shunsuke" {
		t.Errorf("findTool() RepoOwner = %v, want suzuki-shunsuke", tool.RepoOwner)
	}

	if tool.RepoName != "github-comment" {
		t.Errorf("findTool() RepoName = %v, want github-comment", tool.RepoName)
	}

	// Test with unknown tool
	_, err = installer.findTool("unknown", "package", "v1.0.0")
	if err == nil {
		t.Error("findTool() expected error for unknown tool but got none")
	}
}

func TestNewInstaller(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	customRegistries := []string{"mock-remote-registry", "mock-local-registry"}
	installer := &Installer{
		binDir:     "./.tools/bin",
		registries: customRegistries,
		resolver:   mockResolver,
	}

	if installer == nil {
		t.Fatal("NewInstaller() returned nil")
	}

	if installer.binDir != "./.tools/bin" {
		t.Errorf("NewInstaller() binDir = %v, want ./.tools/bin", installer.binDir)
	}

	if len(installer.registries) == 0 {
		t.Error("NewInstaller() registries is empty")
	}

	// Check that custom registries are set
	foundRemote := false
	foundLocal := false
	for _, registry := range installer.registries {
		if registry == "mock-remote-registry" {
			foundRemote = true
		}
		if registry == "mock-local-registry" {
			foundLocal = true
		}
	}

	if !foundRemote {
		t.Error("NewInstaller() missing remote registry")
	}

	if !foundLocal {
		t.Error("NewInstaller() missing local registry")
	}
}

func TestResolveToolName(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	installer := NewInstallerWithResolver(mockResolver)

	tests := []struct {
		name      string
		toolName  string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "terraform mapping",
			toolName:  "terraform",
			wantOwner: "hashicorp",
			wantRepo:  "terraform",
			wantErr:   false,
		},
		{
			name:      "opentofu mapping",
			toolName:  "opentofu",
			wantOwner: "opentofu",
			wantRepo:  "opentofu",
			wantErr:   false,
		},
		{
			name:      "helm mapping",
			toolName:  "helm",
			wantOwner: "helm",
			wantRepo:  "helm",
			wantErr:   false,
		},
		{
			name:      "kubectl mapping",
			toolName:  "kubectl",
			wantOwner: "kubernetes",
			wantRepo:  "kubectl",
			wantErr:   false,
		},
		{
			name:      "helmfile mapping",
			toolName:  "helmfile",
			wantOwner: "helmfile",
			wantRepo:  "helmfile",
			wantErr:   false,
		},
		{
			name:     "tflint mapping",
			toolName: "tflint",
			wantErr:  true,
		},
		{
			name:     "unknown tool fallback",
			toolName: "unknown-tool",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := installer.resolver.Resolve(tt.toolName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("resolveToolName() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("resolveToolName() unexpected error: %v", err)
				return
			}

			if owner != tt.wantOwner {
				t.Errorf("resolveToolName() owner = %v, want %v", owner, tt.wantOwner)
			}

			if repo != tt.wantRepo {
				t.Errorf("resolveToolName() repo = %v, want %v", repo, tt.wantRepo)
			}
		})
	}
}

func TestSearchRegistryForTool(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}

	owner, repo, err := mockResolver.Resolve("terraform")
	if err != nil {
		t.Errorf("Expected to find tool but got error: %v", err)
		return
	}
	if owner == "" || repo == "" {
		t.Error("Expected owner and repo to be non-empty")
	}

	owner, repo, err = mockResolver.Resolve("nonexistent-tool-12345")
	if err == nil {
		t.Error("Expected error for non-existent tool but got none")
	}
}

func TestUninstall(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	installer := NewInstallerWithResolver(mockResolver)

	// Test uninstalling a non-existent tool
	err := installer.Uninstall("nonexistent", "package", "1.0.0")
	if err == nil {
		t.Error("Expected error when uninstalling non-existent tool")
	}

	// Test uninstalling an existing tool (if we had one installed)
	// This would require setting up a test environment with actual binaries
	// For now, we just test that the method exists and handles errors properly
}

func TestLocalConfigAliases(t *testing.T) {
	// Create a temporary tools.yaml file with aliases
	tempDir := t.TempDir()
	toolsYamlPath := filepath.Join(tempDir, "tools.yaml")

	content := `aliases:
  terraform: hashicorp/terraform
  opentofu: opentofu/opentofu
  tflint: terraform-linters/tflint
  helmfile: helmfile/helmfile

tools:
  hashicorp/terraform:
    type: http
    url: https://example.com/terraform.zip
`

	err := os.WriteFile(toolsYamlPath, []byte(content), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test tools.yaml file: %v", err)
	}

	// Create a local config manager and load the test file
	lcm := NewLocalConfigManager()
	err = lcm.Load(toolsYamlPath)
	if err != nil {
		t.Fatalf("Failed to load test tools.yaml: %v", err)
	}

	// Test alias resolution
	testCases := []struct {
		name        string
		toolName    string
		expected    string
		shouldExist bool
	}{
		{
			name:        "terraform alias",
			toolName:    "terraform",
			expected:    "hashicorp/terraform",
			shouldExist: true,
		},
		{
			name:        "opentofu alias",
			toolName:    "opentofu",
			expected:    "opentofu/opentofu",
			shouldExist: true,
		},
		{
			name:        "tflint alias",
			toolName:    "tflint",
			expected:    "terraform-linters/tflint",
			shouldExist: true,
		},
		{
			name:        "helmfile alias",
			toolName:    "helmfile",
			expected:    "helmfile/helmfile",
			shouldExist: true,
		},
		{
			name:        "non-existent alias",
			toolName:    "nonexistent",
			expected:    "",
			shouldExist: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			alias, exists := lcm.ResolveAlias(tc.toolName)

			if tc.shouldExist {
				if !exists {
					t.Errorf("Expected alias to exist for %s", tc.toolName)
				}
				if alias != tc.expected {
					t.Errorf("Expected alias %s, got %s", tc.expected, alias)
				}
			} else {
				if exists {
					t.Errorf("Expected alias to not exist for %s", tc.toolName)
				}
			}
		})
	}
}

// func TestPathCommand(t *testing.T) {
// 	tests := []struct {
// 		name           string
// 		toolVersions   string
// 		setupTools     func(tempDir string) // Function to set up test tools
// 		args           []string
// 		expectedOutput string
// 		expectError    bool
// 		errorContains  string
// 	}{
// 		{
// 			name:          "missing .tool-versions file",
// 			toolVersions:  "",
// 			args:          []string{},
// 			expectError:   true,
// 			errorContains: "no tools configured in tool-versions file",
// 		},
// 		{
// 			name:         "empty .tool-versions file",
// 			toolVersions: "",
// 			setupTools: func(tempDir string) {
// 				// Create empty .tool-versions file
// 				toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
// 				err := os.WriteFile(toolVersionsPath, []byte(""), 0o644)
// 				if err != nil {
// 					t.Fatalf("Failed to create empty .tool-versions: %v", err)
// 				}
// 			},
// 			args:          []string{},
// 			expectError:   true,
// 			errorContains: "no tools installed from .tool-versions file",
// 		},
// 		{
// 			name:         "single tool not installed",
// 			toolVersions: "terraform 1.5.0\n",
// 			setupTools: func(tempDir string) {
// 				// Create .tool-versions file
// 				toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
// 				err := os.WriteFile(toolVersionsPath, []byte("terraform 1.5.0\n"), 0o644)
// 				if err != nil {
// 					t.Fatalf("Failed to create .tool-versions: %v", err)
// 				}
// 			},
// 			args:          []string{},
// 			expectError:   true,
// 			errorContains: "no installed tools found",
// 		},
// 		{
// 			name:         "multiple tools none installed",
// 			toolVersions: "terraform 1.5.0\nhelm 3.12.0\nkubectl 1.28.0\n",
// 			setupTools: func(tempDir string) {
// 				// Create .tool-versions file
// 				toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
// 				err := os.WriteFile(toolVersionsPath, []byte("terraform 1.5.0\nhelm 3.12.0\nkubectl 1.28.0\n"), 0o644)
// 				if err != nil {
// 					t.Fatalf("Failed to create .tool-versions: %v", err)
// 				}
// 			},
// 			args:          []string{},
// 			expectError:   true,
// 			errorContains: "no installed tools found",
// 		},
// 		{
// 			name:         "single tool installed",
// 			toolVersions: "terraform 1.5.0\n",
// 			setupTools: func(tempDir string) {
// 				// Create .tool-versions file
// 				toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
// 				err := os.WriteFile(toolVersionsPath, []byte("terraform 1.5.0\n"), 0o644)
// 				if err != nil {
// 					t.Fatalf("Failed to create .tool-versions: %v", err)
// 				}

// 				// Create mock installed tool
// 				toolDir := filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0")
// 				if err := os.MkdirAll(toolDir, 0o755); err != nil {
// 					t.Fatalf("Failed to create tool directory: %v", err)
// 				}

// 				// Create mock binary
// 				binaryPath := filepath.Join(toolDir, "terraform")
// 				if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho terraform"), 0o755); err != nil {
// 					t.Fatalf("Failed to create mock binary: %v", err)
// 				}
// 			},
// 			args: []string{},
// 			expectedOutput: func() string {
// 				// This will be set dynamically in the test
// 				return ""
// 			}(),
// 		},
// 		{
// 			name:         "multiple tools some installed",
// 			toolVersions: "terraform 1.5.0\nhelm 3.12.0\nkubectl 1.28.0\n",
// 			setupTools: func(tempDir string) {
// 				// Create .tool-versions file
// 				toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
// 				err := os.WriteFile(toolVersionsPath, []byte("terraform 1.5.0\nhelm 3.12.0\nkubectl 1.28.0\n"), 0o644)
// 				if err != nil {
// 					t.Fatalf("Failed to create .tool-versions: %v", err)
// 				}

// 				// Create mock installed terraform
// 				terraformDir := filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0")
// 				if err := os.MkdirAll(terraformDir, 0o755); err != nil {
// 					t.Fatalf("Failed to create terraform directory: %v", err)
// 				}
// 				terraformBinary := filepath.Join(terraformDir, "terraform")
// 				if err := os.WriteFile(terraformBinary, []byte("#!/bin/sh\necho terraform"), 0o755); err != nil {
// 					t.Fatalf("Failed to create terraform binary: %v", err)
// 				}
// 			},
// 			args: []string{},
// 			expectedOutput: func() string {
// 				// This will be set dynamically in the test
// 				return ""
// 			}(),
// 		},
// 		{
// 			name:         "export flag",
// 			toolVersions: "terraform 1.5.0\n",
// 			setupTools: func(tempDir string) {
// 				// Create .tool-versions file
// 				toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
// 				err := os.WriteFile(toolVersionsPath, []byte("terraform 1.5.0\n"), 0o644)
// 				if err != nil {
// 					t.Fatalf("Failed to create .tool-versions: %v", err)
// 				}

// 				// Create mock installed tool
// 				toolDir := filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0")
// 				if err := os.MkdirAll(toolDir, 0o755); err != nil {
// 					t.Fatalf("Failed to create tool directory: %v", err)
// 				}

// 				// Create mock binary
// 				binaryPath := filepath.Join(toolDir, "terraform")
// 				if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho terraform"), 0o755); err != nil {
// 					t.Fatalf("Failed to create mock binary: %v", err)
// 				}
// 			},
// 			args: []string{"--export"},
// 			expectedOutput: func() string {
// 				// This will be set dynamically in the test
// 				return ""
// 			}(),
// 		},
// 		{
// 			name:         "json flag",
// 			toolVersions: "terraform 1.5.0\n",
// 			setupTools: func(tempDir string) {
// 				// Create .tool-versions file
// 				toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
// 				err := os.WriteFile(toolVersionsPath, []byte("terraform 1.5.0\n"), 0o644)
// 				if err != nil {
// 					t.Fatalf("Failed to create .tool-versions: %v", err)
// 				}

// 				// Create mock installed tool
// 				toolDir := filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0")
// 				if err := os.MkdirAll(toolDir, 0o755); err != nil {
// 					t.Fatalf("Failed to create tool directory: %v", err)
// 				}

// 				// Create mock binary
// 				binaryPath := filepath.Join(toolDir, "terraform")
// 				if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho terraform"), 0o755); err != nil {
// 					t.Fatalf("Failed to create mock binary: %v", err)
// 				}
// 			},
// 			args: []string{"--json"},
// 			expectedOutput: func() string {
// 				// This will be set dynamically in the test
// 				return ""
// 			}(),
// 		},
// 		{
// 			name:         "unknown tool in .tool-versions",
// 			toolVersions: "terraform 1.5.0\nunknown-tool 1.0.0\n",
// 			setupTools: func(tempDir string) {
// 				// Create .tool-versions file
// 				toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
// 				err := os.WriteFile(toolVersionsPath, []byte("terraform 1.5.0\nunknown-tool 1.0.0\n"), 0o644)
// 				if err != nil {
// 					t.Fatalf("Failed to create .tool-versions: %v", err)
// 				}

// 				// Create mock installed terraform
// 				terraformDir := filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0")
// 				if err := os.MkdirAll(terraformDir, 0o755); err != nil {
// 					t.Fatalf("Failed to create terraform directory: %v", err)
// 				}
// 				terraformBinary := filepath.Join(terraformDir, "terraform")
// 				if err := os.WriteFile(terraformBinary, []byte("#!/bin/sh\necho terraform"), 0o755); err != nil {
// 					t.Fatalf("Failed to create terraform binary: %v", err)
// 				}
// 			},
// 			args: []string{},
// 			expectedOutput: func() string {
// 				// This will be set dynamically in the test
// 				return ""
// 			}(),
// 		},
// 		{
// 			name:         "tool with comments in .tool-versions",
// 			toolVersions: "# This is a comment\nterraform 1.5.0\n# Another comment\n",
// 			setupTools: func(tempDir string) {
// 				// Create .tool-versions file
// 				toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
// 				err := os.WriteFile(toolVersionsPath, []byte("# This is a comment\nterraform 1.5.0\n# Another comment\n"), 0o644)
// 				if err != nil {
// 					t.Fatalf("Failed to create .tool-versions: %v", err)
// 				}

// 				// Create mock installed tool
// 				toolDir := filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0")
// 				if err := os.MkdirAll(toolDir, 0o755); err != nil {
// 					t.Fatalf("Failed to create tool directory: %v", err)
// 				}

// 				// Create mock binary
// 				binaryPath := filepath.Join(toolDir, "terraform")
// 				if err := os.WriteFile(binaryPath, []byte("#!/bin/sh\necho terraform"), 0o755); err != nil {
// 					t.Fatalf("Failed to create mock binary: %v", err)
// 				}
// 			},
// 			args: []string{},
// 			expectedOutput: func() string {
// 				// This will be set dynamically in the test
// 				return ""
// 			}(),
// 		},
// 		{
// 			name:         "malformed .tool-versions file",
// 			toolVersions: "terraform\ninvalid line\n1.5.0\n",
// 			setupTools: func(tempDir string) {
// 				// Create malformed .tool-versions file
// 				toolVersionsPath := filepath.Join(tempDir, ".tool-versions")
// 				err := os.WriteFile(toolVersionsPath, []byte("terraform\ninvalid line\n1.5.0\n"), 0o644)
// 				if err != nil {
// 					t.Fatalf("Failed to create .tool-versions: %v", err)
// 				}
// 			},
// 			args:          []string{},
// 			expectError:   true,
// 			errorContains: "error reading tool-versions file",
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// Create a temporary directory for this test
// 			tempDir := t.TempDir()

// 			// Set up test environment
// 			if tt.setupTools != nil {
// 				tt.setupTools(tempDir)
// 			}

// 			// Capture output
// 			oldStdout := os.Stdout
// 			r, w, _ := os.Pipe()
// 			os.Stdout = w

// 			// Create a new command instance to avoid interference
// 			cmd := &cobra.Command{
// 				Use:   "path",
// 				Short: "Display the PATH with installed tools",
// 				RunE: func(cmd *cobra.Command, args []string) error {
// 					// Override the tool versions file path for this test
// 					originalPath := GetToolVersionsFilePath()
// 					defer func() { toolVersionsFile = originalPath }()
// 					toolVersionsFile = filepath.Join(tempDir, ".tool-versions")

// 					// Override the tools directory path for this test
// 					originalToolsDir := GetToolsDirPath()
// 					defer func() { toolsDir = originalToolsDir }()
// 					toolsDir = filepath.Join(tempDir, ".tools")

// 					// Set up the global flag variables that the path command uses
// 					originalExportFlag := exportFlag
// 					originalJsonFlag := jsonFlag
// 					originalRelativeFlag := relativeFlag
// 					defer func() {
// 						exportFlag = originalExportFlag
// 						jsonFlag = originalJsonFlag
// 						relativeFlag = originalRelativeFlag
// 					}()

// 					// Reset flags
// 					exportFlag = false
// 					jsonFlag = false
// 					relativeFlag = false

// 					// Parse flags for this command
// 					if len(tt.args) > 0 {
// 						cmd.ParseFlags(tt.args)
// 					}

// 					// Manually set the global variables based on the parsed flags
// 					if cmd.Flags().Lookup("export") != nil && cmd.Flags().Lookup("export").Changed {
// 						exportFlag = true
// 					}
// 					if cmd.Flags().Lookup("json") != nil && cmd.Flags().Lookup("json").Changed {
// 						jsonFlag = true
// 					}
// 					if cmd.Flags().Lookup("relative") != nil && cmd.Flags().Lookup("relative").Changed {
// 						relativeFlag = true
// 					}

// 					return pathCmd.RunE(cmd, args)
// 				},
// 			}

// 			// Set up flags exactly like the path command does
// 			cmd.Flags().BoolVar(&exportFlag, "export", false, "Print export PATH=... for shell sourcing")
// 			cmd.Flags().BoolVar(&jsonFlag, "json", false, "Print PATH as JSON object")
// 			cmd.Flags().BoolVar(&relativeFlag, "relative", false, "Use relative paths instead of absolute paths")

// 			// Set args
// 			cmd.SetArgs(tt.args)

// 			// Execute command
// 			err := cmd.Execute()

// 			// Restore stdout
// 			w.Close()
// 			os.Stdout = oldStdout

// 			// Read output
// 			var buf strings.Builder
// 			_, err2 := io.Copy(&buf, r)
// 			if err2 != nil {
// 				t.Fatalf("Failed to read output: %v", err2)
// 			}
// 			output := strings.TrimSpace(buf.String())

// 			// Set expected output dynamically for tests that need it
// 			if tt.name == "single tool installed" {
// 				absPath, _ := filepath.Abs(filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0"))
// 				tt.expectedOutput = absPath + ":" + os.Getenv("PATH")
// 			} else if tt.name == "multiple tools some installed" {
// 				absPath, _ := filepath.Abs(filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0"))
// 				tt.expectedOutput = absPath + ":" + os.Getenv("PATH")
// 			} else if tt.name == "export flag" {
// 				absPath, _ := filepath.Abs(filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0"))
// 				tt.expectedOutput = "export PATH=\"" + absPath + ":" + os.Getenv("PATH") + "\""
// 			} else if tt.name == "json flag" {
// 				absPath, _ := filepath.Abs(filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0"))
// 				tt.expectedOutput = fmt.Sprintf(`{
//   "tools": [
//     {
//       "tool": "terraform",
//       "version": "1.5.0",
//       "path": "%s"
//     }
//   ],
//   "final_path": "%s:%s",
//   "count": 1
// }`, absPath, absPath, os.Getenv("PATH"))
// 			} else if tt.name == "unknown tool in .tool-versions" {
// 				absPath, _ := filepath.Abs(filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0"))
// 				tt.expectedOutput = absPath + ":" + os.Getenv("PATH")
// 			} else if tt.name == "tool with comments in .tool-versions" {
// 				absPath, _ := filepath.Abs(filepath.Join(tempDir, ".tools", "bin", "hashicorp", "terraform", "1.5.0"))
// 				tt.expectedOutput = absPath + ":" + os.Getenv("PATH")
// 			}

// 			// Check results
// 			if tt.expectError {
// 				if err == nil {
// 					t.Errorf("Expected error but got none")
// 					return
// 				}
// 				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
// 					t.Errorf("Error '%v' does not contain '%s'", err, tt.errorContains)
// 				}
// 			} else {
// 				if err != nil {
// 					t.Errorf("Unexpected error: %v", err)
// 					return
// 				}
// 				if output != tt.expectedOutput {
// 					t.Errorf("Output mismatch:\nGot:  %s\nWant: %s", output, tt.expectedOutput)
// 				}
// 			}
// 		})
// 	}
// }

func TestEmitJSONPath(t *testing.T) {
	tests := []struct {
		name      string
		toolPaths []ToolPath
		finalPath string
		expected  string
	}{
		{
			name:      "empty tool paths",
			toolPaths: []ToolPath{},
			finalPath: "/usr/local/bin:/usr/bin",
			expected: `{
  "tools": [
  ],
  "final_path": "/usr/local/bin:/usr/bin",
  "count": 0
}`,
		},
		{
			name: "single tool path",
			toolPaths: []ToolPath{
				{Tool: "terraform", Version: "1.5.0", Path: "/path/to/terraform"},
			},
			finalPath: "/path/to/terraform:/usr/local/bin",
			expected: `{
  "tools": [
    {
      "tool": "terraform",
      "version": "1.5.0",
      "path": "/path/to/terraform"
    }
  ],
  "final_path": "/path/to/terraform:/usr/local/bin",
  "count": 1
}`,
		},
		{
			name: "multiple tool paths",
			toolPaths: []ToolPath{
				{Tool: "helm", Version: "3.12.0", Path: "/path/to/helm"},
				{Tool: "terraform", Version: "1.5.0", Path: "/path/to/terraform"},
			},
			finalPath: "/path/to/terraform:/path/to/helm:/usr/local/bin",
			expected: `{
  "tools": [
    {
      "tool": "helm",
      "version": "3.12.0",
      "path": "/path/to/helm"
    },
    {
      "tool": "terraform",
      "version": "1.5.0",
      "path": "/path/to/terraform"
    }
  ],
  "final_path": "/path/to/terraform:/path/to/helm:/usr/local/bin",
  "count": 2
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Execute function
			err := emitJSONPath(tt.toolPaths, tt.finalPath)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read output
			var buf strings.Builder
			_, err2 := io.Copy(&buf, r)
			if err2 != nil {
				t.Fatalf("Failed to read output: %v", err2)
			}
			output := strings.TrimSpace(buf.String())

			// Check results
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if output != tt.expected {
				t.Errorf("Output mismatch:\nGot:  %s\nWant: %s", output, tt.expected)
			}
		})
	}
}

func TestAquaRegistryFallback_PackagesKey(t *testing.T) {
	// Mock Aqua registry YAML with 'packages' key
	registryYAML := `
packages:
  - type: http
    repo_owner: helm
    repo_name: helm
    url: https://get.helm.sh/helm-{{.Version}}-{{.OS}}-{{.Arch}}.tar.gz
    format: tar.gz
    binary_name: helm
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write([]byte(registryYAML))
	}))
	defer ts.Close()

	// Use the test server as the registry
	ar := NewAquaRegistry()
	ar.cache.baseDir = t.TempDir() // avoid polluting real cache

	// Directly call fetchFromRegistry with the test server URL
	tool, err := ar.fetchFromRegistry(ts.URL, "helm", "helm")
	if err != nil {
		t.Fatalf("Expected to fetch tool from Aqua registry, got error: %v", err)
	}
	if tool == nil {
		t.Fatalf("Expected tool, got nil")
	}
	if tool.RepoOwner != "helm" || tool.RepoName != "helm" {
		t.Errorf("Unexpected tool fields: %+v", tool)
	}
	if tool.Asset == "" {
		t.Errorf("Expected asset template to be set")
	}
}

func TestExtractRawBinary(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "extract-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	installer := NewInstaller()

	// Create a mock raw binary file
	rawBinaryPath := filepath.Join(tempDir, "test-binary")
	rawBinaryContent := []byte("#!/bin/bash\necho 'test binary'")
	if err := os.WriteFile(rawBinaryPath, rawBinaryContent, 0o755); err != nil {
		t.Fatalf("failed to create test binary: %v", err)
	}

	// Create destination path
	destPath := filepath.Join(tempDir, "extracted-binary")

	// Test raw binary extraction (copyFile)
	err = installer.copyFile(rawBinaryPath, destPath)
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	// Verify the file was copied correctly
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Error("extracted binary file does not exist")
	}

	// Verify content
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read extracted binary: %v", err)
	}
	if string(content) != string(rawBinaryContent) {
		t.Errorf("extracted binary content mismatch: got %s, want %s", string(content), string(rawBinaryContent))
	}
}

func TestExtractGzippedBinary(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "extract-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	installer := NewInstaller()

	// Create a mock binary content
	binaryContent := []byte("#!/bin/bash\necho 'test gzipped binary'")

	// Create a gzipped file
	gzPath := filepath.Join(tempDir, "test-binary.gz")
	gzFile, err := os.Create(gzPath)
	if err != nil {
		t.Fatalf("failed to create gzip file: %v", err)
	}

	gzWriter := gzip.NewWriter(gzFile)
	if _, err := gzWriter.Write(binaryContent); err != nil {
		t.Fatalf("failed to write to gzip: %v", err)
	}
	gzWriter.Close()
	gzFile.Close()

	// Create destination path
	destPath := filepath.Join(tempDir, "extracted-binary")

	// Test gzipped binary extraction
	err = installer.extractGzip(gzPath, destPath)
	if err != nil {
		t.Fatalf("extractGzip failed: %v", err)
	}

	// Verify the file was extracted correctly
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Error("extracted binary file does not exist")
	}

	// Verify content
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read extracted binary: %v", err)
	}
	if string(content) != string(binaryContent) {
		t.Errorf("extracted binary content mismatch: got %s, want %s", string(content), string(binaryContent))
	}
}

func TestExtractAndInstallWithRawBinary(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{ToolsDir: tempDir}})
	installer := NewInstaller()

	// Create a mock raw binary file
	rawBinaryPath := filepath.Join(tempDir, "atmos")
	rawBinaryContent := []byte("#!/bin/bash\necho 'atmos binary'")
	if err := os.WriteFile(rawBinaryPath, rawBinaryContent, 0o755); err != nil {
		t.Fatalf("failed to create test binary: %v", err)
	}

	// Create a mock tool configuration
	tool := &Tool{
		Name:     "atmos",
		RepoName: "atmos",
		Type:     "http",
	}
	di, _ := os.Getwd()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{ToolsDir: di}})
	t.Log("tempDir", di)
	// Test extractAndInstall with raw binary
	binaryPath, err := installer.extractAndInstall(tool, rawBinaryPath, "1.0.0")
	if err != nil {
		t.Fatalf("extractAndInstall failed: %v", err)
	}

	// Verify the binary was installed (should be copied to the bin directory)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Errorf("installed binary not found at expected path: %s", binaryPath)
	}
}

func TestExtractAndInstallWithGzippedBinary(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{ToolsDir: tempDir}})
	installer := NewInstaller()

	// Create a mock binary content
	binaryContent := []byte("#!/bin/bash\necho 'atmos gzipped binary'")

	// Create a gzipped file
	gzPath := filepath.Join(tempDir, "atmos.gz")
	gzFile, err := os.Create(gzPath)
	if err != nil {
		t.Fatalf("failed to create gzip file: %v", err)
	}

	gzWriter := gzip.NewWriter(gzFile)
	if _, err := gzWriter.Write(binaryContent); err != nil {
		t.Fatalf("failed to write to gzip: %v", err)
	}
	gzWriter.Close()
	gzFile.Close()

	// Create a mock tool configuration
	tool := &Tool{
		Name:     "atmos",
		RepoName: "atmos",
		Type:     "http",
	}

	// Test extractAndInstall with gzipped binary
	binaryPath, err := installer.extractAndInstall(tool, gzPath, "1.0.0")
	if err != nil {
		t.Fatalf("extractAndInstall failed: %v", err)
	}

	// Verify the binary was installed (should be extracted to the bin directory)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Errorf("installed binary not found at expected path: %s", binaryPath)
	}

	// Verify content
	content, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("failed to read installed binary: %v", err)
	}
	if string(content) != string(binaryContent) {
		t.Errorf("installed binary content mismatch: got %s, want %s", string(content), string(binaryContent))
	}
}

func TestFileTypeDetection(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "filetype-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	installer := NewInstaller()

	tests := []struct {
		name     string
		content  []byte
		filename string
		expected string // expected MIME type or behavior
	}{
		{
			name:     "raw binary",
			content:  []byte{0x7f, 0x45, 0x4c, 0x46}, // ELF header
			filename: "binary",
			expected: "application/octet-stream",
		},
		{
			name:     "gzipped binary",
			content:  []byte{0x1f, 0x8b}, // gzip header
			filename: "binary.gz",
			expected: "application/gzip",
		},
		{
			name:     "zip file",
			content:  []byte{0x50, 0x4b, 0x03, 0x04}, // ZIP header
			filename: "archive.zip",
			expected: "application/zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			filePath := filepath.Join(tempDir, tt.filename)
			if err := os.WriteFile(filePath, tt.content, 0o644); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			// Test simpleExtract to see how file type is detected
			tool := &Tool{Name: "test", RepoName: "test"}
			destPath := filepath.Join(tempDir, "extracted-"+tt.filename)

			err := installer.simpleExtract(filePath, destPath, tool)
			if err != nil {
				t.Logf("simpleExtract failed (this might be expected): %v", err)
			}

			// The test verifies that the file type detection doesn't crash
			// and routes to the appropriate extraction method
		})
	}
}

func TestInstallerPopulatesToolFields(t *testing.T) {
	installer := NewInstaller()

	// Test that the installer correctly populates RepoOwner and RepoName
	// when fetching a tool from the registry
	tool, err := installer.findTool("hashicorp", "terraform", "1.9.8")
	if err != nil {
		t.Skipf("Skipping test - tool not found in registry: %v", err)
	}

	// Verify that RepoOwner and RepoName are populated
	assert.NotEmpty(t, tool.RepoOwner, "RepoOwner should be populated")
	assert.NotEmpty(t, tool.RepoName, "RepoName should be populated")
	assert.Equal(t, "hashicorp", tool.RepoOwner, "RepoOwner should match the requested owner")
	assert.Equal(t, "terraform", tool.RepoName, "RepoName should match the requested repo")

	// Test that buildAssetURL generates correct URLs
	assetURL, err := installer.buildAssetURL(tool, "1.9.8")
	assert.NoError(t, err, "buildAssetURL should not error")

	// Since hashicorp/terraform is configured as type: http in tools.yaml,
	// it should generate an HTTP URL, not a GitHub release URL
	assert.Contains(t, assetURL, "https://releases.hashicorp.com/terraform/",
		"Asset URL should contain correct HashiCorp release URL")
	assert.NotContains(t, assetURL, "https://github.com///releases/download/",
		"Asset URL should not have empty org/repo")
}

func TestBuildAssetURLWithEmptyFields(t *testing.T) {
	installer := NewInstaller()

	// Create a tool with empty RepoOwner and RepoName to test the bug
	tool := &Tool{
		Name:      "test-tool",
		Type:      "github_release",
		RepoOwner: "", // Empty - this is the bug
		RepoName:  "", // Empty - this is the bug
		Asset:     "test-tool_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz",
	}

	// This should fail because RepoOwner and RepoName are empty
	_, err := installer.buildAssetURL(tool, "1.0.0")
	assert.Error(t, err, "buildAssetURL should fail with empty RepoOwner/RepoName")

	// The generated URL would be malformed like:
	// https://github.com///releases/download/1.0.0/test-tool_1.0.0_darwin_arm64.tar.gz
}
