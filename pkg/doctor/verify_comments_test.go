package doctor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// stageEdits plans and applies every repair the doctor finds in content.
func stageEdits(t *testing.T, kind, content string) (after []byte, edits []edit) {
	t.Helper()
	d := &document{path: filepath.Join(t.TempDir(), "doc.yaml"), kind: kind, content: []byte(content)}
	var err error
	d.node, err = parse(d.content)
	require.NoError(t, err)
	edits, err = scanDocument(d, &project{complete: true, configCount: 1}, &Options{}, &Report{}, nil)
	require.NoError(t, err)
	require.NotEmpty(t, edits, "the fixture must contain a repair")
	after, err = transformDocument(d, edits)
	require.NoError(t, err)
	return after, edits
}

func commentTexts(content string) []string {
	var comments []string
	for _, line := range strings.Split(content, "\n") {
		if _, comment, ok := strings.Cut(line, "#"); ok {
			comments = append(comments, "#"+strings.TrimRight(comment, " \r"))
		}
	}
	return comments
}

func TestCommentsSurviveRepairsOfCommentedKeys(t *testing.T) {
	const reconfigure = "components:\n  terraform:\n    base_path: c\n"
	tests := []struct {
		name, kind, content string
	}{
		{"head comment on the legacy key", documentConfig, reconfigure + "    # Legacy reconfigure policy.\n    init_run_reconfigure: false\n    auto_generate_backend_file: false\n"},
		{"inline comment on the legacy key", documentConfig, reconfigure + "    init_run_reconfigure: true # keep reconfiguring\n    auto_generate_backend_file: false\n"},
		{"head, inline, and following comments", documentConfig, reconfigure + "    # First line.\n    # Second line.\n    init_run_reconfigure: false # inline\n    # Belongs to the next key.\n    auto_generate_backend_file: false\n"},
		{"legacy key is the last key of the document", documentConfig, reconfigure + "    # Why.\n    init_run_reconfigure: false # inline\n"},
		{"legacy key beside an existing init mapping", documentConfig, reconfigure + "    # Why.\n    init_run_reconfigure: false # inline\n    init:\n      # Keep variables.\n      pass_vars: true\n"},
		{"no comments", documentConfig, reconfigure + "    init_run_reconfigure: false\n"},
		{"two trailing comment blocks", documentConfig, "components:\n  terraform:\n    base_path: c\n    init_run_reconfigure: true\nlogs:\n  level: Info\n# a\n\n# b\n"},
		{"two trailing comment blocks under a nested key", documentConfig, "components:\n  terraform:\n    base_path: c\n    init_run_reconfigure: true\nschemas:\n  opa:\n    base_path: x\n# settings:\n#   terminal:\n\n# integrations:\n#   github: {}\n"},
		{"trailing blocks after the legacy key", documentConfig, "components:\n  terraform:\n    init_run_reconfigure: true\n    # about the key\n# closing a\n\n# closing b\n"},
		{"argument type with comments", documentConfig, "commands:\n  - name: c\n    arguments:\n      # Which component.\n      - name: target\n        type: component # legacy\n"},
		{"flag semantic type with comments", documentConfig, "commands:\n  - name: c\n    flags:\n      - name: stack\n        # Legacy.\n        semantic_type: stack # inline\n"},
		{"hook event with an inline comment", "stack", "hooks:\n  notify:\n    # Runs after apply.\n    events: [after-terraform-apply] # legacy\n    command: store\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			after, edits := stageEdits(t, tt.kind, tt.content)

			require.NoError(t, verifyDocument([]byte(tt.content), after, edits))
			want := commentTexts(tt.content)
			got := commentTexts(string(after))
			require.ElementsMatch(t, want, got, "every comment must survive exactly once:\n%s", after)
		})
	}
}

func TestReplacementKeyCarriesTheLegacyKeyComments(t *testing.T) {
	content := "components:\n  terraform:\n    # Legacy reconfigure policy.\n    init_run_reconfigure: false # keep it off\n"

	after, _ := stageEdits(t, documentConfig, content)

	require.Contains(t, string(after), "# Legacy reconfigure policy.\n")
	require.Contains(t, string(after), "reconfigure: never # keep it off")
	require.NotContains(t, string(after), "init_run_reconfigure")
}

func TestVerifyCommentsRejectsAnyOtherChange(t *testing.T) {
	const before = "# header\ncomponents:\n  terraform:\n    # Legacy.\n    init_run_reconfigure: false # inline\n    auto_generate_backend_file: true # untouched\nlogs:\n  level: Info\n# a\n\n# b\n"
	d := &document{path: "atmos.yaml", kind: documentConfig, content: []byte(before)}
	var err error
	d.node, err = parse(d.content)
	require.NoError(t, err)
	edits, err := scanDocument(d, &project{complete: true, configCount: 1}, &Options{}, &Report{}, nil)
	require.NoError(t, err)
	require.Len(t, edits, 1)
	good := "# header\ncomponents:\n  terraform:\n    auto_generate_backend_file: true # untouched\n    init:\n      # Legacy.\n      reconfigure: never # inline\nlogs:\n  level: Info\n# a\n\n# b\n"
	require.NoError(t, verifyDocument([]byte(before), []byte(good), edits), "the control document must verify")

	tests := []struct {
		name, after, want string
	}{
		{"replaced key's head comment dropped", strings.Replace(good, "      # Legacy.\n", "", 1), "replaced key"},
		{"replaced key's inline comment dropped", strings.Replace(good, " # inline", "", 1), "replaced key"},
		{"replaced key's comment duplicated", strings.Replace(good, "      # Legacy.\n", "      # Legacy.\n      # Legacy.\n", 1), "replaced key"},
		{"replaced key's comment rewritten", strings.Replace(good, "# Legacy.", "# Legacy!", 1), "replaced key"},
		{"untouched inline comment dropped", strings.Replace(good, " # untouched", "", 1), "comments changed"},
		{"untouched inline comment rewritten", strings.Replace(good, "# untouched", "# touched", 1), "comments changed"},
		{"header comment dropped", strings.Replace(good, "# header\n", "", 1), "comments changed"},
		{"trailing block dropped", strings.Replace(good, "\n\n# b", "", 1), "end of the document"},
		{"trailing block rewritten", strings.Replace(good, "# b", "# c", 1), "end of the document"},
		{"trailing blocks reordered", strings.Replace(strings.Replace(strings.Replace(good, "# a", "# X", 1), "# b", "# a", 1), "# X", "# b", 1), "end of the document"},
		{"trailing block added", good + "\n# extra\n", "end of the document"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotEqual(t, good, tt.after)
			err := verifyDocument([]byte(before), []byte(tt.after), edits)
			require.ErrorIs(t, err, ErrVerification)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}
