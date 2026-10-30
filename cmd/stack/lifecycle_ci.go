package stack

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

const (
	flagCI = "ci"

	// Commands that launch child Atmos invocations carry this annotation. Each
	// child opens its own invocation-level CI log group, and providers do not
	// nest groups, so the parent must not open one around them.
	annotationSkipInvocationGroup = "atmos.ci/skip-invocation-group"

	summaryOutputKey  = "summary"
	keyValueSeparator = "="
	heredocMarker     = "<<"
	newline           = "\n"
	ciOutputEnv       = "ATMOS_CI_OUTPUT"
	ciSummaryEnv      = "ATMOS_CI_SUMMARY"
	githubOutputEnv   = "GITHUB_OUTPUT"
	githubSummaryEnv  = "GITHUB_STEP_SUMMARY"
	ciModeEnv         = "ATMOS_CI"
	ciChildDirPattern = "atmos-stack-ci-*"
	ciFilePermissions = 0o600
	ciDirPermissions  = 0o700
)

// SkipsInvocationGroup reports whether the command that args select opts out of
// the invocation-level CI log group. A command that cannot be resolved never opts out.
func SkipsInvocationGroup(root *cobra.Command, args []string) bool {
	defer perf.Track(nil, "stack.SkipsInvocationGroup")()

	if root == nil || len(args) == 0 {
		return false
	}
	cmd, _, err := root.Find(args)
	if err != nil || cmd == nil {
		return false
	}
	return cmd.Annotations[annotationSkipInvocationGroup] == "true"
}

// lifecycleCIMode reports whether the parent runs in CI mode, with the same
// precedence as the Terraform commands: an explicit --ci wins, then ATMOS_CI or
// CI, then native CI provider detection.
func lifecycleCIMode(cmd *cobra.Command, v *viper.Viper) bool {
	if f := cmd.Flags().Lookup(flagCI); f != nil && f.Changed {
		value, _ := cmd.Flags().GetBool(flagCI)
		return value
	}
	return ci.ResolveMode(v.GetBool(flagCI))
}

// ciChildFiles holds the per-child files that capture CI outputs and the job summary.
type ciChildFiles struct {
	output, summary string
}

// lifecycleCI collects each child's CI outputs and summary in private files and
// publishes one combined summary and one set of namespaced outputs.
type lifecycleCI struct {
	dir      string
	deleted  int
	files    map[string]ciChildFiles
	verb     string
	reporter ci.Reporter
	parent   []string
}

// newLifecycleCI prepares per-child capture files. It returns nil, and does
// nothing, unless CI mode is on and ci.enabled is true.
func newLifecycleCI(config *schema.AtmosConfiguration, opts *lifecycleOptions, plan *lifecycle.Plan, parentEnv []string, reporter ci.Reporter) (*lifecycleCI, error) {
	if !opts.ci || !ci.Enabled(config) || plan == nil || plan.Graph == nil {
		return nil, nil
	}
	state := &lifecycleCI{files: make(map[string]ciChildFiles, len(plan.Graph.Nodes)), verb: opts.verb, deleted: plan.Deleted, reporter: reporter, parent: parentEnv}
	if plan.Graph.Size() == 0 {
		return state, nil
	}
	dir, err := os.MkdirTemp("", ciChildDirPattern)
	if err != nil {
		return nil, err
	}
	state.dir = dir
	ids := make([]string, 0, len(plan.Graph.Nodes))
	for id := range plan.Graph.Nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for i, id := range ids {
		childDir := filepath.Join(dir, strconv.Itoa(i))
		if err := os.Mkdir(childDir, ciDirPermissions); err != nil {
			state.cleanup()
			return nil, err
		}
		state.files[id] = ciChildFiles{output: filepath.Join(childDir, "output"), summary: filepath.Join(childDir, "summary")}
	}
	return state, nil
}

// newLifecycleReporter binds a reporter to the detected CI provider, or to the
// generic provider when CI mode is forced and nothing is detected.
func newLifecycleReporter(config *schema.AtmosConfiguration) ci.Reporter {
	p := ci.Detect()
	if p == nil {
		p, _ = ci.Get(ci.GenericProviderName)
	}
	return ci.NewReporterForProvider(config, p, nil)
}

// nodeEnvironment points one child at its private output and summary files.
// GitHub variables are redirected only when the parent has them.
func (c *lifecycleCI) nodeEnvironment(node *dependency.Node) []string {
	files, ok := c.files[node.ID]
	if !ok {
		return nil
	}
	env := []string{ciOutputEnv + keyValueSeparator + files.output, ciSummaryEnv + keyValueSeparator + files.summary}
	if envValue(c.parent, githubOutputEnv) != "" {
		env = append(env, githubOutputEnv+keyValueSeparator+files.output)
	}
	if envValue(c.parent, githubSummaryEnv) != "" {
		env = append(env, githubSummaryEnv+keyValueSeparator+files.summary)
	}
	return env
}

// envValue returns the value of key in KEY=VALUE entries, or the empty string.
func envValue(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if name, value, found := strings.Cut(env[i], keyValueSeparator); found && name == key {
			return value
		}
	}
	return ""
}

// cleanup removes the capture files.
func (c *lifecycleCI) cleanup() {
	if c != nil && c.dir != "" {
		_ = os.RemoveAll(c.dir)
	}
}

// ciResult is one component entry of the results output.
type ciResult struct {
	Kind      string            `json:"kind"`
	Stack     string            `json:"stack"`
	Component string            `json:"component"`
	Status    string            `json:"status"`
	Outputs   map[string]string `json:"outputs"`
}

// publish writes one combined job summary and one set of outputs for the whole run.
func (c *lifecycleCI) publish(results []scheduler.Result, excluded int) error {
	if c == nil {
		return nil
	}
	entries := make([]ciResult, len(results))
	details := make([]string, 0, len(results))
	for i := range results {
		r := &results[i]
		files := c.files[r.NodeID]
		entries[i] = ciResult{Kind: r.Node.Type, Stack: r.Node.Stack, Component: r.Node.Component, Status: string(r.Status), Outputs: componentOutputs(readCIFile(files.output))}
		if body := strings.TrimSpace(readCIFile(files.summary)); body != "" {
			details = append(details, fmt.Sprintf("<details><summary>%s</summary>\n\n%s\n\n</details>\n", lifecycle.Label(&r.Node), body))
		}
	}
	pairs, err := c.outputs(entries, results, excluded)
	if err != nil {
		return err
	}
	_, summaryErr := c.reporter.Summary(c.summary(entries, results, excluded, details))
	errs := []error{summaryErr}
	for _, pair := range pairs {
		_, outputErr := c.reporter.Output(pair[0], pair[1])
		errs = append(errs, outputErr)
	}
	return errors.Join(errs...)
}

// summary composes the markdown: a result table, the tally, then each child's own summary.
func (c *lifecycleCI) summary(entries []ciResult, results []scheduler.Result, excluded int, details []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### atmos stack %s\n\n| Component | Kind | Stack | Result |\n| --- | --- | --- | --- |\n", c.verb)
	for i := range entries {
		e := &entries[i]
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", markdownCell(e.Component), markdownCell(e.Kind), markdownCell(e.Stack), e.Status)
	}
	fmt.Fprintf(&b, "\n%s\n\n", lifecycleTally(results, excluded, c.deleted))
	for _, d := range details {
		b.WriteString(d)
		b.WriteString(newline)
	}
	return b.String()
}

// outputs returns the namespaced output pairs in a stable order.
func (c *lifecycleCI) outputs(entries []ciResult, results []scheduler.Result, excluded int) ([][2]string, error) {
	counts := map[scheduler.Status]int{}
	for i := range results {
		counts[results[i].Status]++
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	return [][2]string{
		{"components_total", strconv.Itoa(len(results))},
		{"components_succeeded", strconv.Itoa(counts[scheduler.StatusSucceeded])},
		{"components_failed", strconv.Itoa(counts[scheduler.StatusFailed])},
		{"components_skipped", strconv.Itoa(counts[scheduler.StatusSkipped])},
		{"components_excluded", strconv.Itoa(excluded)},
		{"components_deleted", strconv.Itoa(c.deleted)},
		{"success", strconv.FormatBool(counts[scheduler.StatusFailed] == 0)},
		{"results", string(encoded)},
	}, nil
}

// componentOutputs parses a child's outputs for the results list. The child's
// summary output repeats the markdown already shown in its details block, so it
// is left out to keep the results output compact.
func componentOutputs(content string) map[string]string {
	outputs := parseCIOutputs(content)
	delete(outputs, summaryOutputKey)
	return outputs
}

// markdownCell escapes the characters that would break a table cell.
func markdownCell(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

// readCIFile returns a capture file's content; a child that wrote nothing has no file.
func readCIFile(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// parseCIOutputs decodes a CI output file: key=value lines and the heredoc form
// key<<DELIMITER ... DELIMITER used for multiline values. A repeated key keeps its last value.
func parseCIOutputs(content string) map[string]string {
	outputs := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", newline), newline)
	for i := 0; i < len(lines); i++ {
		if key, delimiter, ok := heredocStart(lines[i]); ok {
			var body []string
			for i++; i < len(lines) && lines[i] != delimiter; i++ {
				body = append(body, lines[i])
			}
			outputs[key] = strings.Join(body, newline)
			continue
		}
		if key, value, found := strings.Cut(lines[i], keyValueSeparator); found && key != "" {
			outputs[key] = value
		}
	}
	return outputs
}

// heredocStart recognizes the opening line key<<DELIMITER of a multiline value.
func heredocStart(line string) (key, delimiter string, ok bool) {
	key, delimiter, found := strings.Cut(line, heredocMarker)
	if !found || key == "" || delimiter == "" || strings.Contains(key, keyValueSeparator) {
		return "", "", false
	}
	return key, delimiter, true
}

// withCIMode propagates both enabled and disabled modes. Provider metadata stays
// available to custom commands and hooks, but cannot re-enable Atmos CI output.
func withCIMode(env []string, enabled bool) []string {
	env = slices.DeleteFunc(slices.Clone(env), func(entry string) bool {
		return strings.HasPrefix(entry, ciModeEnv+"=") || strings.HasPrefix(entry, ci.WorkerModeEnv+"=")
	})
	value := strconv.FormatBool(enabled)
	return append(env, ciModeEnv+"="+value, ci.WorkerModeEnv+"="+value)
}
