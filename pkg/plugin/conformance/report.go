package conformance

import (
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/reported"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// FormatText prints human-readable progress and a summary on the UI channel.
	FormatText = "text"
	// FormatJSON prints the machine-readable report on the data channel.
	FormatJSON = "json"

	detailIndent = "    "
)

// Describe returns the human-readable label for a stable result name.
func Describe(name string) string {
	defer perf.Track(nil, "conformance.Describe")()

	switch name {
	case "manifest-handler-agreement":
		return "**Plugin contract:** executable capabilities match `plugin.yaml`"
	case "oidc-lifecycle":
		return "**OIDC lifecycle:** PKCE login, token validation, credential exchange, cached reuse, refresh, and logout (local fixtures)"
	case "authored-tests":
		return "**Authored tests:** plugin-defined test steps"
	default:
		if command, found := strings.CutPrefix(name, "command/"); found {
			return fmt.Sprintf("**Command `%s`:** output and exit status match the fixture", command)
		}
		if extension, found := strings.CutPrefix(name, "extension/"); found {
			return fmt.Sprintf("**Extension `%s`:** registered capability checks", extension)
		}
		return fmt.Sprintf("**Check `%s`**", name)
	}
}

// Show prints one completed check. A failing check is followed by its recorded evidence, so the
// reader sees expected versus actual values without rerunning the suite in another format.
func Show(result Result) {
	defer perf.Track(nil, "conformance.Show")()

	if result.Passed {
		ui.Success(Describe(result.Name))
		return
	}
	ui.Error(Describe(result.Name))
	if result.Detail != "" {
		ui.Writeln(indentDetail(result.Detail))
	}
}

func indentDetail(detail string) string {
	lines := strings.Split(strings.TrimRight(detail, "\n"), "\n")
	for index := range lines {
		lines[index] = detailIndent + lines[index]
	}
	return strings.Join(lines, "\n")
}

// Present reports a finished run. Failures that the report already shows are marked reported, so
// the root error handler exits non-zero without printing the same failure a second time. Other
// errors, such as a policy denial or a --run filter that selected nothing, are returned
// unchanged and keep their explanation and hints.
func Present(report *Report, err error, format string) error {
	defer perf.Track(nil, "conformance.Present")()

	if report == nil {
		return err
	}
	if format == FormatJSON {
		return errors.Join(markShown(err), data.WriteJSON(report))
	}
	return summarize(report, err)
}

func summarize(report *Report, err error) error {
	passed := 0
	for _, result := range report.Results {
		if result.Passed {
			passed++
		}
	}
	failed := len(report.Results) - passed
	switch {
	case err == nil:
		ui.Successf("All **%d plugin checks passed**.", passed)
		return nil
	case failed > 0:
		ui.Errorf("Plugin tests failed: **%d passed, %d failed**.", passed, failed)
		return markShown(err)
	default:
		return err
	}
}

// markShown silences the root handler for failures the report already presented.
func markShown(err error) error {
	if errors.Is(err, errUtils.ErrTestsFailed) {
		return reported.Wrap(err)
	}
	return err
}
