package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/validation"
)

func TestCompilerDiagnosticSourceAndFormats(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte("components:\n  terraform:\n    init_run_reconfigure: false\n"), 0o600))
	r := diagnostic.Report{SourceRoot: root, Findings: []diagnostic.Finding{{
		File: "atmos.yaml", Line: 3, RuleID: "terraform-reconfigure", Severity: validation.SeverityWarning,
		Message: "Legacy declaration", Status: "available", Fixable: true, Remediation: "Run atmos doctor --fix",
	}}}
	var previous string
	for _, format := range []string{"text", "rich"} {
		t.Run(format, func(t *testing.T) {
			stdout, stderr := captureIO(t)
			ui.SetColorProfile(termenv.Ascii)
			require.NoError(t, render(&r, format, renderOptions{}))
			output := stderr.String()
			require.Empty(t, stdout.String())
			require.NotContains(t, output, "\x1b")
			require.Contains(t, output, "warning[terraform-reconfigure]: Legacy declaration\n  --> atmos.yaml:3")
			require.Contains(t, output, "2 |   terraform:\n3 |     init_run_reconfigure: false\n  |     ^")
			require.Contains(t, output, "= repair: Automatic fix available")
			require.Equal(t, 1, strings.Count(output, "atmos doctor --fix"))
			if previous != "" {
				require.Equal(t, previous, output)
			}
			previous = output
		})
	}
}

func TestCompilerDiagnosticWidthAndColors(t *testing.T) {
	root := t.TempDir()
	source := "  value: " + strings.Repeat("界", 60) + "false\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "long.yaml"), []byte(source), 0o600))
	for _, width := range []int{50, 80, 120} {
		for _, profile := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
			t.Run(fmt.Sprintf("%d/%d", width, profile), func(t *testing.T) {
				_, stderr := captureIO(t)
				ui.SetColorProfile(profile)
				p := newReportRenderer()
				p.width = width
				p.finding(&diagnostic.Finding{
					File: "long.yaml", Line: 1, Column: 70, RuleID: "rule", Severity: validation.SeverityWarning,
					Message: strings.Repeat("Long diagnostic explanation ", 8), Status: "manual", Reason: strings.Repeat("reason ", 30),
					Remediation: strings.Repeat("long-unbroken-path/", 15),
				}, root)
				output := stderr.String()
				for _, line := range strings.Split(output, "\n") {
					assert.LessOrEqual(t, ansi.StringWidth(line), width, line)
				}
				plain := ansi.Strip(output)
				assert.Contains(t, plain, "…")
				assert.Contains(t, plain, "^")
				if profile == termenv.Ascii {
					assert.NotContains(t, output, "\x1b")
				} else {
					assert.Contains(t, output, "\x1b[")
				}
			})
		}
	}
}

func TestCompilerMarkerDisplayCells(t *testing.T) {
	for _, tc := range []struct {
		line         string
		column, want int
	}{
		{"    key: value", 0, 5}, {"\tkey: value", 0, 5}, {"  界: value", 6, 7}, {"value", 99, 6},
	} {
		assert.Equal(t, tc.want, sourceColumn(tc.line, tc.column))
	}
	source := strings.Repeat("界", 30) + "target"
	shown, marker := clipSource(source, 20, 61)
	assert.LessOrEqual(t, ansi.StringWidth(shown), 20)
	assert.Equal(t, "t", ansi.Cut(shown, marker-1, marker))
}

func TestCompilerMissingSourceRetainsGuidance(t *testing.T) {
	for _, line := range []int{0, 1, 999} {
		t.Run(fmt.Sprint(line), func(t *testing.T) {
			_, stderr := captureIO(t)
			p := newReportRenderer()
			p.finding(&diagnostic.Finding{
				File: "missing.yaml", Line: line, RuleID: "schema", Severity: validation.SeverityError,
				Message: "bad configuration", Reason: "explanation", Remediation: "fix it", Status: "verification-failed",
			}, t.TempDir())
			output := plainText(stderr)
			for _, want := range []string{"error[schema]: bad configuration", "missing.yaml", "Verification failed", "= note: explanation", "= help: fix it"} {
				assert.Contains(t, output, want)
			}
			assert.NotContains(t, output, "^")
		})
	}
}

func TestCompilerMasksBeforeClippingAndWrapping(t *testing.T) {
	_, stderr := captureIO(t)
	secret := strings.Repeat("sensitive-token-", 12)
	iolib.RegisterValue(secret)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "secret.yaml"), []byte("token: "+secret+"\n"), 0o600))
	ui.SetColorProfile(termenv.TrueColor)
	p := newReportRenderer()
	p.width = 50
	p.finding(&diagnostic.Finding{File: "secret.yaml", Line: 1, RuleID: "rule", Severity: validation.SeverityWarning, Message: secret, Reason: secret}, root)
	output := ansi.Strip(stderr.String())
	assert.NotContains(t, output, "sensitive-token")
	assert.Contains(t, output, "MASKED")
}

func TestDryRunDoesNotClaimUnverifiedRepairs(t *testing.T) {
	_, stderr := captureIO(t)
	r := diagnostic.Report{DryRun: true, OperationalError: "verification stopped", Findings: []diagnostic.Finding{{Status: "available", Fixable: true}}}
	require.NoError(t, renderSummary(&r, renderOptions{Fix: true}))
	output := plainText(stderr)
	assert.Contains(t, output, "0 repairs verified")
	assert.NotContains(t, output, "atmos doctor --fix")
	assert.Contains(t, output, "Operational error")
}

func TestYAMLHighlightUsesThemeWithoutChangingText(t *testing.T) {
	captureIO(t)
	ui.SetColorProfile(termenv.TrueColor)
	p := newReportRenderer()
	for _, line := range []string{"    terraform:", "    enabled: false", "    value: \"hello\" # comment"} {
		shown := highlightYAML(line, p.styles)
		assert.Equal(t, line, ansi.Strip(shown))
		key := strings.Split(strings.TrimSpace(line), ":")[0]
		assert.Contains(t, shown, p.styles.Command.Render(key))
	}
}

func TestCompilerLongLocationAndAbsentLine(t *testing.T) {
	_, stderr := captureIO(t)
	p := newReportRenderer()
	p.width = 50
	path := strings.Repeat("nested/", 20) + "config.yaml"
	p.finding(&diagnostic.Finding{File: path, Line: 99, Severity: validation.SeverityError, RuleID: "schema", Message: "invalid"}, t.TempDir())
	output := ansi.Strip(stderr.String())
	for _, line := range strings.Split(output, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(line), 50)
	}
	assert.Contains(t, output, "config.yaml:99")
	assert.NotContains(t, output, "^")
}

func TestUnwrappedLineKeepsLongPathsOnOneLineAndMasks(t *testing.T) {
	_, stderr := captureIO(t)
	p := newReportRenderer()
	p.width = 20
	path := filepath.Join("home", "user", ".cache", "atmos", "doctor", "backups", "20261015T103000Z")

	p.unwrapped("Backup: ", path, p.styles.Body)

	lines := strings.Split(strings.TrimRight(plainText(stderr), "\n"), "\n")
	require.Len(t, lines, 1, "the backup path must stay on one line so it can be copied")
	assert.Equal(t, "Backup: "+path, lines[0])
}

func TestFixCommandPreservesScopingFlags(t *testing.T) {
	newCommand := func(t *testing.T, args ...string) *cobra.Command {
		t.Helper()
		cmd := NewCommand()
		// Stand in for the root's persistent flags, which Cobra merges into the command's flag set.
		cmd.Flags().StringSlice("config", nil, "")
		cmd.Flags().String("base-path", "", "")
		cmd.Flags().String("profile", "", "")
		cmd.Flags().Bool("verbose", false, "")
		require.NoError(t, cmd.ParseFlags(args))
		return cmd
	}

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"no flags", nil, "atmos doctor --fix"},
		{"output flags are dropped", []string{"--format=rich", "--dry-run", "--fix"}, "atmos doctor --fix"},
		{"scoping flags are kept", []string{"--only=schema", "--offline", "--affected", "--base=main"}, "atmos doctor --fix --affected --base=main --offline --only=schema"},
		{"slice flags repeat per element", []string{"--only=schema,imports", "--config=a.yaml", "--config=b.yaml"}, "atmos doctor --fix --config=a.yaml --config=b.yaml --only=schema --only=imports"},
		{"array flags repeat per element", []string{"--exclude=vendor/**", "--exclude=dist/**"}, "atmos doctor --fix '--exclude=vendor/**' '--exclude=dist/**'"},
		{"values with spaces are quoted", []string{"--base-path=my project", "--profile=prod"}, "atmos doctor --fix '--base-path=my project' --profile=prod"},
		{"explicit false booleans are preserved", []string{"--offline=false", "--verbose"}, "atmos doctor --fix --offline=false --verbose"},
		{"flags set to their default value are still kept", []string{"--fail-on=error"}, "atmos doctor --fix --fail-on=error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fixCommand(newCommand(t, tt.args...)))
		})
	}
}

// TestCompilerBlankSourceLineHasNoTrailingSpace keeps excerpt lines free of trailing whitespace. A blank
// source line in the excerpt renders as its line number and the gutter bar only.
func TestCompilerBlankSourceLineHasNoTrailingSpace(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "dev.yaml"), []byte("vars:\n  stage: dev\n\n   \ncomponents: {}\n"), 0o600))
	r := diagnostic.Report{SourceRoot: root, Findings: []diagnostic.Finding{{
		File: "dev.yaml", Line: 3, Column: 0, RuleID: "schema", Severity: validation.SeverityError,
		Message: "Problem", Status: "manual",
	}}}
	_, stderr := captureIO(t)
	ui.SetColorProfile(termenv.Ascii)
	require.NoError(t, render(&r, "text", renderOptions{}))

	output := stderr.String()
	require.Contains(t, output, "3 |\n")
	require.Contains(t, output, "4 |\n")
	for _, line := range strings.Split(output, "\n") {
		assert.Equal(t, strings.TrimRight(line, " "), line, "line has trailing whitespace: %q", line)
	}
}
