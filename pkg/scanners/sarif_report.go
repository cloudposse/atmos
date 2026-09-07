package scanners

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const nonBlockingSecuritySeverity = "0.0"

func renderCISummary(scan *Context, out *Output) {
	if out == nil || out.Summary == nil || out.Summary.Body == "" {
		return
	}
	if !ciSummaryEnabled(scan) {
		return
	}
	if err := ci.WriteStepSummary("\n" + out.Summary.Body); err != nil {
		log.Debug("Failed to write scanner summary to CI step summary", "error", err)
	}
}

func emitCIAnnotations(scan *Context, out *Output) {
	if out == nil || out.Summary == nil || len(out.Summary.Findings) == 0 {
		return
	}
	if !ciAnnotationsEnabled(scan) {
		return
	}
	annotations := make([]ci.Annotation, 0, len(out.Summary.Findings))
	for _, f := range out.Summary.Findings {
		annotations = append(annotations, ci.Annotation{
			Path:      f.Path,
			StartLine: f.Line,
			Level:     annotationLevelForScan(scan, f.Severity),
			Title:     f.RuleID,
			Message:   f.Message,
		})
	}
	if err := ci.Annotate(annotations); err != nil {
		log.Debug("Failed to emit CI annotations", logKeyScanner, scan.Name, "error", err)
	}
}

func publishCIResults(scan *Context, out *Output) {
	if out == nil || out.Summary == nil || len(out.Summary.SARIF) == 0 {
		return
	}
	if !ciResultsEnabled(scan) {
		return
	}
	body := out.Summary.SARIF
	if reportsAsWarning(scan) {
		body = normalizeSARIFLevels(body, "warning")
	}
	report := ci.SARIFReport{Body: body, Category: deriveSARIFCategory(scan, body)}
	if err := ci.ReportSARIF(context.Background(), report); err != nil {
		log.Debug("Failed to publish SARIF results to CI provider", logKeyScanner, scan.Name, "error", err)
	}
}

func annotationLevelForScan(scan *Context, severity string) ci.AnnotationLevel {
	if reportsAsWarning(scan) {
		return ci.AnnotationWarning
	}
	return annotationLevelForSeverity(severity)
}

func annotationLevelForSeverity(severity string) ci.AnnotationLevel {
	switch severity {
	case "critical", "high":
		return ci.AnnotationError
	default:
		return ci.AnnotationWarning
	}
}

func reportsAsWarning(scan *Context) bool {
	return scan != nil && scan.OnFailure != OnFailureFail
}

func normalizeSARIFLevels(sarif []byte, level string) []byte {
	if len(sarif) == 0 || level == "" {
		return sarif
	}
	var doc map[string]any
	if err := json.Unmarshal(sarif, &doc); err != nil {
		return sarif
	}
	runs, ok := doc["runs"].([]any)
	if !ok {
		return sarif
	}
	for _, rawRun := range runs {
		run, ok := rawRun.(map[string]any)
		if !ok {
			continue
		}
		normalizeRunResultLevels(run, level)
		normalizeRunRuleLevels(run, level)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return sarif
	}
	return out
}

func normalizeRunResultLevels(run map[string]any, level string) {
	results, ok := run["results"].([]any)
	if !ok {
		return
	}
	for _, rawResult := range results {
		result, ok := rawResult.(map[string]any)
		if ok {
			result["level"] = level
			normalizeSecuritySeverity(result)
		}
	}
}

func normalizeRunRuleLevels(run map[string]any, level string) {
	tool, ok := run["tool"].(map[string]any)
	if !ok {
		return
	}
	driver, ok := tool["driver"].(map[string]any)
	if !ok {
		return
	}
	rules, ok := driver["rules"].([]any)
	if !ok {
		return
	}
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			continue
		}
		defaultConfig, ok := rule["defaultConfiguration"].(map[string]any)
		if !ok {
			defaultConfig = map[string]any{}
			rule["defaultConfiguration"] = defaultConfig
		}
		defaultConfig["level"] = level
		normalizeSecuritySeverity(rule)
	}
}

func normalizeSecuritySeverity(item map[string]any) {
	properties, ok := item["properties"].(map[string]any)
	if !ok {
		return
	}
	if _, ok := properties["security-severity"]; ok {
		properties["security-severity"] = nonBlockingSecuritySeverity
	}
}

func deriveSARIFCategory(scan *Context, sarif []byte) string {
	if toolName := firstSARIFToolName(sarif); toolName != "" {
		return toolName
	}
	if scan == nil {
		return ""
	}
	if scan.Name != "" {
		return scan.Name
	}
	return scan.Command
}

func firstSARIFToolName(sarif []byte) string {
	if len(sarif) == 0 {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal(sarif, &doc); err != nil {
		return ""
	}
	runs, ok := doc["runs"].([]any)
	if !ok {
		return ""
	}
	for _, rawRun := range runs {
		run, ok := rawRun.(map[string]any)
		if !ok {
			continue
		}
		tool, ok := run["tool"].(map[string]any)
		if !ok {
			continue
		}
		driver, ok := tool["driver"].(map[string]any)
		if !ok {
			continue
		}
		name, ok := driver["name"].(string)
		if ok && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

func ciEnabled(scan *Context) bool {
	return scan != nil && scan.AtmosConfig != nil && scan.AtmosConfig.CI.Enabled
}

func ciSummaryEnabled(scan *Context) bool {
	if !ciEnabled(scan) {
		return false
	}
	e := scan.AtmosConfig.CI.Summary.Enabled
	return e == nil || *e
}

func ciAnnotationsEnabled(scan *Context) bool {
	if !ciEnabled(scan) {
		return false
	}
	e := scan.AtmosConfig.CI.Annotations.Enabled
	return e == nil || *e
}

func ciResultsEnabled(scan *Context) bool {
	if !ciEnabled(scan) {
		return false
	}
	e := scan.AtmosConfig.CI.Results.Enabled
	return e != nil && *e
}
