package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stampedLine is one line a process wrote to a stream, with the moment this test read it.
type stampedLine struct {
	At   time.Time
	Text string
}

// stampedWriter records the lines a process writes, as it writes them, so a test can ask what was already
// visible at a given moment. It stands in for a terminal that is watched while the command runs.
type stampedWriter struct {
	mutex   sync.Mutex
	partial string
	lines   []stampedLine
}

func (w *stampedWriter) Write(p []byte) (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	now := time.Now()
	w.partial += string(p)
	for {
		line, rest, found := strings.Cut(w.partial, "\n")
		if !found {
			break
		}
		w.lines = append(w.lines, stampedLine{At: now, Text: line})
		w.partial = rest
	}
	return len(p), nil
}

// find returns the first line that contains every text, or false.
func (w *stampedWriter) find(texts ...string) (stampedLine, bool) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	for _, line := range w.lines {
		matches := true
		for _, text := range texts {
			matches = matches && strings.Contains(line.Text, text)
		}
		if matches {
			return line, true
		}
	}
	return stampedLine{}, false
}

func (w *stampedWriter) text() string {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	var all strings.Builder
	for _, line := range w.lines {
		all.WriteString(line.Text + "\n")
	}
	return all.String()
}

// buildGateTool compiles the acceptance helper that blocks until a file exists. It is the same Go helper
// that stands in for tools elsewhere in the plugin's acceptance tests, so no shell is involved.
func buildGateTool(t *testing.T) string {
	t.Helper()
	name := "gate"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	output := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", output, "./fixtures/toolprobe")
	build.Dir = filepath.Join(repoRoot, "plugins", "terragrunt")
	if combined, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cannot build the gate helper: %v\n%s", err, combined)
	}
	return output
}

// A unit whose inputs run a command that stays open until the test releases it, like a slow run_cmd, a
// registry fetch, or an STS exchange does in real use.
const slowUnit = `terraform { source = "../../../modules/unit" }
inputs = { greeting = run_cmd("--terragrunt-quiet", "%s", "--wait-for", "%s") }
`

// TestTerragruntLogsReachTheTerminalWhileAUnitIsStillEvaluating runs the real CLI, the real plugin, and
// real OpenTofu. A unit's evaluation is held open by a command; the test watches stderr as the process
// writes it and releases the command only after the earlier log line of that unit is visible. If the plugin
// returned its log events with the result, the line could not appear first, the command would never be
// released, and the unit would fail on the helper's own time limit.
func TestTerragruntLogsReachTheTerminalWhileAUnitIsStillEvaluating(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the Terragrunt plugin and runs OpenTofu")
	}
	ensureAtmosRunner(t)
	pluginPackage := buildTerragruntPluginPackage(t)
	project := newTerragruntProject(t, pluginPackage, newFakeAtmosPro(t))
	gateTool := buildGateTool(t)
	gate := filepath.Join(t.TempDir(), "released")
	project.write("live/dev/slow/terragrunt.hcl", fmt.Sprintf(slowUnit, filepath.ToSlash(gateTool), filepath.ToSlash(gate)))

	run := func(args ...string) (*stampedWriter, *stampedWriter, chan error) {
		cmd := atmosRunner.Command(args...)
		cmd.Dir = project.root
		cmd.Env = append(cmd.Env, "NO_COLOR=1", "ATMOS_TELEMETRY_ENABLED=false", "TF_IN_AUTOMATION=true")
		stdout, stderr := &stampedWriter{}, &stampedWriter{}
		cmd.Stdout, cmd.Stderr = stdout, stderr
		require.NoError(t, cmd.Start())
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		return stdout, stderr, done
	}

	t.Run("an earlier log line is on screen before the command it precedes finishes", func(t *testing.T) {
		stdout, stderr, done := run("terraform", "plan", "slow", "-s", "dev", "--logs-level=Debug")
		defer func() { _ = os.WriteFile(gate, nil, 0o600) }()

		require.Eventually(t, func() bool {
			_, seen := stderr.find("Evaluating unit", "component=slow", "stack=dev")
			return seen
		}, 2*time.Minute, 10*time.Millisecond, "the unit's log line never appeared while its command was open:\n%s", stderr.text())
		// Only preparing the unit scopes an event to its component and stack, and preparing is the call that
		// runs the held command right after logging this line.
		early, _ := stderr.find("Evaluating unit", "component=slow", "stack=dev")
		select {
		case err := <-done:
			t.Fatalf("the command finished before it was released (%v), so the line was not delivered live:\n%s", err, stderr.text())
		default:
		}
		assert.Contains(t, early.Text, "unit=live/dev/slow")
		assert.Contains(t, early.Text, "plugin=terragrunt")

		released := time.Now()
		require.NoError(t, os.WriteFile(gate, nil, 0o600))
		err := <-done
		finished := time.Now()
		require.NoError(t, err, "stderr:\n%s", stderr.text())
		assert.True(t, early.At.Before(released), "line at %s, release at %s", early.At, released)
		assert.True(t, released.Before(finished))
		assert.NotContains(t, stdout.text(), "Evaluating unit", "log events never reach the data channel")
	})

	t.Run("the default level shows no engine events", func(t *testing.T) {
		require.NoError(t, os.WriteFile(gate, nil, 0o600))
		stdout, stderr, done := run("terraform", "plan", "slow", "-s", "dev")
		require.NoError(t, <-done, "stderr:\n%s", stderr.text())
		assert.NotContains(t, stderr.text(), "Evaluating unit")
		assert.NotContains(t, stdout.text(), "Evaluating unit")
	})
}
