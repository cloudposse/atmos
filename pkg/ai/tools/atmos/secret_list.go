package atmos

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// SecretListTool lists declared secrets and their local initialization status.
//
// This tool is strictly metadata-only and credential-free: it never authenticates, never
// contacts a remote secret backend, and never calls a decrypt/read-value API (no kms:Decrypt,
// no fetching an actual secret value). It mirrors `atmos secret list` WITHOUT the `--verify`
// flag: declarations are resolved with auth disabled (see credentialFreeSkipTags), and each
// secret's status is checked via secrets.Service.Status(false), which "never registers values
// with the masker (uses the backend status check, not Get) and never decrypts" (see
// pkg/secrets/service.go). Local backends (e.g. SOPS) report a real initialized/missing status
// because their existence check is local; remote-store secrets report "unknown" since verifying
// them would require authenticating to the backend.
type SecretListTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewSecretListTool creates a new secret list tool.
func NewSecretListTool(atmosConfig *schema.AtmosConfiguration) *SecretListTool {
	return &SecretListTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *SecretListTool) Name() string {
	return "atmos_secret_list"
}

// Description returns the tool description.
func (t *SecretListTool) Description() string {
	return "List declared secrets and their initialization status, optionally scoped to a stack " +
		"and/or component. Strictly metadata-only: never decrypts or retrieves a secret value, and " +
		"never contacts a remote secret backend (remote-store status is reported as \"unknown\")."
}

// Parameters returns the tool parameters.
func (t *SecretListTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramStack,
			Description: "Limit results to this stack (omit to list across all stacks).",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramComponent,
			Description: "Limit results to this component (omit to list across all components).",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// secretScopeEntry is a single (stack, component) instance that declares one or more secrets,
// paired with its resolved component section.
type secretScopeEntry struct {
	Stack     string
	Component string
	Section   map[string]any
}

// Execute runs the tool.
func (t *SecretListTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	stack, _ := params[paramStack].(string)
	component, _ := params[paramComponent].(string)

	atmosConfig, err := currentStackConfig(t.atmosConfig)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	var components []string
	if component != "" {
		components = []string{component}
	}

	// Auth is disabled: enumeration only needs the static `secrets.vars` declarations, never a
	// resolved value, so authenticating per component would be pure overhead. With auth disabled,
	// credentialed YAML functions (!secret, !store, !store.get, !terraform.output,
	// !terraform.state) must be skipped too, or an evaluation attempt would fall back to the
	// default cloud credential chain and fail.
	stacksMap, err := exec.ExecuteDescribeStacksWithAuthDisabled(
		atmosConfig, stack, components, nil, nil,
		false, true, true, false, credentialFreeSkipTags(), nil, true,
	)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	entries := collectSecretScopeEntries(stacksMap, component)
	rows := secretStatusRows(atmosConfig, entries)

	return buildSecretListResult(stack, component, rows), nil
}

// credentialFreeSkipTags lists the YAML function tags that must not be evaluated while listing
// secret declarations without authentication (mirrors cmd/secret's credentialFreeSkip).
func credentialFreeSkipTags() []string {
	tags := []string{
		u.AtmosYamlFuncSecret,
		u.AtmosYamlFuncStore,
		u.AtmosYamlFuncStoreGet,
		u.AtmosYamlFuncTerraformOutput,
		u.AtmosYamlFuncTerraformState,
	}
	skip := make([]string, len(tags))
	for i, tag := range tags {
		skip[i] = strings.TrimPrefix(tag, "!")
	}
	return skip
}

// collectSecretScopeEntries traverses the describe-stacks map (stack -> components -> <type> ->
// component -> section) and keeps the instances that declare secrets, optionally narrowed to a
// single component. Entries are sorted by stack then component.
func collectSecretScopeEntries(stacksMap map[string]any, componentFilter string) []secretScopeEntry {
	var entries []secretScopeEntry
	for stackName, raw := range stacksMap {
		stackMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entries = append(entries, secretEntriesInStack(stackName, stackMap, componentFilter)...)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Stack != entries[j].Stack {
			return entries[i].Stack < entries[j].Stack
		}
		return entries[i].Component < entries[j].Component
	})
	return entries
}

// secretEntriesInStack returns the secret-declaring instances within a single stack's describe map.
func secretEntriesInStack(stackName string, stackMap map[string]any, componentFilter string) []secretScopeEntry {
	comps, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
	if !ok {
		return nil
	}
	var entries []secretScopeEntry
	for _, typeRaw := range comps {
		typeMap, ok := typeRaw.(map[string]any)
		if !ok {
			continue
		}
		for compName, secRaw := range typeMap {
			if componentFilter != "" && compName != componentFilter {
				continue
			}
			section, ok := secRaw.(map[string]any)
			if !ok {
				continue
			}
			if len(secrets.ExtractDeclarations(section)) == 0 {
				continue
			}
			entries = append(entries, secretScopeEntry{Stack: stackName, Component: compName, Section: section})
		}
	}
	return entries
}

// secretStatusRow is a single rendered row of secret status output.
type secretStatusRow struct {
	Stack       string `yaml:"stack" json:"stack"`
	Component   string `yaml:"component" json:"component"`
	Secret      string `yaml:"secret" json:"secret"` //nolint:gosec // G117: this is the declared secret's name, never its value.
	Scope       string `yaml:"scope" json:"scope"`
	Provider    string `yaml:"provider" json:"provider"`
	Status      string `yaml:"status" json:"status"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// secretStatusRows builds status rows across every matching (stack, component) instance. Shared
// secrets are de-duplicated to a single row per storage location, matching `atmos secret list`.
func secretStatusRows(atmosConfig *schema.AtmosConfiguration, entries []secretScopeEntry) []secretStatusRow {
	var rows []secretStatusRow
	seenShared := make(map[string]bool)
	for _, entry := range entries {
		svc := secrets.NewService(atmosConfig, entry.Stack, entry.Component, entry.Section)
		// verify=false: credential-free by design (see SecretListTool doc comment).
		statuses := svc.Status(false)
		for i := range statuses {
			st := &statuses[i]
			switch st.Declaration.Scope {
			case secrets.ScopeGlobal:
				key := "global\x00" + st.Declaration.Name
				if seenShared[key] {
					continue
				}
				seenShared[key] = true
				rows = append(rows, secretStatusRowFrom("*", "*", st))
			case secrets.ScopeStack:
				key := entry.Stack + "\x00" + st.Declaration.Name
				if seenShared[key] {
					continue
				}
				seenShared[key] = true
				rows = append(rows, secretStatusRowFrom(entry.Stack, "*", st))
			default:
				rows = append(rows, secretStatusRowFrom(entry.Stack, entry.Component, st))
			}
		}
	}
	return rows
}

// secretStatusRowFrom converts a single secrets.Status into a row.
func secretStatusRowFrom(stack, component string, st *secrets.Status) secretStatusRow {
	scope := string(st.Declaration.Scope)
	if scope == "" {
		scope = string(secrets.ScopeInstance)
	}
	provider := "(none)"
	if st.Declaration.BackendName != "" {
		provider = string(st.Declaration.BackendType) + ":" + st.Declaration.BackendName
	}
	return secretStatusRow{
		Stack:       stack,
		Component:   component,
		Secret:      st.Declaration.Name,
		Scope:       scope,
		Provider:    provider,
		Status:      secretStatusLabel(st),
		Description: st.Declaration.Description,
	}
}

// secretStatusLabel returns the initialization status text for a secret (never a value).
func secretStatusLabel(st *secrets.Status) string {
	if st.Err != nil {
		return "error"
	}
	if st.Unknown {
		return "unknown"
	}
	if st.Initialized {
		return "initialized"
	}
	return "missing"
}

// buildSecretListResult formats secret status rows into a tools.Result.
func buildSecretListResult(stack, component string, rows []secretStatusRow) *tools.Result {
	var output strings.Builder
	fmt.Fprintf(&output, "Declared Secrets (%d):\n\n", len(rows))
	if len(rows) == 0 {
		output.WriteString("(none)\n")
	}
	for _, r := range rows {
		fmt.Fprintf(&output, "  - [%s/%s] %s (scope=%s, provider=%s, status=%s)\n",
			r.Stack, r.Component, r.Secret, r.Scope, r.Provider, r.Status)
	}

	return &tools.Result{
		Success: true,
		Output:  output.String(),
		Data: map[string]interface{}{
			paramStack:     stack,
			paramComponent: component,
			"secrets":      rows,
		},
	}
}

// RequiresPermission returns true if this tool needs permission.
func (t *SecretListTool) RequiresPermission() bool {
	return false // Read-only, metadata-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *SecretListTool) IsRestricted() bool {
	return false
}
