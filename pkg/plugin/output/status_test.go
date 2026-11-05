package output

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func interactiveColors(t *testing.T) {
	t.Helper()
	t.Setenv("ATMOS_FORCE_TTY", "true")
	t.Setenv("ATMOS_FORCE_COLOR", "true")
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLUMNS", "120")
	setTerminalFlag(t, "force-tty", true)
	setTerminalFlag(t, "force-color", true)
	previous := ui.GetColorProfile()
	ui.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { ui.SetColorProfile(previous) })
}

func setTerminalFlag(t *testing.T, name string, value bool) {
	t.Helper()
	previous := viper.Get(name)
	viper.Set(name, value)
	t.Cleanup(func() { viper.Set(name, previous) })
}

func TestInteractiveStatusUsesSharedThemeAndRetainsWords(t *testing.T) {
	stdout, _ := capture(t)
	interactiveColors(t)
	styles := theme.GetCurrentStyles()
	expected := map[string]string{
		"installed": styles.Success.Render(theme.IconActive),
		"missing":   styles.Warning.Render(theme.IconActive),
		"path":      styles.Info.Render(theme.IconActive),
		"broken":    styles.Error.Render(theme.IconActive),
		"disabled":  styles.Muted.Render(theme.IconActive),
	}
	for state, dot := range expected {
		require.Contains(t, dot, "\x1b[", "test must exercise actual colored styles")
		status := manage.Status{Alias: "login", State: state}
		require.NoError(t, List([]manage.Status{status}, "table"))
		require.Contains(t, stdout.String(), dot)
		plain := ansi.Strip(stdout.String())
		require.Contains(t, plain, state)
		for _, line := range strings.Split(plain, "\n") {
			if strings.Contains(line, "login") {
				require.True(t, strings.HasPrefix(strings.TrimSpace(line), theme.IconActive),
					"list follows auth's leading status indicator convention")
			}
		}
		stdout.Reset()
		require.NoError(t, Info(&status, "table"))
		require.Contains(t, stdout.String(), dot)
		require.Regexp(t, `(?m)^\s*●\s+Status\s+`+state+`\s*$`, ansi.Strip(stdout.String()))
		require.NotContains(t, ansi.Strip(stdout.String()), "Indicator")
		stdout.Reset()
	}
}

func TestStatusMachineFormatsStayCleanEvenInTTY(t *testing.T) {
	stdout, _ := capture(t)
	interactiveColors(t)
	status := manage.Status{Alias: "login", State: "installed"}
	for _, outputFormat := range []string{"json", "yaml", "csv", "tsv"} {
		for _, run := range []func() error{
			func() error { return List([]manage.Status{status}, outputFormat) },
			func() error { return Info(&status, outputFormat) },
		} {
			require.NoError(t, run())
			require.Contains(t, stdout.String(), "installed")
			require.NotContains(t, stdout.String(), theme.IconActive)
			require.NotContains(t, stdout.String(), "\x1b")
			stdout.Reset()
		}
	}
}

func TestStatusDegradesForPipesAndNoColor(t *testing.T) {
	stdout, _ := capture(t)
	status := manage.Status{Alias: "login", State: "installed"}
	require.NoError(t, List([]manage.Status{status}, "table"))
	require.Contains(t, stdout.String(), "installed")
	require.NotContains(t, stdout.String(), theme.IconActive)
	require.NotContains(t, stdout.String(), "\x1b")
	stdout.Reset()

	t.Setenv("ATMOS_FORCE_TTY", "true")
	setTerminalFlag(t, "force-tty", true)
	previous := ui.GetColorProfile()
	ui.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { ui.SetColorProfile(previous) })
	require.NoError(t, List([]manage.Status{status}, "table"))
	require.Contains(t, stdout.String(), theme.IconActive)
	require.Contains(t, stdout.String(), "installed")
	require.NotContains(t, stdout.String(), "\x1b")
	stdout.Reset()
	require.NoError(t, Info(&status, "table"))
	require.Regexp(t, `(?m)^\s*●\s+Status\s+installed\s*$`, stdout.String())
	require.NotContains(t, stdout.String(), "\x1b")
}
