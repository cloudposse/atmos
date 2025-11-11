package merge

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestYAMLMerger_CleanMerges(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		ours    string
		theirs  string
		want    string
		wantErr bool
	}{
		{
			name: "user changes value, template adds key",
			base: `components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
`,
			ours: `components:
  terraform:
    base_path: "infrastructure/terraform"
    apply_auto_approve: false
`,
			theirs: `components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
    deploy_run_init: true
`,
			want: `components:
  terraform:
    base_path: "infrastructure/terraform"
    apply_auto_approve: false
    deploy_run_init: true
`,
			wantErr: false,
		},
		{
			name: "template adds multiple keys",
			base: `settings:
  enabled: true
`,
			ours: `settings:
  enabled: true
  timeout: 30
`,
			theirs: `settings:
  enabled: true
  retries: 3
  log_level: info
`,
			// Note: Key order follows "ours" first, then template additions
			want: `settings:
  enabled: true
  timeout: 30
  retries: 3
  log_level: info
`,
			wantErr: false,
		},
		{
			name: "user and template modify different nested keys",
			base: `database:
  host: localhost
  port: 5432
  credentials:
    user: admin
    password: secret
`,
			ours: `database:
  host: prod-db.example.com
  port: 5432
  credentials:
    user: admin
    password: secret
`,
			theirs: `database:
  host: localhost
  port: 5432
  credentials:
    user: admin
    password: secret
    ssl: true
`,
			// Note: YAML preserves "ours" key ordering at all levels
			want: `database:
  host: prod-db.example.com
  port: 5432
  credentials:
    user: admin
    password: secret
    ssl: true
`,
			wantErr: false,
		},
		{
			name: "identical changes",
			base: `version: 1
name: myapp
`,
			ours: `version: 2
name: myapp
`,
			theirs: `version: 2
name: myapp
`,
			want: `version: 2
name: myapp
`,
			wantErr: false,
		},
		{
			name:    "no changes",
			base:    "key: value\n",
			ours:    "key: value\n",
			theirs:  "key: value\n",
			want:    "key: value\n",
			wantErr: false,
		},
		{
			name: "user adds key, template adds different key",
			base: `config:
  feature_a: true
`,
			ours: `config:
  feature_a: true
  feature_b: true
`,
			theirs: `config:
  feature_a: true
  feature_c: true
`,
			want: `config:
  feature_a: true
  feature_b: true
  feature_c: true
`,
			wantErr: false,
		},
		{
			name: "deep nesting with changes at different levels",
			base: `root:
  level1:
    level2:
      value: original
`,
			ours: `root:
  level1:
    level2:
      value: original
      user_added: true
`,
			theirs: `root:
  level1:
    level2:
      value: original
    template_added: true
`,
			want: `root:
  level1:
    level2:
      value: original
      user_added: true
    template_added: true
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewYAMLMerger(50)
			result, err := merger.Merge(tt.base, tt.ours, tt.theirs)

			if tt.wantErr && err == nil {
				t.Errorf("Expected error, got nil")
				return
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if err == nil {
				// Normalize whitespace for comparison
				got := strings.TrimSpace(result.Content)
				want := strings.TrimSpace(tt.want)

				if got != want {
					t.Errorf("Merge result mismatch\nGot:\n%s\n\nWant:\n%s", got, want)
				}

				if result.HasConflicts {
					t.Errorf("Unexpected conflicts in clean merge: %d conflicts", result.ConflictCount)
				}
			}
		})
	}
}

func TestYAMLMerger_Conflicts(t *testing.T) {
	tests := []struct {
		name              string
		base              string
		ours              string
		theirs            string
		threshold         int
		wantConflicts     bool
		wantConflictCount int
		wantErr           bool
	}{
		{
			name: "both modify same scalar value",
			base: `config:
  version: 1
`,
			ours: `config:
  version: 2
`,
			theirs: `config:
  version: 3
`,
			threshold:         50,
			wantConflicts:     true,
			wantConflictCount: 1,
			wantErr:           false,
		},
		{
			name: "multiple conflicting scalar values",
			base: `settings:
  timeout: 30
  retries: 3
  enabled: true
`,
			ours: `settings:
  timeout: 60
  retries: 5
  enabled: true
`,
			theirs: `settings:
  timeout: 45
  retries: 10
  enabled: true
`,
			threshold:         50,
			wantConflicts:     true,
			wantConflictCount: 2,
			wantErr:           false,
		},
		{
			name: "conflicting list modifications",
			base: `allowed_ips:
  - 192.168.1.1
  - 192.168.1.2
`,
			ours: `allowed_ips:
  - 192.168.1.1
  - 10.0.0.1
`,
			theirs: `allowed_ips:
  - 192.168.1.1
  - 172.16.0.1
`,
			threshold:         100, // Increase threshold - one conflict in single-key doc is 100%
			wantConflicts:     true,
			wantConflictCount: 1,
			wantErr:           false,
		},
		{
			name: "conflicts exceeding threshold",
			base: `a: 1
b: 2
c: 3
`,
			ours: `a: 10
b: 20
c: 30
`,
			theirs: `a: 100
b: 200
c: 300
`,
			threshold:         10, // Low threshold
			wantConflicts:     true,
			wantConflictCount: 3,
			wantErr:           true, // Should exceed threshold
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewYAMLMerger(tt.threshold)
			result, err := merger.Merge(tt.base, tt.ours, tt.theirs)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error due to threshold, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result.HasConflicts != tt.wantConflicts {
				t.Errorf("HasConflicts = %v, want %v", result.HasConflicts, tt.wantConflicts)
			}

			if result.ConflictCount != tt.wantConflictCount {
				t.Errorf("ConflictCount = %d, want %d", result.ConflictCount, tt.wantConflictCount)
			}
		})
	}
}

func TestYAMLMerger_CommentPreservation(t *testing.T) {
	tests := []struct {
		name           string
		base           string
		ours           string
		theirs         string
		wantHasKey     string   // Key that should be in result
		wantComments   []string // Comments that should be preserved
		wantNoComments []string // Comments that should not be in result
	}{
		{
			name: "preserves head comments",
			base: `# Base comment
key: value
`,
			ours: `# User comment
key: value
`,
			theirs: `# Template comment
key: value
`,
			wantHasKey:   "key",
			wantComments: []string{"# User comment"},
			// Template comment should not appear since we prefer user's version
			wantNoComments: []string{"# Template comment"},
		},
		{
			name: "preserves comments when adding keys",
			base: `config:
  # Important setting
  enabled: true
`,
			ours: `config:
  # Important setting
  enabled: true
  # User's setting
  timeout: 30
`,
			theirs: `config:
  # Important setting
  enabled: true
  # Template's setting
  retries: 3
`,
			wantHasKey: "timeout",
			wantComments: []string{
				"# User's setting",
				"# Template's setting",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewYAMLMerger(50)
			result, err := merger.Merge(tt.base, tt.ours, tt.theirs)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Check that result is valid YAML
			if !strings.Contains(result.Content, tt.wantHasKey) {
				t.Errorf("Expected result to contain key %q, but it doesn't.\nGot:\n%s", tt.wantHasKey, result.Content)
			}

			// Check expected comments are present
			for _, comment := range tt.wantComments {
				if !strings.Contains(result.Content, comment) {
					t.Errorf("Expected result to contain comment %q\nGot:\n%s", comment, result.Content)
				}
			}

			// Check unwanted comments are not present
			for _, comment := range tt.wantNoComments {
				if strings.Contains(result.Content, comment) {
					t.Errorf("Expected result NOT to contain comment %q\nGot:\n%s", comment, result.Content)
				}
			}
		})
	}
}

func TestYAMLMerger_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		ours    string
		theirs  string
		wantErr bool
	}{
		{
			name:    "empty base",
			base:    "",
			ours:    "key: value\n",
			theirs:  "key: value\n",
			wantErr: false, // Empty YAML parses to empty document
		},
		{
			name:    "empty ours",
			base:    "key: value\n",
			ours:    "",
			theirs:  "key: value\n",
			wantErr: false, // Empty YAML parses to empty document
		},
		{
			name:    "null values",
			base:    "key: null\n",
			ours:    "key: null\n",
			theirs:  "key: value\n",
			wantErr: false,
		},
		{
			name: "boolean values",
			base: `enabled: false
debug: true
`,
			ours: `enabled: true
debug: true
`,
			theirs: `enabled: false
debug: false
`,
			wantErr: false,
		},
		{
			name: "numeric values",
			base: `count: 1
timeout: 30.5
`,
			ours: `count: 2
timeout: 30.5
`,
			theirs: `count: 1
timeout: 60.0
`,
			wantErr: false,
		},
		{
			name: "multiline strings",
			base: `description: |
  This is a
  multiline string
`,
			ours: `description: |
  This is a
  modified string
`,
			theirs: `description: |
  This is a
  multiline string
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewYAMLMerger(100) // High threshold for edge cases
			result, err := merger.Merge(tt.base, tt.ours, tt.theirs)

			if tt.wantErr && err == nil {
				t.Errorf("Expected error, got nil")
				return
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if err == nil && result == nil {
				t.Errorf("Expected non-nil result when no error occurred")
			}
		})
	}
}

func TestYAMLMerger_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name           string
		base           string
		ours           string
		theirs         string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "atmos.yaml: user changes path, template adds features",
			base: `components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
`,
			ours: `components:
  terraform:
    base_path: "infrastructure/terraform"
    apply_auto_approve: false
`,
			theirs: `components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
    deploy_run_init: true
    auto_generate_backend_file: true
`,
			wantContains: []string{
				"infrastructure/terraform", // User's change
				"deploy_run_init: true",    // Template's addition
				"auto_generate_backend_file: true",
			},
			wantNotContain: []string{
				"components/terraform", // Should be replaced by user's version
			},
		},
		{
			name: "stack config: user adds vars, template adds settings",
			base: `components:
  terraform:
    vpc:
      vars:
        cidr_block: "10.0.0.0/16"
`,
			ours: `components:
  terraform:
    vpc:
      vars:
        cidr_block: "10.1.0.0/16"
        enable_dns: true
`,
			theirs: `components:
  terraform:
    vpc:
      settings:
        depends_on:
          - networking/config
      vars:
        cidr_block: "10.0.0.0/16"
`,
			wantContains: []string{
				"10.1.0.0/16",      // User's CIDR
				"enable_dns: true", // User's var
				"depends_on",       // Template's setting
			},
			wantNotContain: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewYAMLMerger(50)
			result, err := merger.Merge(tt.base, tt.ours, tt.theirs)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result.Content, want) {
					t.Errorf("Expected result to contain %q\nGot:\n%s", want, result.Content)
				}
			}

			for _, dontWant := range tt.wantNotContain {
				if strings.Contains(result.Content, dontWant) {
					t.Errorf("Expected result NOT to contain %q\nGot:\n%s", dontWant, result.Content)
				}
			}

			t.Logf("Merge result:\n%s", result.Content)
		})
	}
}

func TestYAMLMerger_KeyDeletion(t *testing.T) {
	tests := []struct {
		name          string
		base          string
		ours          string
		theirs        string
		wantHasKey    string
		wantNotHasKey string
	}{
		{
			name: "user deletes key, template keeps it - preserve deletion",
			base: `settings:
  feature_a: true
  feature_b: true
  feature_c: true
`,
			ours: `settings:
  feature_a: true
  feature_c: true
`,
			theirs: `settings:
  feature_a: true
  feature_b: true
  feature_c: true
`,
			wantHasKey:    "feature_a", // Should have feature_a and feature_c, not feature_b
			wantNotHasKey: "feature_b",
		},
		{
			name: "template deletes key, user keeps it - preserve user's version",
			base: `settings:
  old_feature: true
  new_feature: false
`,
			ours: `settings:
  old_feature: true
  new_feature: false
  user_feature: true
`,
			theirs: `settings:
  new_feature: false
`,
			wantHasKey:    "old_feature", // User kept it, so preserve it
			wantNotHasKey: "",            // Nothing should be deleted in this case
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewYAMLMerger(50)
			result, err := merger.Merge(tt.base, tt.ours, tt.theirs)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !strings.Contains(result.Content, tt.wantHasKey) {
				t.Errorf("Expected result to contain key %q\nGot:\n%s", tt.wantHasKey, result.Content)
			}

			if tt.wantNotHasKey != "" && strings.Contains(result.Content, tt.wantNotHasKey) {
				t.Errorf("Expected result NOT to contain key %q\nGot:\n%s", tt.wantNotHasKey, result.Content)
			}
		})
	}
}

func TestYAMLMerger_PreservesTagsAndStyle(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		ours     string
		theirs   string
		wantTag  string
		wantStr  string
	}{
		{
			name: "preserves explicit !!str tag when user changes value",
			base: `value: "123"
`,
			ours: `value: !!str 456
`,
			theirs: `value: "123"
`,
			wantTag: "!!str",
			wantStr: "456",
		},
		{
			name: "preserves folded scalar style",
			base: `description: short
`,
			ours: `description: >
  This is a folded
  scalar that spans
  multiple lines
`,
			theirs: `description: short
`,
			wantStr: "This is a folded scalar that spans multiple lines",
		},
		{
			name: "preserves literal scalar style",
			base: `script: echo hello
`,
			ours: `script: |
  line one
  line two
  line three
`,
			theirs: `script: echo hello
`,
			wantStr: "line one\nline two\nline three",
		},
		{
			name: "preserves tag when user changes value with explicit tag",
			base: `port: 8080
`,
			ours: `port: !!str 9000
`,
			theirs: `port: 8080
`,
			wantTag: "!!str",
			wantStr: "9000", // User's version wins
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewYAMLMerger(50)
			result, err := merger.Merge(tt.base, tt.ours, tt.theirs)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Parse result to check node properties
			var resultNode yaml.Node
			if err := yaml.Unmarshal([]byte(result.Content), &resultNode); err != nil {
				t.Fatalf("Failed to parse merge result: %v", err)
			}

			// Navigate to the value node
			if len(resultNode.Content) == 0 || resultNode.Content[0].Kind != yaml.MappingNode {
				t.Fatal("Result is not a mapping")
			}

			mapping := resultNode.Content[0]
			if len(mapping.Content) < 2 {
				t.Fatal("Result mapping is empty")
			}

			valueNode := mapping.Content[1]

			// Check tag if specified
			if tt.wantTag != "" && valueNode.Tag != tt.wantTag {
				t.Errorf("Expected tag %q, got %q", tt.wantTag, valueNode.Tag)
			}

			// Check value content
			if tt.wantStr != "" {
				// Normalize whitespace for comparison
				gotValue := strings.TrimSpace(valueNode.Value)
				wantValue := strings.TrimSpace(tt.wantStr)
				if gotValue != wantValue {
					t.Errorf("Expected value %q, got %q", wantValue, gotValue)
				}
			}

			t.Logf("Result:\n%s", result.Content)
		})
	}
}
