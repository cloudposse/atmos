package output

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const corruptHint = "The cached release of plugin `login` at `/cache/releases/abc` no longer matches its locked digest. Run `atmos plugin install` to quarantine it and re-download it."

func TestCorruptStatusRendersDistinctlyWithHint(t *testing.T) {
	stdout, stderr := capture(t)
	interactiveColors(t)
	dot := theme.GetCurrentStyles().Error.Render(theme.IconActive)
	require.Contains(t, dot, "\x1b[", "test must exercise actual colored styles")
	status := manage.Status{Alias: "login", State: "corrupt", Hint: corruptHint}

	require.NoError(t, List([]manage.Status{status}, "table"))
	require.Contains(t, stdout.String(), dot, "corrupt uses the error indicator, not the missing (warning) one")
	require.NotContains(t, stdout.String(), theme.GetCurrentStyles().Warning.Render(theme.IconActive))
	require.Contains(t, ansi.Strip(stdout.String()), "corrupt")
	require.Contains(t, ansi.Strip(stderr.String()), "atmos plugin install", "the hint is shown on the UI channel")
	stdout.Reset()

	require.NoError(t, Info(&status, "table"))
	require.Contains(t, ansi.Strip(stdout.String()), "Hint")
	require.Contains(t, ansi.Strip(stdout.String()), "atmos plugin install")
}

func TestCorruptStatusKeepsMachineOutputCleanAndStructured(t *testing.T) {
	stdout, stderr := capture(t)
	status := manage.Status{Alias: "login", State: "corrupt", Hint: corruptHint}
	require.NoError(t, List([]manage.Status{status}, "json"))
	require.Contains(t, stdout.String(), `"state": "corrupt"`)
	require.Contains(t, stdout.String(), `"hint"`)
	require.NotContains(t, stdout.String(), "\x1b")
	require.Contains(t, stderr.String(), "atmos plugin install")

	healthy := manage.Status{Alias: "ok", State: "installed"}
	stderr.Reset()
	require.NoError(t, List([]manage.Status{healthy}, "json"))
	require.Empty(t, stderr.String(), "healthy lists print no hints")
}
