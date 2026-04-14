package atmospro_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/skills/atmospro"
)

// regenerateSnapshots rewrites the golden files instead of diffing against them.
// Run with: go test ./pkg/ai/skills/atmospro/... -regenerate-snapshots.
var regenerateSnapshots = flag.Bool("regenerate-snapshots", false, "regenerate golden snapshot files")

const (
	templatesDir = "../../../../agent-skills/skills/atmos-pro/templates"
	fixtureDir   = "../../../../tests/fixtures/scenarios/atmos-pro-setup"
)

func loadFixture(t *testing.T) *atmospro.RenderData {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, "fixture.json"))
	require.NoError(t, err, "read fixture.json")

	var rd atmospro.RenderData
	require.NoError(t, json.Unmarshal(data, &rd), "unmarshal fixture.json")
	return &rd
}

func TestRenderData_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*atmospro.RenderData)
		wantErr error
	}{
		{
			name:    "ok with full fixture",
			mutate:  func(d *atmospro.RenderData) {},
			wantErr: nil,
		},
		{
			name:    "missing org",
			mutate:  func(d *atmospro.RenderData) { d.Org = "" },
			wantErr: atmospro.ErrOrgRequired,
		},
		{
			name:    "missing repo",
			mutate:  func(d *atmospro.RenderData) { d.Repo = "" },
			wantErr: atmospro.ErrRepoRequired,
		},
		{
			name:    "missing namespace",
			mutate:  func(d *atmospro.RenderData) { d.Namespace = "" },
			wantErr: atmospro.ErrNamespaceRequired,
		},
		{
			name:    "no accounts",
			mutate:  func(d *atmospro.RenderData) { d.Accounts = nil },
			wantErr: atmospro.ErrAccountsRequired,
		},
		{
			name: "account missing tenant",
			mutate: func(d *atmospro.RenderData) {
				d.Accounts = []atmospro.Account{{Stage: "iam", AccountID: "123"}}
			},
			wantErr: atmospro.ErrAccountFieldMissing,
		},
		{
			name: "multiple roots",
			mutate: func(d *atmospro.RenderData) {
				d.Accounts = []atmospro.Account{
					{Tenant: "gov", Stage: "root", AccountID: "1", IsRoot: true},
					{Tenant: "soc", Stage: "root", AccountID: "2", IsRoot: true},
				}
			},
			wantErr: atmospro.ErrMultipleRoots,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := loadFixture(t)
			tt.mutate(d)
			err := d.Validate()
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

// Render normalizes output to end with exactly one trailing newline so these
// tests append "\n" to their expected values. See normalizeTrailingNewline.

func TestRender_ScalarSubstitution(t *testing.T) {
	d := loadFixture(t)
	out, err := atmospro.Render("t.tmpl", "org=<<.org>> repo=<<.repo>> ns=<<.namespace>>", d)
	require.NoError(t, err)
	assert.Equal(t, "org=example repo=infra ns=ex\n", out)
}

func TestRender_LiteralBracesPassThrough(t *testing.T) {
	d := loadFixture(t)
	src := `component: "{{ .atmos_component }}"` + "\n" +
		`gh: "${{ vars.ATMOS_VERSION }}"`
	out, err := atmospro.Render("t.tmpl", src, d)
	require.NoError(t, err)
	// The {{ }} and ${{ }} must pass through untouched, plus the normalized
	// trailing newline.
	assert.Equal(t, src+"\n", out)
}

func TestRender_Range(t *testing.T) {
	d := loadFixture(t)
	src := "<<range .accounts>><<.tenant>>-<<.stage>>,<<end>>"
	out, err := atmospro.Render("t.tmpl", src, d)
	require.NoError(t, err)
	// Accounts in fixture order.
	assert.Equal(t, "gov-root,gov-iam,gov-dss,soc-clip,soc-siem,soc-wksn,\n", out)
}

func TestRender_RootPinnedToPlanRole(t *testing.T) {
	d := loadFixture(t)
	src := "<<range .accounts>><<if .is_root>>root=<<.account_id>><<end>><<end>>"
	out, err := atmospro.Render("t.tmpl", src, d)
	require.NoError(t, err)
	assert.Equal(t, "root=111111111111\n", out, "root account must be identifiable by is_root flag")
}

func TestRender_NormalizesTrailingNewlines(t *testing.T) {
	d := loadFixture(t)

	t.Run("no trailing newline becomes one", func(t *testing.T) {
		out, err := atmospro.Render("t.tmpl", "hello", d)
		require.NoError(t, err)
		assert.Equal(t, "hello\n", out)
	})

	t.Run("multiple trailing newlines collapse to one", func(t *testing.T) {
		out, err := atmospro.Render("t.tmpl", "hello\n\n\n\n", d)
		require.NoError(t, err)
		assert.Equal(t, "hello\n", out)
	})

	t.Run("empty output stays empty", func(t *testing.T) {
		out, err := atmospro.Render("t.tmpl", "", d)
		require.NoError(t, err)
		assert.Empty(t, out)
	})
}

func TestRender_TopLevelContextInRange(t *testing.T) {
	d := loadFixture(t)
	src := "<<range .accounts>><<$.namespace>>/<<.tenant>> <<end>>"
	out, err := atmospro.Render("t.tmpl", src, d)
	require.NoError(t, err)
	assert.Contains(t, out, "ex/gov")
	assert.Contains(t, out, "ex/soc")
}

// TestRenderAll_GoldenSnapshot renders every template against the fixture and diffs each
// result against its counterpart in tests/fixtures/scenarios/atmos-pro-setup/golden/.
// Run with -regenerate-snapshots to rewrite the golden files after intentional changes.
func TestRenderAll_GoldenSnapshot(t *testing.T) {
	d := loadFixture(t)

	fsys := os.DirFS(templatesDir)
	out, err := atmospro.RenderAll(fsys, d)
	require.NoError(t, err)
	require.NotEmpty(t, out, "expected at least one template to render")

	// Assert the full list of output paths, not just the count, so regressions that
	// drop a template fail loudly. At least the canonical first and last.
	keys := atmospro.SortedKeys(out)
	require.Contains(t, keys, ".github/workflows/atmos-pro.yaml")
	require.Contains(t, keys, "stacks/mixins/atmos-pro.yaml")

	goldenDir := filepath.Join(fixtureDir, "golden")

	if *regenerateSnapshots {
		// Wipe old golden files so renamed/removed templates don't leave orphans.
		require.NoError(t, os.RemoveAll(goldenDir))
		require.NoError(t, os.MkdirAll(goldenDir, 0o755))
		for _, k := range keys {
			path := filepath.Join(goldenDir, snapshotFilename(k))
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(out[k]), 0o644))
		}
		t.Logf("regenerated %d snapshot files", len(keys))
		return
	}

	for _, k := range keys {
		expectedPath := filepath.Join(goldenDir, snapshotFilename(k))
		expected, err := os.ReadFile(expectedPath)
		require.NoError(t, err, "missing golden file %s (rerun with -regenerate-snapshots)", expectedPath)
		assert.Equal(t, string(expected), out[k], "rendered output for %s drifted from golden", k)
	}
}

// snapshotFilename flattens an output path into a single file under golden/.
// We keep the full relative path but replace "/" with "__" so each snapshot is
// a single file that's easy to find and diff.
func snapshotFilename(outputPath string) string {
	return strings.ReplaceAll(outputPath, "/", "__") + ".golden"
}
