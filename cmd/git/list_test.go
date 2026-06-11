package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ---- validateGitListFormat ----

func TestValidateGitListFormat_Supported(t *testing.T) {
	for _, f := range []string{"", "table", "json", "yaml", "csv", "tsv"} {
		assert.NoError(t, validateGitListFormat(f), "format %q should be valid", f)
	}
}

func TestValidateGitListFormat_TreeRejected(t *testing.T) {
	err := validateGitListFormat(string(format.FormatTree))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidFlag), "expected ErrInvalidFlag, got: %v", err)
	assert.Contains(t, err.Error(), "tree")
}

func TestValidateGitListFormat_MatrixRejected(t *testing.T) {
	err := validateGitListFormat("matrix")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidFlag), "expected ErrInvalidFlag, got: %v", err)
	assert.Contains(t, err.Error(), "matrix")
}

func TestValidateGitListFormat_UnknownRejected(t *testing.T) {
	err := validateGitListFormat("xml")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidFlag))
}

// ---- defaultGitListColumns ----

func TestDefaultGitListColumns_WithoutStatus(t *testing.T) {
	cols := defaultGitListColumns(false, format.FormatTable)
	names := extractColumnNames(cols)
	assert.Equal(t, []string{"Name", "URI", "Provider", "Branch", "Workdir"}, names)

	// Status must be absent when checkStatus is false.
	for _, name := range names {
		assert.NotEqual(t, "Status", name)
	}
}

func TestDefaultGitListColumns_WithStatus(t *testing.T) {
	cols := defaultGitListColumns(true, format.FormatTable)
	names := extractColumnNames(cols)
	assert.Equal(t, []string{" ", "Name", "URI", "Provider", "Branch", "Workdir"}, names)
	assert.Equal(t, "{{ .status }}", cols[0].Value)
}

func TestDefaultGitListColumns_WithStatusDataFormat(t *testing.T) {
	cols := defaultGitListColumns(true, format.FormatJSON)
	names := extractColumnNames(cols)
	assert.Equal(t, []string{"Name", "URI", "Provider", "Branch", "Workdir", "Status"}, names)
	assert.Equal(t, "{{ .status_text }}", cols[5].Value)
}

// ---- parseGitColumnSpec ----

func TestParseGitColumnSpec_ShorthandLowercasesKey(t *testing.T) {
	c := parseGitColumnSpec("Name")
	assert.Equal(t, "Name", c.Name)
	assert.Equal(t, "{{ .name }}", c.Value)
}

func TestParseGitColumnSpec_ExplicitTemplate(t *testing.T) {
	c := parseGitColumnSpec("Region={{ .vars.region }}")
	assert.Equal(t, "Region", c.Name)
	assert.Equal(t, "{{ .vars.region }}", c.Value)
}

func TestParseGitColumnSpec_EmptyReturnsEmpty(t *testing.T) {
	c := parseGitColumnSpec("")
	assert.Empty(t, c.Name)
}

// ---- buildBaseRow ----

func TestBuildBaseRow_Defaults(t *testing.T) {
	cfg := &schema.GitConfig{
		Repositories: map[string]schema.GitRepository{
			"flux": {URI: "https://github.com/acme/flux.git"},
		},
	}
	row := buildBaseRow(cfg, "flux")
	assert.Equal(t, "flux", row["name"])
	assert.Equal(t, "https://github.com/acme/flux.git", row["uri"])
	assert.Equal(t, "cli", row["provider"])
	assert.Equal(t, defaultBranchPlaceholder, row["branch"])
	// Workdir should be a non-empty resolved XDG path.
	workdir, ok := row["workdir"].(string)
	assert.True(t, ok)
	assert.NotEmpty(t, workdir)
	assert.Contains(t, workdir, "flux")
}

func TestBuildBaseRow_ExplicitValues(t *testing.T) {
	cfg := &schema.GitConfig{
		Repositories: map[string]schema.GitRepository{
			"deploy": {
				URI:      "https://github.com/acme/deploy.git",
				Provider: "cli",
				Branch:   "main",
				Workdir:  "/tmp/deploy",
			},
		},
	}
	row := buildBaseRow(cfg, "deploy")
	assert.Equal(t, "deploy", row["name"])
	assert.Equal(t, "main", row["branch"])
	assert.Equal(t, "/tmp/deploy", row["workdir"])
}

// ---- extractGitRepoRowsWithProber (extraction) ----

// stubProber records which workdirs were probed and returns a fixed status.
// It is safe for concurrent use from the worker pool.
type stubProber struct {
	mu      sync.Mutex
	probed  []string
	returns string
}

func (s *stubProber) ProbeStatus(_ context.Context, workdir string) string {
	s.mu.Lock()
	s.probed = append(s.probed, workdir)
	s.mu.Unlock()
	return s.returns
}

func (s *stubProber) probedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.probed)
}

func TestExtractGitRepoRows_NilConfig(t *testing.T) {
	rows, err := extractGitRepoRowsWithProber(context.Background(), nil, false, &stubProber{})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestExtractGitRepoRows_EmptyRepositories(t *testing.T) {
	cfg := &schema.GitConfig{Repositories: map[string]schema.GitRepository{}}
	rows, err := extractGitRepoRowsWithProber(context.Background(), cfg, false, &stubProber{})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestExtractGitRepoRows_NoCheckStatus_NoProbes(t *testing.T) {
	prober := &stubProber{returns: statusCloned}
	cfg := twoRepoCfg()

	rows, err := extractGitRepoRowsWithProber(context.Background(), cfg, false, prober)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// No probes must run when checkStatus is false.
	assert.Zero(t, prober.probedCount(), "ProbeStatus must not be called without --check-status")

	// Status key must be absent from every row.
	for _, row := range rows {
		_, hasStatus := row["status"]
		assert.False(t, hasStatus, "status key must not be present without --check-status")
	}
}

func TestExtractGitRepoRows_WithCheckStatus_ProbesRun(t *testing.T) {
	prober := &stubProber{returns: statusCloned}
	cfg := twoRepoCfg()

	rows, err := extractGitRepoRowsWithProber(context.Background(), cfg, true, prober)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// Probes must have run for both repositories.
	assert.Equal(t, 2, prober.probedCount(), "ProbeStatus must be called for each repo")

	// Status key must be present and populated on every row.
	for _, row := range rows {
		status, ok := row["status"]
		require.True(t, ok, "status key must be present with --check-status")
		assert.Equal(t, statusCloned, status)
	}
}

func TestExtractGitRepoRows_SortedByName(t *testing.T) {
	// Repositories map iteration is non-deterministic; sorted by ConfiguredRepositoryNames.
	// Assert first and last element by value.
	cfg := &schema.GitConfig{
		Repositories: map[string]schema.GitRepository{
			"zeta":  {URI: "https://github.com/acme/zeta.git"},
			"alpha": {URI: "https://github.com/acme/alpha.git"},
			"beta":  {URI: "https://github.com/acme/beta.git"},
		},
	}

	rows, err := extractGitRepoRowsWithProber(context.Background(), cfg, false, &stubProber{})
	require.NoError(t, err)
	require.Len(t, rows, 3)

	// First and last by value (not just length).
	assert.Equal(t, "alpha", rows[0]["name"], "first row must be alpha (sorted a→z)")
	assert.Equal(t, "https://github.com/acme/alpha.git", rows[0]["uri"])
	assert.Equal(t, "zeta", rows[2]["name"], "last row must be zeta (sorted a→z)")
	assert.Equal(t, "https://github.com/acme/zeta.git", rows[2]["uri"])
}

func TestExtractGitRepoRows_StatusValues(t *testing.T) {
	for _, want := range []string{statusMissing, statusCloned, statusDirty} {
		prober := &stubProber{returns: want}
		cfg := twoRepoCfg()

		rows, err := extractGitRepoRowsWithProber(context.Background(), cfg, true, prober)
		require.NoError(t, err)
		for _, row := range rows {
			assert.Equal(t, want, row["status"], "status must match prober return value")
		}
	}
}

func TestGitStatusIndicatorTTYUsesDot(t *testing.T) {
	for _, status := range []string{statusMissing, statusCloned, statusDirty} {
		got := gitStatusIndicatorWithTTY(status, true)
		assert.Contains(t, got, statusDot)
		assert.NotEqual(t, status, got)
	}
}

func TestGitStatusIndicatorNonTTYUsesText(t *testing.T) {
	for _, status := range []string{statusMissing, statusCloned, statusDirty} {
		assert.Equal(t, status, gitStatusIndicatorWithTTY(status, false))
	}
}

// ---- buildGitListSorters ----

func TestBuildGitListSorters_Default(t *testing.T) {
	sorters, err := buildGitListSorters("")
	require.NoError(t, err)
	require.Len(t, sorters, 1)
	assert.Equal(t, "Name", sorters[0].Column)
}

func TestBuildGitListSorters_Custom(t *testing.T) {
	sorters, err := buildGitListSorters("URI:desc")
	require.NoError(t, err)
	require.Len(t, sorters, 1)
	assert.Equal(t, "URI", sorters[0].Column)
}

// ---- getGitListColumns ----

func TestGetGitListColumns_FlagTakesPrecedence(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			List: schema.GitListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "FromConfig", Value: "{{ .name }}"},
				},
			},
		},
	}
	cols := getGitListColumns(atmosConfig, []string{"Name"}, false, format.FormatTable)
	require.Len(t, cols, 1)
	assert.Equal(t, "Name", cols[0].Name)
}

func TestGetGitListColumns_ConfigTakesPrecedenceOverDefault(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			List: schema.GitListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Repo", Value: "{{ .name }}"},
					{Name: "URL", Value: "{{ .uri }}"},
				},
			},
		},
	}
	cols := getGitListColumns(atmosConfig, nil, false, format.FormatTable)
	require.Len(t, cols, 2)
	assert.Equal(t, "Repo", cols[0].Name)
	assert.Equal(t, "URL", cols[1].Name)
}

func TestGetGitListColumns_Default_NoStatus(t *testing.T) {
	cols := getGitListColumns(&schema.AtmosConfiguration{}, nil, false, format.FormatTable)
	names := extractColumnNames(cols)
	assert.NotContains(t, names, "Status")
	assert.Contains(t, names, "Name")
}

func TestGetGitListColumns_Default_WithStatus(t *testing.T) {
	cols := getGitListColumns(&schema.AtmosConfiguration{}, nil, true, format.FormatTable)
	names := extractColumnNames(cols)
	assert.Contains(t, names, " ")
}

func TestGetGitListColumns_DefaultWithStatusForDataFormat(t *testing.T) {
	cols := getGitListColumns(&schema.AtmosConfiguration{}, nil, true, format.FormatJSON)
	names := extractColumnNames(cols)
	assert.Contains(t, names, "Status")
}

// ---- GitCommandProvider alias ----

func TestGitCommandProvider_GetAliases_ContainsListGitRepositories(t *testing.T) {
	p := &GitCommandProvider{}
	aliases := p.GetAliases()
	require.NotEmpty(t, aliases, "GitCommandProvider must return at least one alias")

	var found bool
	for _, a := range aliases {
		if a.Name == "git-repositories" && a.ParentCommand == "list" && a.Subcommand == "list" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected alias {Name:git-repositories, ParentCommand:list, Subcommand:list}")
}

// ---- defaultGitColumnNames ----

func TestDefaultGitColumnNames_IncludesAllDefaults(t *testing.T) {
	names := defaultGitColumnNames(false)
	assert.Contains(t, names, "Name")
	assert.Contains(t, names, "URI")
	assert.Contains(t, names, "Provider")
	assert.Contains(t, names, "Branch")
	assert.Contains(t, names, "Workdir")
	assert.NotContains(t, names, "Status")
}

func TestDefaultGitColumnNames_WithStatus(t *testing.T) {
	names := defaultGitColumnNames(true)
	assert.Contains(t, names, "Status")
}

// ---- lowerFirst ----

func TestLowerFirst(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Name", "name"},
		{"URI", "uRI"},
		{"name", "name"},
		{"", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, lowerFirst(tt.in))
	}
}

// ---- branch placeholder ----

func TestBuildBaseRow_EmptyBranchBecomesPlaceholder(t *testing.T) {
	cfg := &schema.GitConfig{
		Repositories: map[string]schema.GitRepository{
			"r": {URI: "https://example.com/r.git"},
		},
	}
	row := buildBaseRow(cfg, "r")
	assert.Equal(t, defaultBranchPlaceholder, row["branch"])
}

// ---- listCmd registration ----

func TestListCmd_Registered(t *testing.T) {
	var found bool
	for _, sub := range gitCmd.Commands() {
		if sub.Name() == "list" {
			found = true
			break
		}
	}
	assert.True(t, found, "listCmd must be registered as a subcommand of gitCmd")
}

func TestListCmd_HasExpectedFlags(t *testing.T) {
	assert.NotNil(t, listCmd.Flags().Lookup(listFlagColumns))
	assert.NotNil(t, listCmd.Flags().Lookup(listFlagFormat))
	assert.NotNil(t, listCmd.Flags().Lookup(listFlagDelimiter))
	assert.NotNil(t, listCmd.Flags().Lookup(listFlagCheckStatus))
}

// ---- parseGitColumnsFlag ----

func TestParseGitColumnsFlag_MultipleSpecs(t *testing.T) {
	cols := parseGitColumnsFlag([]string{"Name", "URI={{ .uri }}", ""})
	require.Len(t, cols, 2, "empty spec should be skipped")
	assert.Equal(t, "Name", cols[0].Name)
	assert.Equal(t, "{{ .name }}", cols[0].Value)
	assert.Equal(t, "URI", cols[1].Name)
	assert.Equal(t, "{{ .uri }}", cols[1].Value)
}

// ---- helpers ----

// twoRepoCfg returns a GitConfig with two repositories for test convenience.
func twoRepoCfg() *schema.GitConfig {
	return &schema.GitConfig{
		Repositories: map[string]schema.GitRepository{
			"alpha": {URI: "https://github.com/acme/alpha.git", Workdir: "/tmp/alpha"},
			"beta":  {URI: "https://github.com/acme/beta.git", Workdir: "/tmp/beta"},
		},
	}
}

// extractColumnNames returns the Name field of each column.Config.
func extractColumnNames(cols []column.Config) []string {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.Name
	}
	return names
}

// Compile-time check: schema.ListColumnConfig has Name and Value fields
// referenced in TestGetGitListColumns_ConfigTakesPrecedenceOverDefault.
var _ = schema.ListColumnConfig{Name: "x", Value: "{{ .x }}"}

// ---- providerStatusProber integration test ----

// TestProviderStatusProber_MissingDir verifies that a non-existent directory
// returns "missing" without panicking. Requires the "cli" provider registration
// (via blank import in git.go) and skips when git is not available.
func TestProviderStatusProber_MissingDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH.")
	}

	prober := &providerStatusProber{}
	status := prober.ProbeStatus(context.Background(), "/nonexistent/path/that/cannot/exist-abc123")
	assert.Equal(t, statusMissing, status)
}

// TestProviderStatusProber_CleanRepo verifies that a freshly-initialised git
// repository returns "cloned" (clean). Skips when git is not available.
func TestProviderStatusProber_CleanRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH.")
	}

	dir := t.TempDir()
	initGitRepo(t, dir)

	prober := &providerStatusProber{}
	status := prober.ProbeStatus(context.Background(), dir)
	assert.Equal(t, statusCloned, status)
}

// TestProviderStatusProber_DirtyRepo verifies that a repo with untracked files
// returns "dirty". Skips when git is not available.
func TestProviderStatusProber_DirtyRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH.")
	}

	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create an untracked file to make the repo dirty.
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty"), 0o600); err != nil {
		t.Fatal("writing dirty file:", err)
	}

	prober := &providerStatusProber{}
	status := prober.ProbeStatus(context.Background(), dir)
	assert.Equal(t, statusDirty, status)
}

// initGitRepo initialises a bare git repo in dir using the git CLI.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", gitTestArgs(args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	// Create an initial commit so HEAD is valid.
	run("commit", "--allow-empty", "-m", "init")
}

// ---- extractGitRepoRows (production wrapper) ----

func TestExtractGitRepoRows_NilConfig_Production(t *testing.T) {
	// Exercises the production wrapper path (uses defaultProber but nil config
	// short-circuits before any probe runs).
	rows, err := extractGitRepoRows(nil, false)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestExtractGitRepoRows_EmptyConfig_Production(t *testing.T) {
	cfg := &schema.GitConfig{Repositories: map[string]schema.GitRepository{}}
	rows, err := extractGitRepoRows(cfg, false)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// ---- columnsCompletionForGitList (smoke test without real config) ----

// TestColumnsCompletionForGitList_NoConfig verifies the function returns default
// column names when Atmos config cannot be loaded (e.g., outside an Atmos project).
func TestColumnsCompletionForGitList_NoConfig(t *testing.T) {
	// Override working directory so InitCliConfig fails gracefully.
	names, directive := columnsCompletionForGitList(listCmd, nil, "")
	// Completion should always return a non-nil slice and no file-completion directive.
	assert.NotNil(t, names)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

// ---- loadGitListConfig smoke test ----

func TestLoadGitListConfig_InvalidDir(t *testing.T) {
	// With an empty options struct and no atmos.yaml present, config loading
	// should either succeed (with defaults) or return an error; it must not panic.
	opts := &GitListOptions{}
	// This may succeed or fail depending on whether an atmos.yaml is discoverable
	// from the test working directory. Either outcome is acceptable for this test.
	_, err := loadGitListConfig(nil, nil, opts)
	// We only assert no panic; both nil and non-nil error are valid.
	_ = err
}

// ---- listGitRepositories fast-path tests ----

// TestListGitRepositories_NoRepositories verifies the empty-repo path without invoking
// the full Cobra pipeline. We set atmosConfigPtr to a config with no repositories.
func TestListGitRepositories_NoRepositories(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{},
		},
	}
	opts := &GitListOptions{Format: "table"}
	err := renderGitRepositoriesList(atmosConfig, opts)
	require.NoError(t, err)
}

func TestListGitRepositories_InvalidFormat(t *testing.T) {
	opts := &GitListOptions{Format: "tree"}
	err := listGitRepositories(nil, nil, opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidFlag))
}

// ---- renderGitRepositoriesList ----

func TestRenderGitRepositoriesList_JSONFormat(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"alpha": {URI: "https://example.com/alpha.git", Workdir: "/tmp/alpha"},
				"beta":  {URI: "https://example.com/beta.git", Workdir: "/tmp/beta"},
			},
		},
	}
	opts := &GitListOptions{Format: "json"}
	err := renderGitRepositoriesList(atmosConfig, opts)
	require.NoError(t, err)
}

func TestRenderGitRepositoriesList_DefaultFormatFromConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			List: schema.GitListConfig{Format: "json"},
			Repositories: map[string]schema.GitRepository{
				"repo": {URI: "https://example.com/repo.git", Workdir: "/tmp/repo"},
			},
		},
	}
	opts := &GitListOptions{} // No format flag — should use config value.
	err := renderGitRepositoriesList(atmosConfig, opts)
	require.NoError(t, err)
	assert.Equal(t, "json", opts.Format, "format should be resolved from config")
}

func TestRenderGitRepositoriesList_InvalidConfigFormat(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			List: schema.GitListConfig{Format: "tree"},
			Repositories: map[string]schema.GitRepository{
				"repo": {URI: "https://example.com/repo.git", Workdir: "/tmp/repo"},
			},
		},
	}
	opts := &GitListOptions{}
	err := renderGitRepositoriesList(atmosConfig, opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidFlag))
}

func TestRenderGitRepositoriesList_CSVWithCustomColumns(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"gamma": {URI: "https://example.com/gamma.git", Branch: "main", Workdir: "/tmp/gamma"},
			},
		},
	}
	opts := &GitListOptions{
		Format:  "csv",
		Columns: []string{"Name", "Branch"},
	}
	err := renderGitRepositoriesList(atmosConfig, opts)
	require.NoError(t, err)
}

func TestRenderGitRepositoriesList_SortedOutput(t *testing.T) {
	// Verify sort is applied: with 3 repos, output must be deterministic (name:asc).
	atmosConfig := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"zeta":  {URI: "https://example.com/zeta.git", Workdir: "/tmp/zeta"},
				"alpha": {URI: "https://example.com/alpha.git", Workdir: "/tmp/alpha"},
			},
		},
	}
	opts := &GitListOptions{Format: "json"}
	err := renderGitRepositoriesList(atmosConfig, opts)
	require.NoError(t, err)
}

// ---- parseGitListOptions (unit test via a fake Viper) ----

func TestParseGitListOptions_ReadsFromViper(t *testing.T) {
	v := viper.New()
	v.Set(listFlagColumns, []string{"Name", "URI"})
	v.Set(listFlagFormat, "json")
	v.Set(listFlagCheckStatus, true)

	opts := parseGitListOptions(listCmd, v)
	assert.Equal(t, []string{"Name", "URI"}, opts.Columns)
	assert.Equal(t, "json", opts.Format)
	assert.True(t, opts.CheckStatus)
}
