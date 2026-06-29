// Package terraform provides the CI provider implementation for Terraform.
package terraform

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/perf"
)

// testJSONMaxLine is the max bufio token size for a `test -json` line (a
// diagnostic with a long detail can be large).
const testJSONMaxLine = 4 * 1024 * 1024

// millisecondsPerSecond converts the `elapsed` field (milliseconds in the
// `test -json` stream) to the seconds used by JUnit `time` attributes.
const millisecondsPerSecond = 1000.0

// Regular expressions for parsing terraform stdout (errors/warnings only).
// These are compiled at package initialization and are safe for concurrent use
// as regexp.Regexp is immutable after compilation.
var (
	// Matches error messages in terraform output.
	errorRe = regexp.MustCompile(`(?m)^(?:[│|] )?Error:\s*(.+)$`)

	// Matches warning messages in terraform output.
	warningRe = regexp.MustCompile(`(?m)^(?:[│|] )?Warning:\s*(.+)$`)

	// Matches apply summary: "Apply complete! Resources: X added, Y changed, Z destroyed."
	// Used as fallback when JSON is not available.
	applySummaryRe = regexp.MustCompile(`Apply complete!\s*Resources:\s*(\d+)\s+added,\s*(\d+)\s+changed,\s*(\d+)\s+destroyed`)

	// Matches destroy summary: "Destroy complete! Resources: X destroyed."
	// Used as fallback when JSON is not available.
	destroySummaryRe = regexp.MustCompile(`Destroy complete!\s*Resources:\s*(\d+)\s+destroyed`)

	// Matches "Plan: X to add, Y to change, Z to destroy."
	// Used as fallback when JSON is not available.
	planSummaryRe = regexp.MustCompile(`Plan:\s*(\d+)\s+to add,\s*(\d+)\s+to change,\s*(\d+)\s+to destroy`)

	// NoChangesRe matches "No changes. Your infrastructure matches the configuration.".
	noChangesRe = regexp.MustCompile(`No changes\.|Your infrastructure matches the configuration`)

	// Matches resource action lines in terraform plan output:
	//   # resource_type.name will be created
	//   # resource_type.name will be updated in-place
	//   # resource_type.name will be destroyed
	//   # resource_type.name must be replaced
	//   # resource_type.name will be read during apply
	resourceActionRe  = regexp.MustCompile(`(?m)^\s*#\s+(\S+)\s+will be (created|destroyed|updated in-place|read during apply)`)
	resourceReplaceRe = regexp.MustCompile(`(?m)^\s*#\s+(\S+)\s+must be replaced`)

	// Matches resource operation progress lines in terraform apply stdout:
	//   aws_instance.web: Creating...
	//   aws_instance.web: Modifying... [id=i-12345]
	//   aws_instance.web: Destroying... [id=i-12345]
	applyResourceActionRe = regexp.MustCompile(`(?m)^(\S+): (Creating|Modifying|Destroying)\.\.\.`)

	// Matches "Outputs:" section header in terraform apply stdout.
	outputsSectionRe = regexp.MustCompile(`(?m)^Outputs:\s*$`)

	// Matches "Changes to Outputs:" section header in terraform plan stdout.
	// This appears when only output values change (no resource changes).
	outputChangesRe = regexp.MustCompile(`(?m)^Changes to Outputs:`)

	// Matches simple output lines: 'key = "value"' or 'key = value'.
	// Captures key and raw value (including quotes).
	outputLineRe = regexp.MustCompile(`^(\w+)\s*=\s*(.+)$`)

	// Matches per-run result lines in `terraform test` stdout:
	//   run "bucket_name_is_namespaced"... pass
	//   run "provisions_resources"... fail
	//   run "skipped_case"... skip
	testRunRe = regexp.MustCompile(`(?m)^\s*run "([^"]+)"\.\.\.\s*(pass|fail|skip)\s*$`)

	// Matches the `terraform test` summary line, e.g.:
	//   Success! 3 passed, 0 failed.
	//   Failure! 2 passed, 1 failed.
	// Used as a fallback when per-run lines were not captured.
	testSummaryRe = regexp.MustCompile(`(?m)^(?:Success|Failure)!\s*(\d+)\s+passed,\s*(\d+)\s+failed`)
)

// ParsePlanJSON parses terraform plan JSON from `terraform show -json <planfile>`.
// This is the preferred method for parsing plan data as it provides structured data.
func ParsePlanJSON(jsonData []byte) (*plugin.OutputResult, error) {
	defer perf.Track(nil, "terraform.ParsePlanJSON")()

	var plan tfjson.Plan
	if err := json.Unmarshal(jsonData, &plan); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal plan JSON: %w", errUtils.ErrParseFile, err)
	}

	result := newEmptyResult()
	data := result.Data.(*plugin.TerraformOutputData)

	processResourceChanges(plan.ResourceChanges, data)
	processOutputChanges(plan.OutputChanges, data)

	result.HasChanges = hasResourceChanges(data.ResourceCounts) || data.HasOutputChanges
	if hasResourceChanges(data.ResourceCounts) {
		data.ChangedResult = buildChangeSummary(data.ResourceCounts)
	} else if data.HasOutputChanges {
		data.ChangedResult = buildOutputChangeSummary(len(data.Outputs))
	}

	return result, nil
}

// newEmptyResult creates an empty OutputResult with initialized data.
func newEmptyResult() *plugin.OutputResult {
	return &plugin.OutputResult{
		ExitCode:   0,
		HasChanges: false,
		HasErrors:  false,
		Errors:     nil,
		Data: &plugin.TerraformOutputData{
			ResourceCounts:    plugin.ResourceCounts{},
			CreatedResources:  []string{},
			UpdatedResources:  []string{},
			ReplacedResources: []string{},
			DeletedResources:  []string{},
			MovedResources:    []plugin.MovedResource{},
			ImportedResources: []string{},
			Outputs:           make(map[string]plugin.TerraformOutput),
		},
	}
}

// processResourceChanges processes plan resource changes and updates the data.
func processResourceChanges(changes []*tfjson.ResourceChange, data *plugin.TerraformOutputData) {
	for _, rc := range changes {
		if rc == nil || rc.Change == nil {
			continue
		}
		processResourceChange(rc, data)
	}
}

// processResourceChange processes a single resource change.
func processResourceChange(rc *tfjson.ResourceChange, data *plugin.TerraformOutputData) {
	addr := rc.Address
	actions := rc.Change.Actions

	switch {
	case actions.Create():
		data.ResourceCounts.Create++
		data.CreatedResources = append(data.CreatedResources, addr)
	case actions.Delete():
		data.ResourceCounts.Destroy++
		data.DeletedResources = append(data.DeletedResources, addr)
	case actions.Update():
		data.ResourceCounts.Change++
		data.UpdatedResources = append(data.UpdatedResources, addr)
	case actions.Replace():
		data.ResourceCounts.Replace++
		data.ReplacedResources = append(data.ReplacedResources, addr)
	}

	if rc.Change.Importing != nil {
		data.ImportedResources = append(data.ImportedResources, addr)
	}
}

// processOutputChanges processes plan output changes.
func processOutputChanges(changes map[string]*tfjson.Change, data *plugin.TerraformOutputData) {
	for name, output := range changes {
		if output == nil {
			continue
		}
		data.HasOutputChanges = true
		data.Outputs[name] = plugin.TerraformOutput{
			Value:     output.After,
			Sensitive: output.AfterSensitive != nil,
		}
	}
}

// hasResourceChanges checks if there are any resource changes.
func hasResourceChanges(counts plugin.ResourceCounts) bool {
	return counts.Create > 0 || counts.Change > 0 || counts.Replace > 0 || counts.Destroy > 0
}

// OutputJSON represents the structure of `terraform output -json`.
type OutputJSON map[string]OutputValue

// OutputValue represents a single output value from `terraform output -json`.
type OutputValue struct {
	Sensitive bool `json:"sensitive"`
	Type      any  `json:"type"`
	Value     any  `json:"value"`
}

// ParseOutputJSON parses terraform output JSON from `terraform output -json`.
// Returns a map of output name to TerraformOutput.
func ParseOutputJSON(jsonData []byte) (map[string]plugin.TerraformOutput, error) {
	defer perf.Track(nil, "terraform.ParseOutputJSON")()

	var outputs OutputJSON
	if err := json.Unmarshal(jsonData, &outputs); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal output JSON: %w", errUtils.ErrParseFile, err)
	}

	result := make(map[string]plugin.TerraformOutput)
	for name, out := range outputs {
		result[name] = plugin.TerraformOutput{
			Value:     out.Value,
			Type:      formatType(out.Type),
			Sensitive: out.Sensitive,
		}
	}

	return result, nil
}

// formatType converts terraform type to a string representation.
func formatType(t any) string {
	if t == nil {
		return ""
	}
	switch v := t.(type) {
	case string:
		return v
	case []any:
		// Complex types like ["object", {"key": "type"}].
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
		return "complex"
	default:
		return "unknown"
	}
}

// ExtractErrors extracts error messages from terraform stdout.
// This is used when JSON parsing is not available or for runtime errors.
func ExtractErrors(stdout string) []string {
	defer perf.Track(nil, "terraform.ExtractErrors")()

	var errors []string
	matches := errorRe.FindAllStringSubmatch(stdout, -1)
	for _, match := range matches {
		if len(match) > 1 {
			errors = append(errors, strings.TrimSpace(match[1]))
		}
	}
	return errors
}

// ExtractWarnings extracts warning messages from terraform stdout.
func ExtractWarnings(stdout string) []string {
	defer perf.Track(nil, "terraform.ExtractWarnings")()

	var warnings []string
	matches := warningRe.FindAllStringSubmatch(stdout, -1)
	for _, match := range matches {
		if len(match) > 1 {
			warnings = append(warnings, strings.TrimSpace(match[1]))
		}
	}
	return warnings
}

// ExtractErrorBlocks extracts full error blocks from terraform stdout.
func ExtractErrorBlocks(stdout string) []string {
	defer perf.Track(nil, "terraform.ExtractErrorBlocks")()

	return ExtractBlocks(stdout, "Error")
}

// ExtractWarningBlocks extracts full warning blocks from terraform stdout.
func ExtractWarningBlocks(stdout string) []string {
	defer perf.Track(nil, "terraform.ExtractWarningBlocks")()

	return ExtractBlocks(stdout, "Warning")
}

// ExtractBlocks extracts full blocks from terraform stdout.
func ExtractBlocks(stdout string, blockType string) []string {
	defer perf.Track(nil, "terraform.ExtractBlocks")()

	var blocks []string
	lines := strings.Split(stdout, "\n")
	var current []string
	inBlock := false

	for _, line := range lines {
		// Strip box-drawing prefix (│ or |) from the line for content checking.
		stripped := strings.TrimPrefix(line, "│ ")
		if stripped == line {
			stripped = strings.TrimPrefix(line, "| ")
		}

		// Detect warning start.
		if strings.HasPrefix(stripped, blockType+": ") && !inBlock {
			inBlock = true
			current = []string{stripped}
			continue
		}

		if inBlock {
			trimmed := strings.TrimSpace(line)
			// End of block on box-drawing end marker.
			if trimmed == "╵" {
				if len(current) > 0 {
					blocks = append(blocks, strings.TrimRight(strings.Join(current, "\n"), "\n"))
				}
				current = nil
				inBlock = false
				continue
			}
			// Detect start of a new block (error or another warning).
			if strings.HasPrefix(stripped, blockType+": ") && len(current) > 0 {
				blocks = append(blocks, strings.TrimRight(strings.Join(current, "\n"), "\n"))
				current = []string{stripped}
				continue
			}
			// Empty lines are preserved as part of the warning block content.
			current = append(current, stripped)
		}
	}

	// Handle block that extends to end of output.
	if inBlock && len(current) > 0 {
		blocks = append(blocks, strings.TrimRight(strings.Join(current, "\n"), "\n"))
	}

	return blocks
}

// ParsePlanOutput parses terraform plan stdout (fallback when JSON not available).
// Prefer ParsePlanJSON when a planfile is available.
func ParsePlanOutput(output string) *plugin.OutputResult {
	defer perf.Track(nil, "terraform.ParsePlanOutput")()

	result := &plugin.OutputResult{
		ExitCode:   0,
		HasChanges: false,
		HasErrors:  false,
		Errors:     nil,
		Data: &plugin.TerraformOutputData{
			ResourceCounts:    plugin.ResourceCounts{},
			CreatedResources:  []string{},
			UpdatedResources:  []string{},
			ReplacedResources: []string{},
			DeletedResources:  []string{},
			MovedResources:    []plugin.MovedResource{},
			ImportedResources: []string{},
			Outputs:           make(map[string]plugin.TerraformOutput),
		},
	}

	data := result.Data.(*plugin.TerraformOutputData)

	// Check for errors.
	if errors := ExtractErrors(output); len(errors) > 0 {
		result.HasErrors = true
		result.Errors = ExtractErrorBlocks(output)
	}

	// Check for no changes.
	if noChangesRe.MatchString(output) {
		result.HasChanges = false
		data.ChangedResult = "No changes. Your infrastructure matches the configuration."
		data.Warnings = ExtractWarningBlocks(output)
		return result
	}

	// Extract individual resource addresses from plan output.
	extractResourceAddresses(output, data)

	// Try to parse plan summary (fallback regex parsing).
	if matches := planSummaryRe.FindStringSubmatch(output); len(matches) == 4 {
		data.ResourceCounts.Create = parseIntOrZero(matches[1])
		data.ResourceCounts.Change = parseIntOrZero(matches[2])
		data.ResourceCounts.Destroy = parseIntOrZero(matches[3])
		result.HasChanges = data.ResourceCounts.Create > 0 ||
			data.ResourceCounts.Change > 0 ||
			data.ResourceCounts.Destroy > 0

		if result.HasChanges {
			data.ChangedResult = buildChangeSummary(data.ResourceCounts)
		}
	}

	// Detect output-only changes (no resource changes but outputs changing).
	if !result.HasChanges && outputChangesRe.MatchString(output) {
		result.HasChanges = true
		data.HasOutputChanges = true
		data.ChangedResult = "Output values will change. No infrastructure changes."
	}

	// Extract full warning blocks for CI summary display.
	data.Warnings = ExtractWarningBlocks(output)

	return result
}

// ParseApplyOutput parses terraform apply stdout.
func ParseApplyOutput(output string) *plugin.OutputResult {
	defer perf.Track(nil, "terraform.ParseApplyOutput")()

	result := &plugin.OutputResult{
		ExitCode:   0,
		HasChanges: false,
		HasErrors:  false,
		Errors:     nil,
		Data: &plugin.TerraformOutputData{
			ResourceCounts:    plugin.ResourceCounts{},
			CreatedResources:  []string{},
			UpdatedResources:  []string{},
			ReplacedResources: []string{},
			DeletedResources:  []string{},
			MovedResources:    []plugin.MovedResource{},
			ImportedResources: []string{},
			Outputs:           make(map[string]plugin.TerraformOutput),
		},
	}

	data := result.Data.(*plugin.TerraformOutputData)

	// Check for errors.
	if errors := ExtractErrors(output); len(errors) > 0 {
		result.HasErrors = true
		result.Errors = errors
	}

	// Parse apply summary.
	if matches := applySummaryRe.FindStringSubmatch(output); len(matches) == 4 {
		data.ResourceCounts.Create = parseIntOrZero(matches[1])
		data.ResourceCounts.Change = parseIntOrZero(matches[2])
		data.ResourceCounts.Destroy = parseIntOrZero(matches[3])
		result.HasChanges = data.ResourceCounts.Create > 0 ||
			data.ResourceCounts.Change > 0 ||
			data.ResourceCounts.Destroy > 0
		data.ChangedResult = matches[0]
	}

	// Extract resource names from apply progress lines.
	// Terraform apply prints "resource.name: Creating/Modifying/Destroying..." for each resource.
	// Use a set to deduplicate (terraform may print multiple lines per resource).
	seen := make(map[string]bool)
	for _, match := range applyResourceActionRe.FindAllStringSubmatch(output, -1) {
		resource := match[1]
		action := match[2]
		key := resource + ":" + action
		if seen[key] {
			continue
		}
		seen[key] = true
		switch action {
		case "Creating":
			data.CreatedResources = append(data.CreatedResources, resource)
		case "Modifying":
			data.UpdatedResources = append(data.UpdatedResources, resource)
		case "Destroying":
			data.DeletedResources = append(data.DeletedResources, resource)
		}
	}

	// Extract outputs from apply stdout (e.g., 'key = "value"' lines after "Outputs:").
	// This avoids needing to run `terraform output` separately, which would require
	// backend credentials that may not be available in PostRunE context.
	data.Outputs = extractApplyOutputs(output)

	// Extract full warning blocks for CI summary display.
	data.Warnings = ExtractWarningBlocks(output)

	return result
}

// ParseDestroyOutput parses terraform destroy stdout.
func ParseDestroyOutput(output string) *plugin.OutputResult {
	defer perf.Track(nil, "terraform.ParseDestroyOutput")()

	result := ParseApplyOutput(output)
	data, _ := result.Data.(*plugin.TerraformOutputData)
	if data == nil {
		return result
	}

	if matches := destroySummaryRe.FindStringSubmatch(output); len(matches) == 2 {
		data.ResourceCounts.Create = 0
		data.ResourceCounts.Change = 0
		data.ResourceCounts.Destroy = parseIntOrZero(matches[1])
		result.HasChanges = data.ResourceCounts.Destroy > 0
		data.ChangedResult = matches[0]
	}

	return result
}

// extractApplyOutputs extracts terraform outputs from apply stdout.
// After a successful apply, terraform prints outputs in the format:
//
//	Outputs:
//
//	key1 = "string_value"
//	key2 = 42
//	key3 = true
//
// Complex values (lists, maps) span multiple lines and are captured as raw strings.
// This is used instead of running `terraform output` separately, which would
// require backend credentials that may not be available in PostRunE context.
func extractApplyOutputs(output string) map[string]plugin.TerraformOutput {
	defer perf.Track(nil, "terraform.extractApplyOutputs")()

	outputs := make(map[string]plugin.TerraformOutput)

	// Find the "Outputs:" section.
	loc := outputsSectionRe.FindStringIndex(output)
	if loc == nil {
		return outputs
	}

	// Parse lines after "Outputs:".
	section := output[loc[1]:]
	lines := strings.Split(section, "\n")

	var currentKey string
	var currentValue strings.Builder
	depth := 0 // Track nesting depth for complex values (lists, maps).

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines.
		if trimmed == "" {
			continue
		}

		// If we're inside a complex value (depth > 0), accumulate lines.
		if depth > 0 {
			currentValue.WriteString("\n")
			currentValue.WriteString(trimmed)
			depth += strings.Count(trimmed, "[") + strings.Count(trimmed, "{")
			depth -= strings.Count(trimmed, "]") + strings.Count(trimmed, "}")
			if depth <= 0 {
				// Complex value complete.
				outputs[currentKey] = plugin.TerraformOutput{
					Value: currentValue.String(),
				}
				currentKey = ""
				currentValue.Reset()
				depth = 0
			}
			continue
		}

		// Try to match a new output line.
		matches := outputLineRe.FindStringSubmatch(trimmed)
		if matches == nil {
			// Non-output line — end of outputs section.
			break
		}

		key := matches[1]
		rawValue := strings.TrimSpace(matches[2])

		// Check if this is the start of a complex value.
		openCount := strings.Count(rawValue, "[") + strings.Count(rawValue, "{")
		closeCount := strings.Count(rawValue, "]") + strings.Count(rawValue, "}")
		if openCount > closeCount {
			// Multi-line complex value.
			currentKey = key
			currentValue.WriteString(rawValue)
			depth = openCount - closeCount
			continue
		}

		// Simple value — strip surrounding quotes if present.
		value := rawValue
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}

		outputs[key] = plugin.TerraformOutput{
			Value: value,
		}
	}

	return outputs
}

// ParseTestOutput parses terraform test stdout into per-run results and counts.
// Terraform test does not emit structured JSON by default, so this parses the
// human-readable output (keeping the terminal output untouched).
func ParseTestOutput(output string) *plugin.OutputResult {
	defer perf.Track(nil, "terraform.ParseTestOutput")()

	data := &plugin.TerraformTestOutputData{
		Runs: []plugin.TerraformTestRun{},
	}
	result := &plugin.OutputResult{Data: data}

	for _, match := range testRunRe.FindAllStringSubmatch(output, -1) {
		run := plugin.TerraformTestRun{Name: match[1], Status: match[2]}
		switch match[2] {
		case "pass":
			data.Pass++
		case "fail":
			data.Fail++
		case "skip":
			data.Skip++
		}
		data.Runs = append(data.Runs, run)
	}
	data.Total = len(data.Runs)

	// Fall back to the summary line when per-run lines were not captured (e.g.
	// output buffering differences); per-run lines are preferred since they also
	// surface skips.
	if data.Total == 0 {
		if match := testSummaryRe.FindStringSubmatch(output); len(match) == 3 {
			data.Pass = parseIntOrZero(match[1])
			data.Fail = parseIntOrZero(match[2])
			data.Total = data.Pass + data.Fail
		}
	}

	// Surface terraform's "Error:" blocks (assertion failures, provider errors).
	if errs := ExtractErrors(output); len(errs) > 0 {
		result.HasErrors = true
		result.Errors = ExtractErrorBlocks(output)
	}
	if data.Fail > 0 {
		result.HasErrors = true
	}

	return result
}

// isJSONStream reports whether output looks like a `test -json` stream (the
// first non-whitespace byte is `{`).
func isJSONStream(output string) bool {
	// The captured stream is usually prefixed with terraform init/workspace
	// preamble (human text) before the `-json` event lines begin, so scan each
	// line for a JSON object carrying terraform's `@level` field rather than only
	// checking the first non-blank character.
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimLeft(line, " \t\r")
		if strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, `"@level"`) {
			return true
		}
	}
	return false
}

// testJSONEvent is the envelope shared by all `terraform test -json` messages.
type testJSONEvent struct {
	Level    string          `json:"@level"`
	Type     string          `json:"type"`
	TestFile string          `json:"@testfile"`
	TestRun  string          `json:"@testrun"`
	TestRunP json.RawMessage `json:"test_run"`
	Summary  json.RawMessage `json:"test_summary"`
	Diag     json.RawMessage `json:"diagnostic"`
}

// testJSONRun is the `test_run` payload.
type testJSONRun struct {
	Path     string `json:"path"`
	Run      string `json:"run"`
	Progress string `json:"progress"`
	Status   string `json:"status"`
	Elapsed  int64  `json:"elapsed"` // milliseconds
}

// testJSONSummary is the `test_summary` payload.
type testJSONSummary struct {
	Status  string `json:"status"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Errored int    `json:"errored"`
	Skipped int    `json:"skipped"`
}

// testJSONDiag is the `diagnostic` payload (mirrors `terraform validate`).
type testJSONDiag struct {
	Summary string `json:"summary"`
	Detail  string `json:"detail"`
	Range   struct {
		Filename string `json:"filename"`
		Start    struct {
			Line int `json:"line"`
		} `json:"start"`
	} `json:"range"`
}

// testRunKey identifies a run by its file + name (used to attach diagnostics).
type testRunKey struct {
	file string
	run  string
}

// pendingDiag holds the first error diagnostic seen for a run, attached when the
// run's `complete` event arrives (diagnostics precede the complete event).
type pendingDiag struct {
	message string
	file    string
	line    int
}

// ParseTestJSON parses a `terraform/tofu test -json` event stream into per-run
// results enriched with failure messages and source file:line (from diagnostic
// ranges). Tool-agnostic: both Terraform and OpenTofu emit this format.
func ParseTestJSON(stream []byte) *plugin.OutputResult {
	defer perf.Track(nil, "terraform.ParseTestJSON")()

	data := &plugin.TerraformTestOutputData{Runs: []plugin.TerraformTestRun{}}
	result := &plugin.OutputResult{Data: data}

	diagByRun := map[testRunKey]pendingDiag{}
	var summary *testJSONSummary

	scanner := bufio.NewScanner(bytes.NewReader(stream))
	scanner.Buffer(make([]byte, 0, 64*1024), testJSONMaxLine)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev testJSONEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}

		switch ev.Type {
		case "diagnostic":
			recordDiagnostic(&ev, diagByRun)
		case "test_run":
			if run, ok := completedTestRun(&ev, diagByRun); ok {
				data.Runs = append(data.Runs, run)
			}
		case "test_summary":
			if s := parseTestSummary(&ev); s != nil {
				summary = s
			}
		}
	}

	finalizeTestJSON(data, result, summary)
	return result
}

// recordDiagnostic remembers the first error diagnostic for a run keyed by
// file+run, so it can be attached when that run's complete event arrives later.
func recordDiagnostic(ev *testJSONEvent, diagByRun map[testRunKey]pendingDiag) {
	if ev.Level != "error" || ev.TestRun == "" {
		return
	}
	var d testJSONDiag
	_ = json.Unmarshal(ev.Diag, &d)
	key := testRunKey{file: ev.TestFile, run: ev.TestRun}
	if _, exists := diagByRun[key]; !exists {
		diagByRun[key] = pendingDiag{message: diagMessage(d), file: d.Range.Filename, line: d.Range.Start.Line}
	}
}

// completedTestRun decodes a test_run event and returns the assembled run only
// when the run has completed (intermediate progress events are ignored).
func completedTestRun(ev *testJSONEvent, diagByRun map[testRunKey]pendingDiag) (plugin.TerraformTestRun, bool) {
	var tr testJSONRun
	_ = json.Unmarshal(ev.TestRunP, &tr)
	if tr.Progress != "complete" {
		return plugin.TerraformTestRun{}, false
	}
	return buildTestRun(ev, tr, diagByRun), true
}

// parseTestSummary decodes a test_summary event, returning nil on malformed input.
func parseTestSummary(ev *testJSONEvent) *testJSONSummary {
	var s testJSONSummary
	if json.Unmarshal(ev.Summary, &s) == nil {
		return &s
	}
	return nil
}

// buildTestRun assembles one run from its complete event, attaching any pending
// diagnostic (file/line/message) recorded earlier in the stream.
func buildTestRun(ev *testJSONEvent, tr testJSONRun, diagByRun map[testRunKey]pendingDiag) plugin.TerraformTestRun {
	file := firstNonEmpty(tr.Path, ev.TestFile)
	name := firstNonEmpty(tr.Run, ev.TestRun)
	run := plugin.TerraformTestRun{
		Name:     name,
		File:     file,
		Status:   tr.Status,
		Duration: float64(tr.Elapsed) / millisecondsPerSecond,
	}
	if dg, ok := diagByRun[testRunKey{file: ev.TestFile, run: name}]; ok {
		run.Error = dg.message
		if dg.line > 0 {
			run.Line = dg.line
		}
		if dg.file != "" {
			run.File = dg.file
		}
	}
	return run
}

// finalizeTestJSON sets totals/counts and HasErrors from the parsed runs and the
// authoritative test_summary (when present).
func finalizeTestJSON(data *plugin.TerraformTestOutputData, result *plugin.OutputResult, summary *testJSONSummary) {
	data.Total = len(data.Runs)
	for _, r := range data.Runs {
		switch r.Status {
		case "pass":
			data.Pass++
		case "skip":
			data.Skip++
		case "fail", "error":
			data.Fail++
		}
		if r.Error != "" {
			result.Errors = append(result.Errors, r.Error)
		}
	}
	if summary != nil {
		data.Pass = summary.Passed
		data.Fail = summary.Failed + summary.Errored
		data.Skip = summary.Skipped
		if data.Total == 0 {
			data.Total = summary.Passed + summary.Failed + summary.Errored + summary.Skipped
		}
	}
	if data.Fail > 0 || len(result.Errors) > 0 {
		result.HasErrors = true
	}
}

// diagMessage joins a diagnostic summary and detail into a single message.
func diagMessage(d testJSONDiag) string {
	if d.Detail != "" && d.Summary != "" {
		return d.Summary + ": " + d.Detail
	}
	return firstNonEmpty(d.Summary, d.Detail)
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// RenderTestText renders a concise, human-readable summary of a
// `terraform/tofu test -json` stream — used to keep the terminal/CI log readable
// when the raw JSON stream is suppressed. Returns "" when the stream has no runs
// (the caller can fall back to surfacing stderr).
func RenderTestText(jsonStream []byte) string {
	defer perf.Track(nil, "terraform.RenderTestText")()

	data, ok := ParseTestJSON(jsonStream).Data.(*plugin.TerraformTestOutputData)
	if !ok || data == nil || data.Total == 0 {
		return ""
	}

	var b strings.Builder
	for _, run := range data.Runs {
		icon := "✓"
		switch run.Status {
		case "fail", "error":
			icon = "✗"
		case "skip":
			icon = "⏭"
		}
		fmt.Fprintf(&b, "  %s run %q... %s\n", icon, run.Name, run.Status)
		if run.Error != "" {
			fmt.Fprintf(&b, "      %s\n", run.Error)
		}
	}
	headline := "Success!"
	if data.Fail > 0 {
		headline = "Failure!"
	}
	fmt.Fprintf(&b, "%s %d passed, %d failed, %d skipped.\n", headline, data.Pass, data.Fail, data.Skip)
	return b.String()
}

// ParseOutput parses terraform output for a given command (fallback when JSON not available).
// Prefer ParsePlanJSON + ParseOutputJSON for structured data.
func ParseOutput(output string, command string) *plugin.OutputResult {
	defer perf.Track(nil, "terraform.ParseOutput")()

	switch command {
	case "plan":
		return ParsePlanOutput(output)
	case "apply":
		return ParseApplyOutput(output)
	case "destroy":
		return ParseDestroyOutput(output)
	case "test":
		// `terraform/tofu test -json` (CI mode) emits line-delimited JSON; the
		// human runner emits text. Sniff the first non-blank char to choose.
		if isJSONStream(output) {
			return ParseTestJSON([]byte(output))
		}
		return ParseTestOutput(output)
	default:
		// For unknown commands, return minimal result.
		return &plugin.OutputResult{
			ExitCode:   0,
			HasChanges: false,
			HasErrors:  false,
			Errors:     nil,
			Data: &plugin.TerraformOutputData{
				ResourceCounts:    plugin.ResourceCounts{},
				CreatedResources:  []string{},
				UpdatedResources:  []string{},
				ReplacedResources: []string{},
				DeletedResources:  []string{},
				MovedResources:    []plugin.MovedResource{},
				ImportedResources: []string{},
				Outputs:           make(map[string]plugin.TerraformOutput),
			},
		}
	}
}

// extractResourceAddresses extracts individual resource addresses from terraform plan stdout.
// It parses lines like "# aws_instance.example will be created" to populate resource lists.
func extractResourceAddresses(output string, data *plugin.TerraformOutputData) {
	// Extract create/update/destroy resources.
	for _, match := range resourceActionRe.FindAllStringSubmatch(output, -1) {
		addr := match[1]
		action := match[2]
		switch action {
		case "created":
			data.CreatedResources = append(data.CreatedResources, addr)
		case "destroyed":
			data.DeletedResources = append(data.DeletedResources, addr)
		case "updated in-place":
			data.UpdatedResources = append(data.UpdatedResources, addr)
		}
	}

	// Extract replaced resources.
	for _, match := range resourceReplaceRe.FindAllStringSubmatch(output, -1) {
		data.ReplacedResources = append(data.ReplacedResources, match[1])
	}
}

// buildChangeSummary builds a human-readable change summary string.
func buildChangeSummary(counts plugin.ResourceCounts) string {
	var parts []string
	if counts.Create > 0 {
		parts = append(parts, resourceCount(counts.Create)+" to add")
	}
	if counts.Change > 0 {
		parts = append(parts, resourceCount(counts.Change)+" to change")
	}
	if counts.Replace > 0 {
		parts = append(parts, resourceCount(counts.Replace)+" to replace")
	}
	if counts.Destroy > 0 {
		parts = append(parts, resourceCount(counts.Destroy)+" to destroy")
	}
	if len(parts) == 0 {
		return "No changes"
	}
	return strings.Join(parts, ", ")
}

// buildOutputChangeSummary builds a human-readable summary for output-only changes.
func buildOutputChangeSummary(count int) string {
	if count == 1 {
		return "1 output to change"
	}
	return strconv.Itoa(count) + " outputs to change"
}

// resourceCount formats a count of resources with proper pluralization.
func resourceCount(count int) string {
	if count == 1 {
		return "1 resource"
	}
	return strconv.Itoa(count) + " resources"
}

// parseIntOrZero parses a string to int, returning 0 on error.
func parseIntOrZero(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
