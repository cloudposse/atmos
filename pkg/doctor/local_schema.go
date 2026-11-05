package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cloudposse/atmos/pkg/datafetcher"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/validation"
)

const (
	ruleLocalAtmosSchema     = "local-atmos-schema"
	customLocalSchemaMessage = "Custom local schema overrides Atmos validation"
)

// checkLocalSchemas runs before stack discovery, so a broken local override still receives
// the modernization recommendation. It never edits or deletes schemas or editor associations.
func (e *Engine) checkLocalSchemas(p *project, o *Options, r *Report) {
	if !enabled(o, ruleLocalAtmosSchema, ruleModernization) {
		return
	}
	for _, d := range p.docs {
		if d.kind != documentConfig || !selected(o, p, d) {
			continue
		}
		e.checkDocumentLocalSchemas(p, d, r)
	}
	if value, _ := os.LookupEnv("ATMOS_SCHEMAS_ATMOS_MANIFEST"); localSchemaSource(value) {
		f := e.localSchemaFinding(p, value, stackSchema)
		f.Path = "ATMOS_SCHEMAS_ATMOS_MANIFEST"
		if f.Message != customLocalSchemaMessage {
			f.Remediation = "Unset ATMOS_SCHEMAS_ATMOS_MANIFEST and remove any schemas.atmos.manifest override to use the bundled schema. " + f.Remediation
		}
		r.Findings = append(r.Findings, f)
	}
}

func (e *Engine) checkDocumentLocalSchemas(p *project, d *document, r *Report) {
	for _, path := range [][]string{{"schemas", "atmos", "manifest"}, {"schemas", "config", "schema"}} {
		n := nodeAt(rootNode(d.node), path)
		if n == nil || n.Tag != "!!str" || !localSchemaSource(n.Value) {
			continue
		}
		embedded := stackSchema
		if path[1] == "config" {
			embedded = configSchema
		}
		f := e.localSchemaFinding(p, n.Value, embedded)
		f.File = d.path
		f.Path = displayNodePath(path)
		f.Line = n.Line
		f.Column = n.Column
		f.Remediation = localSchemaRemediation(&f, embedded)
		r.Findings = append(r.Findings, f)
	}
}

func localSchemaRemediation(f *Finding, embedded string) string {
	if f.Message == customLocalSchemaMessage {
		return f.Remediation
	}
	remediation := "Remove the local override at " + f.Path + " to use the schema bundled with the installed Atmos version. " + f.Remediation
	if embedded == configSchema {
		remediation = "Use the built-in schemas.config entry; preserve custom matches by setting its schema to " + configSchema + " when necessary. " + remediation
	}
	return remediation
}

func localSchemaSource(value string) bool {
	return value != "" && !strings.Contains(value, "://") && !strings.Contains(value, "{{") && !strings.HasPrefix(value, "!")
}

// localSchemaPath follows manifest lookup order: working/project directory, then base_path.
func (e *Engine) localSchemaPath(p *project, source string) string {
	path := absolute(p.root, source)
	if _, err := e.FS.Stat(path); err == nil || filepath.IsAbs(source) {
		return path
	}
	return absolute(p.base, source)
}

func (e *Engine) localSchemaFinding(p *project, source, embedded string) Finding {
	path := e.localSchemaPath(p, source)
	f := Finding{RuleID: ruleLocalAtmosSchema, Category: ruleModernization, Severity: validation.SeverityWarning, Status: statusManual, Message: "Local Atmos schema override can drift from the installed version", Documentation: "https://atmos.tools/cli/configuration/schemas"}
	f.Remediation = "After reviewing custom constraints and updating remaining references (including editor schema associations), delete the obsolete local copy " + path + ". Component schemas and OPA/CUE policies are independent."
	content, err := e.FS.ReadFile(path)
	if err != nil {
		f.Reason = "Local schema could not be read: " + err.Error()
		return f
	}
	var local map[string]any
	if err := json.Unmarshal(content, &local); err != nil {
		f.Reason = "Local schema is not valid JSON: " + err.Error()
		return f
	}
	bundled, err := datafetcher.NewDataFetcher(&schema.AtmosConfiguration{}).GetData(embedded)
	if err != nil {
		f.Reason = "Bundled schema could not be compared: " + err.Error()
		return f
	}
	var current map[string]any
	if err := json.Unmarshal(bundled, &current); err != nil {
		f.Reason = err.Error()
		return f
	}
	if reflect.DeepEqual(local, current) {
		f.Reason = "Local schema is identical to the bundled schema; the copy is redundant"
		return f
	}
	id, _ := local["$id"].(string)
	official := id == "https://json.schemastore.org/atmos-manifest.json" || strings.HasPrefix(id, "https://atmos.tools/schemas/atmos/")
	if official {
		f.Reason = "Local schema differs from the bundled schema; this may be an older copy or contain custom constraints. A difference does not establish its age"
	} else {
		f.Message = customLocalSchemaMessage
		f.Reason = fmt.Sprintf("Schema %q is not identifiable as a bundled Atmos schema; review intentional custom validation before changing the override", id)
		f.Remediation = "Review and preserve custom validation before switching to the bundled schema, which does not include your added constraints; keep this file while it is required by validation or editor associations."
	}
	return f
}
