package track

import (
	"bytes"
	"context"
	"errors"
	stdio "io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// tableDemoResolver serves the "track-table-demo" datasource with a single
// candidate, used to exercise update/lock/status/diff's output-format
// dispatch without requiring network access.
type tableDemoResolver struct{}

func (tableDemoResolver) Names() []string { return []string{"track-table-demo"} }

func (tableDemoResolver) Versions(_ context.Context, _ *resolver.Request) ([]resolver.Candidate, error) {
	return []resolver.Candidate{{Version: "v1.0.0"}}, nil
}

func (tableDemoResolver) Pin(_ context.Context, _ *resolver.Request, version string) (string, error) {
	return "pinned-" + version, nil
}

func init() {
	resolver.Register(tableDemoResolver{})
}

// tableDemoConfig builds a fresh config with one unlocked entry served by
// tableDemoResolver, suitable for exercising update/lock/status/diff.
func tableDemoConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	return &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		Version: schema.Version{
			Track: "prod",
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Dependencies: map[string]schema.VersionEntry{
						"widget": {
							Datasource: "track-table-demo",
							Package:    "acme/widget",
							Desired:    "latest",
						},
					},
				},
			},
		},
	}
}

type trackTestStreams struct {
	stdin  stdio.Reader
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func (s *trackTestStreams) Input() stdio.Reader     { return s.stdin }
func (s *trackTestStreams) Output() stdio.Writer    { return s.stdout }
func (s *trackTestStreams) Error() stdio.Writer     { return s.stderr }
func (s *trackTestStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *trackTestStreams) RawError() stdio.Writer  { return s.stderr }

func setupTrackOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	stdout := &bytes.Buffer{}
	streams := &trackTestStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
	return stdout
}

func runTrackCommand(t *testing.T, cmd *cobra.Command, args ...string) error {
	t.Helper()
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd.Execute()
}

func newTrackCommand(source *cobra.Command, opts ...flags.Option) *cobra.Command {
	cmd := &cobra.Command{
		Use:   source.Use,
		Args:  source.Args,
		RunE:  source.RunE,
		Short: source.Short,
	}
	flags.NewStandardParser(opts...).RegisterFlags(cmd)
	return cmd
}

func newRenderCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  trackRenderCmd.Use,
		Args: trackRenderCmd.Args,
		RunE: trackRenderCmd.RunE,
	}
	flags.NewStandardParser(
		flags.WithStringFlag("track", "", "", "Version track to operate on"),
		flags.WithStringFlag("file", "", "", "Template source file to render"),
		flags.WithStringFlag("output", "", "", "Rendered output file"),
		flags.WithBoolFlag("check", "", false, "Check rendered output"),
	).RegisterFlags(cmd)
	return cmd
}

func trackConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	dir := t.TempDir()
	cfg := &schema.AtmosConfiguration{
		BasePath: dir,
		Version: schema.Version{
			Track:    "prod",
			LockFile: "versions.lock.yaml",
			Tracks: map[string]schema.VersionTrack{
				"dev": {},
				"prod": {
					Dependencies: map[string]schema.VersionEntry{
						"opentofu": {
							Ecosystem: "toolchain",
							Package:   "opentofu",
							Desired:   "1.10.0",
							Group:     "tools",
						},
					},
				},
			},
		},
	}
	lock := &manager.LockFile{
		Tracks: map[string]map[string]manager.LockEntry{
			"prod": {
				"opentofu": {Version: "1.10.0"},
			},
		},
	}
	if err := manager.SaveLock(cfg, lock); err != nil {
		t.Fatalf("SaveLock: %v", err)
	}
	return cfg
}

func editableTrackConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	content := `base_path: "."
version:
  track: prod
  lock_file: versions.lock.yaml
  tracks:
    prod:
      dependencies:
        opentofu:
          ecosystem: toolchain
          package: opentofu
          desired: "1.10.0"
`
	if err := os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write atmos.yaml: %v", err)
	}
	return &schema.AtmosConfiguration{
		BasePath: dir,
		Version: schema.Version{
			Track:    "prod",
			LockFile: "versions.lock.yaml",
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Dependencies: map[string]schema.VersionEntry{
						"opentofu": {Ecosystem: "toolchain", Package: "opentofu", Desired: "1.10.0"},
					},
				},
			},
		},
	}
}

func setTrackConfigForTest(t *testing.T, cfg *schema.AtmosConfiguration) {
	t.Helper()
	previous := atmosConfig
	SetAtmosConfig(cfg)
	t.Cleanup(func() {
		SetAtmosConfig(previous)
	})
}

func TestTrackCommandAccessorsAndFlagHelpers(t *testing.T) {
	cfg := trackConfig(t)
	SetAtmosConfig(cfg)
	if atmosConfig != cfg {
		t.Fatal("SetAtmosConfig did not update package config")
	}
	if GetTrackCommand() != trackCmd {
		t.Fatal("GetTrackCommand did not return trackCmd")
	}

	cmd := newTrackCommand(trackShowCmd, trackParserOptions(groupFlagOption())...)
	if cmd.Flags().Lookup("format") == nil || cmd.Flags().Lookup("track") == nil || cmd.Flags().Lookup("group") == nil {
		t.Fatalf("expected format, track, and group flags to be registered")
	}
	if got := trackFromArgs(cmd, []string{"arg-track"}); got != "arg-track" {
		t.Fatalf("trackFromArgs positional = %q", got)
	}
	if err := cmd.Flags().Set("track", "flag-track"); err != nil {
		t.Fatalf("set track flag: %v", err)
	}
	if got := trackFromArgs(cmd, nil); got != "flag-track" {
		t.Fatalf("trackFromArgs flag = %q", got)
	}

	if err := cmd.Flags().Set("format", "toml"); err != nil {
		t.Fatalf("set format flag: %v", err)
	}
	if err := writeFormatted(cmd, map[string]string{"ok": "true"}); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("writeFormatted error = %v, want %v", err, ErrUnsupportedFormat)
	}
}

func TestTrackListShowAndGetCommands(t *testing.T) {
	cfg := trackConfig(t)
	setTrackConfigForTest(t, cfg)
	stdout := setupTrackOutput(t)

	listCmd := newTrackCommand(trackListCmd, trackListParserOptions()...)
	if err := runTrackCommand(t, listCmd, "--format", "json"); err != nil {
		t.Fatalf("list command returned error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, `"dev"`) || !strings.Contains(output, `"prod"`) || !strings.Contains(output, `"opentofu": "1.10.0"`) {
		t.Fatalf("list output = %q", output)
	}

	stdout.Reset()
	listCmd = newTrackCommand(trackListCmd, trackListParserOptions()...)
	if err := runTrackCommand(t, listCmd, "--format", "json", "--show", "desired"); err != nil {
		t.Fatalf("list --show desired returned error: %v", err)
	}
	output = stdout.String()
	if !strings.Contains(output, `"opentofu": "1.10.0"`) {
		t.Fatalf("list --show desired output = %q", output)
	}

	stdout.Reset()
	listCmd = newTrackCommand(trackListCmd, trackListParserOptions()...)
	if err := runTrackCommand(t, listCmd, "--show", "bogus"); !errors.Is(err, manager.ErrUnsupportedVersionShow) {
		t.Fatalf("list --show bogus error = %v, want %v", err, manager.ErrUnsupportedVersionShow)
	}

	stdout.Reset()
	showCmd := newTrackCommand(trackShowCmd, trackParserOptions()...)
	if err := runTrackCommand(t, showCmd, "--format", "json", "prod"); err != nil {
		t.Fatalf("show command returned error: %v", err)
	}
	output = stdout.String()
	if !strings.Contains(output, `"opentofu"`) || !strings.Contains(output, `"desired": "1.10.0"`) {
		t.Fatalf("show output = %q", output)
	}

	stdout.Reset()
	getCmd := newTrackCommand(trackGetCmd, trackGetParserOptions()...)
	if err := runTrackCommand(t, getCmd, "opentofu"); err != nil {
		t.Fatalf("get command returned error: %v", err)
	}
	output = stdout.String()
	if strings.TrimSpace(output) != "1.10.0" {
		t.Fatalf("get output = %q, want just the locked version", output)
	}

	stdout.Reset()
	getCmd = newTrackCommand(trackGetCmd, trackGetParserOptions()...)
	if err := runTrackCommand(t, getCmd, "opentofu", "--show", "ecosystem"); err != nil {
		t.Fatalf("get --show ecosystem returned error: %v", err)
	}
	output = stdout.String()
	if strings.TrimSpace(output) != "toolchain" {
		t.Fatalf("get --show ecosystem output = %q", output)
	}

	stdout.Reset()
	getCmd = newTrackCommand(trackGetCmd, trackGetParserOptions()...)
	if err := runTrackCommand(t, getCmd, "opentofu", "--show", "bogus"); !errors.Is(err, manager.ErrUnsupportedEntryField) {
		t.Fatalf("get --show bogus error = %v, want %v", err, manager.ErrUnsupportedEntryField)
	}

	if err := runTrackCommand(t, newTrackCommand(trackGetCmd, trackGetParserOptions()...), "missing"); !errors.Is(err, manager.ErrEntryNotFound) {
		t.Fatalf("missing get error = %v, want %v", err, manager.ErrEntryNotFound)
	}
}

func TestTrackAddSetRemoveCommandsEditConfig(t *testing.T) {
	cfg := editableTrackConfig(t)
	setTrackConfigForTest(t, cfg)
	stdout := setupTrackOutput(t)

	addCmd := newTrackCommand(trackAddCmd, trackParserOptions(
		flags.WithStringFlag("package", "", "", "Package coordinate"),
		flags.WithStringFlag("ecosystem", "", "", "Ecosystem"),
		flags.WithStringFlag("datasource", "", "", "Datasource"),
		flags.WithStringFlag("provider", "", "", "Provider"),
		flags.WithStringFlag("desired", "", "latest", "Desired version"),
		flags.WithStringFlag("group", "", "", "Version group name"),
		flags.WithStringFlag("pin", "", "", "Pin policy"),
		flags.WithStringSliceFlag("include", "", nil, "Include patterns"),
		flags.WithStringSliceFlag("exclude", "", nil, "Exclude patterns"),
		flags.WithBoolFlag("prerelease", "", false, "Allow prerelease candidates"),
	)...)
	if err := runTrackCommand(t, addCmd, "checkout", "--package", "actions/checkout", "--desired", "v6", "--pin", "sha", "--include", "v6.*", "--exclude", "v6.0.0", "--prerelease"); err != nil {
		t.Fatalf("add command returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "name: checkout") {
		t.Fatalf("add output = %q", stdout.String())
	}

	configPath := filepath.Join(cfg.BasePath, "atmos.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read atmos.yaml: %v", err)
	}
	for _, expected := range []string{"checkout:", "pin: sha", "include:", "v6.*", "exclude:", "v6.0.0", "prerelease: true"} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("expected config to contain %q after add:\n%s", expected, content)
		}
	}

	if !strings.Contains(string(content), "checkout:") || !strings.Contains(string(content), "pin: sha") {
		t.Fatalf("config after add =\n%s", content)
	}

	setCmd := newTrackCommand(trackSetCmd, trackParserOptions(
		flags.WithStringFlag("desired", "", "", "Desired version"),
		flags.WithStringFlag("package", "", "", "Package coordinate"),
		flags.WithStringFlag("provider", "", "", "Provider name"),
		flags.WithStringFlag("group", "", "", "Version group name"),
		flags.WithStringFlag("pin", "", "", "Pin policy"),
		flags.WithStringSliceFlag("include", "", nil, "Include patterns"),
		flags.WithStringSliceFlag("exclude", "", nil, "Exclude patterns"),
		flags.WithBoolFlag("prerelease", "", false, "Allow prerelease candidates"),
	)...)
	if err := runTrackCommand(t, setCmd, "checkout", "--desired", "v7", "--group", "ci", "--include", "v7.*", "--exclude", "v7.0.0", "--prerelease=false"); err != nil {
		t.Fatalf("set command returned error: %v", err)
	}
	content, _ = os.ReadFile(configPath)
	for _, expected := range []string{"desired: v7", "group: ci", "v7.*", "v7.0.0", "prerelease: false"} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("expected config to contain %q after set:\n%s", expected, content)
		}
	}
	if !strings.Contains(string(content), "desired: v7") || !strings.Contains(string(content), "group: ci") {
		t.Fatalf("config after set =\n%s", content)
	}

	removeCmd := newTrackCommand(trackRemoveCmd, trackParserOptions()...)
	if err := runTrackCommand(t, removeCmd, "checkout"); err != nil {
		t.Fatalf("remove command returned error: %v", err)
	}
	content, _ = os.ReadFile(configPath)
	if strings.Contains(string(content), "checkout:") {
		t.Fatalf("config after remove =\n%s", content)
	}
}

func TestTrackRenderCommandCheckMode(t *testing.T) {
	cfg := trackConfig(t)
	setTrackConfigForTest(t, cfg)
	setupTrackOutput(t)

	source := filepath.Join(cfg.BasePath, "versions.txt.tmpl")
	output := filepath.Join(cfg.BasePath, "versions.txt")
	if err := os.WriteFile(source, []byte("static content\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(output, []byte("static content\n"), 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}
	if err := runTrackCommand(t, newRenderCommand(), "--file", source, "--output", output, "--check"); err != nil {
		t.Fatalf("render check returned error: %v", err)
	}

	if err := os.WriteFile(output, []byte("drifted\n"), 0o644); err != nil {
		t.Fatalf("write drifted output: %v", err)
	}
	if err := runTrackCommand(t, newRenderCommand(), "--file", source, "--output", output, "--check"); !errors.Is(err, ErrRenderDrift) {
		t.Fatalf("render drift error = %v, want %v", err, ErrRenderDrift)
	}
	if err := runTrackCommand(t, newRenderCommand()); !errors.Is(err, ErrRenderFileRequired) {
		t.Fatalf("render missing file error = %v, want %v", err, ErrRenderFileRequired)
	}
}

func newUpdateCommand() *cobra.Command {
	return newTrackCommand(trackUpdateCmd, trackTableParserOptions(
		groupFlagOption(),
		flags.WithStringSliceFlag("only", "", nil, "Limit the update to the named entries (repeatable)"),
	)...)
}

func newLockCommand() *cobra.Command {
	return newTrackCommand(trackLockCmd, trackTableParserOptions(groupFlagOption())...)
}

func newStatusCommand() *cobra.Command {
	return newTrackCommand(trackStatusCmd, trackTableParserOptions(groupFlagOption())...)
}

func newDiffCommand() *cobra.Command {
	return newTrackCommand(trackDiffCmd, trackTableParserOptions(groupFlagOption())...)
}

// TestTrackUpdateLockStatusDiffDefaultToTable verifies that update/lock/status/diff
// default to a human-readable table (not raw YAML), while --format=yaml and
// --format=json continue to return the original full-fidelity struct.
func TestTrackUpdateLockStatusDiffDefaultToTable(t *testing.T) {
	commands := []struct {
		name         string
		new          func() *cobra.Command
		yamlFidelity string // A string only present in the full-fidelity YAML/JSON dump.
		jsonFidelity string
	}{
		{"update", newUpdateCommand, "name: widget", `"name": "widget"`},
		// LockFile keys entries by name (map[name]LockEntry) rather than
		// carrying a Name field, so full fidelity shows up as the map key.
		{"lock", newLockCommand, "widget:", `"widget": {`},
		{"status", newStatusCommand, "name: widget", `"name": "widget"`},
		{"diff", newDiffCommand, "name: widget", `"name": "widget"`},
	}

	// All four commands share the "Track, Name, ..." column order (see
	// updateColumns/lockColumns/statusColumns), so the Name field always
	// lands at tab-split index 1.
	const nameFieldIndex = 1

	for _, tc := range commands {
		t.Run(tc.name+"/default is not YAML", func(t *testing.T) {
			setTrackConfigForTest(t, tableDemoConfig(t))
			stdout := setupTrackOutput(t)

			if err := runTrackCommand(t, tc.new()); err != nil {
				t.Fatalf("%s command returned error: %v", tc.name, err)
			}
			output := stdout.String()
			if strings.Contains(output, "track: prod") || strings.Contains(output, "results:") || strings.Contains(output, "entries:") {
				t.Fatalf("%s default output looks like YAML, want a table: %q", tc.name, output)
			}
			if strings.Contains(output, `"name"`) || strings.Contains(output, "{") {
				t.Fatalf("%s default output looks like JSON, want a table: %q", tc.name, output)
			}
			// Outside a TTY the shared renderer prints the table as a
			// headerless tab-delimited row (pkg/list/renderer.formatPlainList);
			// a JSON/YAML regression would never produce a raw tab.
			line := strings.TrimRight(output, "\n")
			fields := strings.Split(line, "\t")
			if len(fields) <= nameFieldIndex || fields[nameFieldIndex] != "widget" {
				t.Fatalf("%s default output = %q, want a tab-delimited row with Name (field %d) = %q", tc.name, output, nameFieldIndex, "widget")
			}
		})

		t.Run(tc.name+"/format=tsv", func(t *testing.T) {
			setTrackConfigForTest(t, tableDemoConfig(t))
			stdout := setupTrackOutput(t)

			if err := runTrackCommand(t, tc.new(), "--format", "tsv"); err != nil {
				t.Fatalf("%s --format=tsv returned error: %v", tc.name, err)
			}
			output := stdout.String()
			lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
			if len(lines) < 2 {
				t.Fatalf("%s --format=tsv output = %q, want a header line and at least one row", tc.name, output)
			}
			if !strings.Contains(lines[0], "Name") {
				t.Fatalf("%s --format=tsv header = %q, want it to contain the Name column", tc.name, lines[0])
			}
			fields := strings.Split(lines[1], "\t")
			if len(fields) <= nameFieldIndex || fields[nameFieldIndex] != "widget" {
				t.Fatalf("%s --format=tsv row = %q, want Name (field %d) = %q", tc.name, lines[1], nameFieldIndex, "widget")
			}
		})

		t.Run(tc.name+"/format=yaml keeps full fidelity", func(t *testing.T) {
			setTrackConfigForTest(t, tableDemoConfig(t))
			stdout := setupTrackOutput(t)

			cmd := tc.new()
			if err := runTrackCommand(t, cmd, "--format", "yaml"); err != nil {
				t.Fatalf("%s --format=yaml returned error: %v", tc.name, err)
			}
			output := stdout.String()
			if !strings.Contains(output, tc.yamlFidelity) {
				t.Fatalf("%s --format=yaml output = %q, want full-fidelity YAML containing %q", tc.name, output, tc.yamlFidelity)
			}
		})

		t.Run(tc.name+"/format=json keeps full fidelity", func(t *testing.T) {
			setTrackConfigForTest(t, tableDemoConfig(t))
			stdout := setupTrackOutput(t)

			cmd := tc.new()
			if err := runTrackCommand(t, cmd, "--format", "json"); err != nil {
				t.Fatalf("%s --format=json returned error: %v", tc.name, err)
			}
			output := stdout.String()
			if !strings.Contains(output, tc.jsonFidelity) {
				t.Fatalf("%s --format=json output = %q, want full-fidelity JSON containing %q", tc.name, output, tc.jsonFidelity)
			}
		})
	}

	t.Run("status/format=csv smoke test", func(t *testing.T) {
		setTrackConfigForTest(t, tableDemoConfig(t))
		stdout := setupTrackOutput(t)

		if err := runTrackCommand(t, newStatusCommand(), "--format", "csv"); err != nil {
			t.Fatalf("status --format=csv returned error: %v", err)
		}
		output := stdout.String()
		if !strings.Contains(output, "Name") || !strings.Contains(output, ",") || !strings.Contains(output, "widget") {
			t.Fatalf("status --format=csv output = %q, want a CSV header and row", output)
		}
	})

	t.Run("format=toml is rejected", func(t *testing.T) {
		for _, tc := range commands {
			setTrackConfigForTest(t, tableDemoConfig(t))
			setupTrackOutput(t)

			if err := runTrackCommand(t, tc.new(), "--format", "toml"); !errors.Is(err, ErrUnsupportedFormat) {
				t.Fatalf("%s --format=toml error = %v, want %v", tc.name, err, ErrUnsupportedFormat)
			}
		}
	})
}

func TestWriteFormattedFallbacksAndErrors(t *testing.T) {
	stdout := setupTrackOutput(t)
	cmd := newTrackCommand(trackShowCmd, structuredFormatParserOptions()...)

	// atmosConfig == nil falls back to the plain data.WriteYAML/WriteJSON writers.
	setTrackConfigForTest(t, nil)

	if err := cmd.Flags().Set("format", "yaml"); err != nil {
		t.Fatalf("set format: %v", err)
	}
	if err := writeFormatted(cmd, map[string]string{"ok": "true"}); err != nil {
		t.Fatalf("writeFormatted yaml (nil config) error: %v", err)
	}
	if !strings.Contains(stdout.String(), "ok:") {
		t.Fatalf("yaml fallback output = %q", stdout.String())
	}

	stdout.Reset()
	if err := cmd.Flags().Set("format", "json"); err != nil {
		t.Fatalf("set format: %v", err)
	}
	if err := writeFormatted(cmd, map[string]string{"ok": "true"}); err != nil {
		t.Fatalf("writeFormatted json (nil config) error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"ok"`) {
		t.Fatalf("json fallback output = %q", stdout.String())
	}

	// With atmosConfig set, a value that fails to marshal surfaces the
	// GetHighlightedYAML/GetHighlightedJSON error instead of writing output.
	setTrackConfigForTest(t, trackConfig(t))

	if err := cmd.Flags().Set("format", "yaml"); err != nil {
		t.Fatalf("set format: %v", err)
	}
	if err := writeFormatted(cmd, failingYAML{}); err == nil {
		t.Fatal("expected writeFormatted yaml error for a failing MarshalYAML")
	}

	if err := cmd.Flags().Set("format", "json"); err != nil {
		t.Fatalf("set format: %v", err)
	}
	if err := writeFormatted(cmd, failingJSON{}); err == nil {
		t.Fatal("expected writeFormatted json error for a failing MarshalJSON")
	}
}

// failingYAML always fails to marshal, forcing writeFormatted's yaml error path.
type failingYAML struct{}

func (failingYAML) MarshalYAML() (any, error) {
	return nil, errors.New("boom yaml")
}

// failingJSON always fails to marshal, forcing writeFormatted's json error path.
type failingJSON struct{}

func (failingJSON) MarshalJSON() ([]byte, error) {
	return nil, errors.New("boom json")
}
