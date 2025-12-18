package workdir

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractSourceConfig(t *testing.T) {
	defer t.Cleanup(func() {})

	tests := []struct {
		name     string
		config   map[string]any
		expected *SourceConfig
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: nil,
		},
		{
			name:     "no metadata",
			config:   map[string]any{"vars": map[string]any{}},
			expected: nil,
		},
		{
			name: "metadata without source",
			config: map[string]any{
				"metadata": map[string]any{
					"component": "test",
				},
			},
			expected: nil,
		},
		{
			name: "source as string",
			config: map[string]any{
				"metadata": map[string]any{
					"source": "github.com/cloudposse/terraform-aws-vpc?ref=v1.0.0",
				},
			},
			expected: &SourceConfig{
				URI: "github.com/cloudposse/terraform-aws-vpc?ref=v1.0.0",
			},
		},
		{
			name: "source as map with uri only",
			config: map[string]any{
				"metadata": map[string]any{
					"source": map[string]any{
						"uri": "github.com/cloudposse/terraform-aws-vpc",
					},
				},
			},
			expected: &SourceConfig{
				URI: "github.com/cloudposse/terraform-aws-vpc",
			},
		},
		{
			name: "source as map with all fields",
			config: map[string]any{
				"metadata": map[string]any{
					"source": map[string]any{
						"uri":            "github.com/cloudposse/terraform-aws-vpc",
						"version":        "1.0.0",
						"included_paths": []any{"*.tf", "modules/**"},
						"excluded_paths": []any{"examples/**"},
					},
				},
			},
			expected: &SourceConfig{
				URI:           "github.com/cloudposse/terraform-aws-vpc",
				Version:       "1.0.0",
				IncludedPaths: []string{"*.tf", "modules/**"},
				ExcludedPaths: []string{"examples/**"},
			},
		},
		{
			name: "source map with empty uri",
			config: map[string]any{
				"metadata": map[string]any{
					"source": map[string]any{
						"version": "1.0.0",
					},
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSourceConfig(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsWorkdirEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name:     "no provision",
			config:   map[string]any{},
			expected: false,
		},
		{
			name: "provision without workdir",
			config: map[string]any{
				"provision": map[string]any{},
			},
			expected: false,
		},
		{
			name: "workdir without enabled",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{},
				},
			},
			expected: false,
		},
		{
			name: "enabled false",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": false,
					},
				},
			},
			expected: false,
		},
		{
			name: "enabled true",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "enabled as string (invalid)",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": "true",
					},
				},
			},
			expected: false,
		},
		{
			name: "workdir as bool instead of map (invalid)",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": true,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWorkdirEnabled(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractComponentName(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		expected string
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: "",
		},
		{
			name: "component in root",
			config: map[string]any{
				"component": "vpc",
			},
			expected: "vpc",
		},
		{
			name: "component in metadata",
			config: map[string]any{
				"metadata": map[string]any{
					"component": "vpc",
				},
			},
			expected: "vpc",
		},
		{
			name: "component in vars (fallback)",
			config: map[string]any{
				"vars": map[string]any{
					"component": "vpc",
				},
			},
			expected: "vpc",
		},
		{
			name: "root takes precedence",
			config: map[string]any{
				"component": "root-vpc",
				"metadata": map[string]any{
					"component": "metadata-vpc",
				},
			},
			expected: "root-vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentName(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildFullURI(t *testing.T) {
	tests := []struct {
		name     string
		source   *SourceConfig
		expected string
	}{
		{
			name: "uri without version",
			source: &SourceConfig{
				URI: "github.com/cloudposse/terraform-aws-vpc",
			},
			expected: "github.com/cloudposse/terraform-aws-vpc",
		},
		{
			name: "uri with version",
			source: &SourceConfig{
				URI:     "github.com/cloudposse/terraform-aws-vpc",
				Version: "v1.0.0",
			},
			expected: "github.com/cloudposse/terraform-aws-vpc?ref=v1.0.0",
		},
		{
			name: "uri already has ref",
			source: &SourceConfig{
				URI:     "github.com/cloudposse/terraform-aws-vpc?ref=main",
				Version: "v1.0.0",
			},
			expected: "github.com/cloudposse/terraform-aws-vpc?ref=main",
		},
		{
			name: "uri with existing query params",
			source: &SourceConfig{
				URI:     "github.com/cloudposse/terraform-aws-vpc?depth=1",
				Version: "v1.0.0",
			},
			expected: "github.com/cloudposse/terraform-aws-vpc?depth=1&ref=v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildFullURI(tt.source)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSemver(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"v1.0.0", true},
		{"1.0.0", true},
		{"v1.2.3", true},
		{"v1.2.3-rc1", true},
		{"v1.2.3-beta.1", true},
		{"main", false},
		{"develop", false},
		{"abc1234", false},
		{"1.0", false},
		{"v1", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isSemver(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsCommitSHA(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"abc1234", true},
		{"1234567", true},
		{"abcdef1234567890abcdef1234567890abcdef12", true},
		{"ABCDEF1234567890ABCDEF1234567890ABCDEF12", true},
		{"abc123", false},  // Too short.
		{"main", false},    // Not hex.
		{"v1.0.0", false},  // Version tag.
		{"ghijkl", false},  // Not hex.
		{"abc123g", false}, // Contains non-hex.
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isCommitSHA(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractRefFromURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "no ref",
			uri:      "github.com/cloudposse/terraform-aws-vpc",
			expected: "",
		},
		{
			name:     "ref at end",
			uri:      "github.com/cloudposse/terraform-aws-vpc?ref=v1.0.0",
			expected: "v1.0.0",
		},
		{
			name:     "ref with other params",
			uri:      "github.com/cloudposse/terraform-aws-vpc?depth=1&ref=main&other=value",
			expected: "main",
		},
		{
			name:     "ref at start of query",
			uri:      "github.com/cloudposse/terraform-aws-vpc?ref=develop&depth=1",
			expected: "develop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRefFromURI(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCacheGetPolicy(t *testing.T) {
	cache := NewDefaultCache()

	tests := []struct {
		name     string
		source   *SourceConfig
		expected CachePolicy
	}{
		{
			name: "tagged version",
			source: &SourceConfig{
				URI:     "github.com/cloudposse/terraform-aws-vpc",
				Version: "v1.0.0",
			},
			expected: CachePolicyPermanent,
		},
		{
			name: "commit SHA",
			source: &SourceConfig{
				URI:     "github.com/cloudposse/terraform-aws-vpc",
				Version: "abc1234567890",
			},
			expected: CachePolicyPermanent,
		},
		{
			name: "branch name",
			source: &SourceConfig{
				URI:     "github.com/cloudposse/terraform-aws-vpc",
				Version: "main",
			},
			expected: CachePolicyTTL,
		},
		{
			name: "tag in URI ref",
			source: &SourceConfig{
				URI: "github.com/cloudposse/terraform-aws-vpc?ref=v2.0.0",
			},
			expected: CachePolicyPermanent,
		},
		{
			name: "branch in URI ref",
			source: &SourceConfig{
				URI: "github.com/cloudposse/terraform-aws-vpc?ref=develop",
			},
			expected: CachePolicyTTL,
		},
		{
			name: "no version info",
			source: &SourceConfig{
				URI: "github.com/cloudposse/terraform-aws-vpc",
			},
			expected: CachePolicyTTL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cache.GetPolicy(tt.source)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCacheGenerateKey(t *testing.T) {
	cache := NewDefaultCache()

	// Same source should produce same key.
	source1 := &SourceConfig{
		URI:     "github.com/cloudposse/terraform-aws-vpc",
		Version: "v1.0.0",
	}
	source2 := &SourceConfig{
		URI:     "github.com/cloudposse/terraform-aws-vpc",
		Version: "v1.0.0",
	}

	key1 := cache.GenerateKey(source1)
	key2 := cache.GenerateKey(source2)

	assert.Equal(t, key1, key2)
	assert.Len(t, key1, 64) // SHA256 hex is 64 chars.

	// Different version should produce different key.
	source3 := &SourceConfig{
		URI:     "github.com/cloudposse/terraform-aws-vpc",
		Version: "v2.0.0",
	}
	key3 := cache.GenerateKey(source3)
	assert.NotEqual(t, key1, key3)
}

func TestNormalizeURI(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"github.com/org/repo", "github.com/org/repo"},
		{"  github.com/org/repo  ", "github.com/org/repo"},
		{"HTTPS://github.com/org/repo", "https://github.com/org/repo"},
		{"Git://github.com/org/repo", "git://github.com/org/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeURI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultPathFilter_Match(t *testing.T) {
	filter := NewDefaultPathFilter()

	tests := []struct {
		name     string
		path     string
		included []string
		excluded []string
		expected bool
	}{
		{
			name:     "no patterns includes all",
			path:     "main.tf",
			included: nil,
			excluded: nil,
			expected: true,
		},
		{
			name:     "matches include pattern",
			path:     "main.tf",
			included: []string{"*.tf"},
			excluded: nil,
			expected: true,
		},
		{
			name:     "does not match include pattern",
			path:     "README.md",
			included: []string{"*.tf"},
			excluded: nil,
			expected: false,
		},
		{
			name:     "matches exclude pattern",
			path:     "test.tf",
			included: []string{"*.tf"},
			excluded: []string{"test*"},
			expected: false,
		},
		{
			name:     "no include but matches exclude",
			path:     "test.tf",
			included: nil,
			excluded: []string{"test*"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filter.Match(tt.path, tt.included, tt.excluded)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
