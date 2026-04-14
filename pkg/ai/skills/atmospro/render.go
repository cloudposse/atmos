// Package atmospro implements the template renderer for the atmos-pro skill.
//
// Templates live at agent-skills/skills/atmos-pro/templates/ and use Go text/template
// with custom delimiters "<<" / ">>" to avoid collisions with native {{ }} syntax
// that appears literally in Atmos Pro workflow-dispatch inputs, GitHub Actions
// expressions, and Atmos vendor manifests.
//
// See agent-skills/skills/atmos-pro/templates/README.md for the placeholder contract.
package atmospro

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Static sentinel errors so callers can match with errors.Is.
var (
	// ErrOrgRequired is returned when RenderData.Org is empty.
	ErrOrgRequired = errors.New("atmospro: Org is required")
	// ErrRepoRequired is returned when RenderData.Repo is empty.
	ErrRepoRequired = errors.New("atmospro: Repo is required")
	// ErrNamespaceRequired is returned when RenderData.Namespace is empty.
	ErrNamespaceRequired = errors.New("atmospro: Namespace is required")
	// ErrAccountsRequired is returned when RenderData.Accounts is empty.
	ErrAccountsRequired = errors.New("atmospro: at least one account is required")
	// ErrAccountFieldMissing is returned when an account is missing tenant, stage, or account_id.
	ErrAccountFieldMissing = errors.New("atmospro: account is missing tenant, stage, or account_id")
	// ErrMultipleRoots is returned when more than one account has IsRoot set.
	ErrMultipleRoots = errors.New("atmospro: more than one root account in Accounts")
)

// Account describes one target AWS account derived from the stack hierarchy.
type Account struct {
	Tenant    string `json:"tenant" yaml:"tenant"`
	Stage     string `json:"stage" yaml:"stage"`
	AccountID string `json:"account_id" yaml:"account_id"`
	// IsRoot marks the root account. The apply profile pins root-account
	// identities to the plan role's ARN as a safety rail.
	IsRoot bool `json:"is_root" yaml:"is_root"`
}

// RenderData is the context passed to every template.
type RenderData struct {
	// Repository identity.
	Org  string `json:"org"`
	Repo string `json:"repo"`

	// Deployment target.
	Namespace     string `json:"namespace"`
	TargetOrg     string `json:"target_org"`
	RootAccountID string `json:"root_account_id"`

	// Discovered accounts. Ordering is stable (alphabetical by tenant, then stage).
	Accounts []Account `json:"accounts"`

	// Values for the OIDC test workflow (one specific account used as the probe).
	ProbeStack     string `json:"probe_stack"`
	ProbeTenant    string `json:"probe_tenant"`
	ProbeStage     string `json:"probe_stage"`
	ProbeAccountID string `json:"probe_account_id"`

	// Starting-condition flags. Drive variant sections in generated docs.
	GeodesicDetected    bool `json:"geodesic_detected"`
	SpaceliftWasEnabled bool `json:"spacelift_was_enabled"`
	NoAtmosAuth         bool `json:"no_atmos_auth"`
}

// Validate ensures RenderData has the minimum fields needed to render all templates.
// It does not enforce business rules (e.g., that Root is actually the root account);
// those are callers' responsibility.
func (d *RenderData) Validate() error {
	if err := d.validateScalars(); err != nil {
		return err
	}
	return d.validateAccounts()
}

// validateScalars checks the top-level string fields.
func (d *RenderData) validateScalars() error {
	if d.Org == "" {
		return ErrOrgRequired
	}
	if d.Repo == "" {
		return ErrRepoRequired
	}
	if d.Namespace == "" {
		return ErrNamespaceRequired
	}
	return nil
}

// validateAccounts checks the Accounts slice for required fields and at-most-one root.
func (d *RenderData) validateAccounts() error {
	if len(d.Accounts) == 0 {
		return ErrAccountsRequired
	}
	rootCount := 0
	for _, a := range d.Accounts {
		if a.Tenant == "" || a.Stage == "" || a.AccountID == "" {
			return fmt.Errorf("%w: %q-%q", ErrAccountFieldMissing, a.Tenant, a.Stage)
		}
		if a.IsRoot {
			rootCount++
		}
	}
	if rootCount > 1 {
		return ErrMultipleRoots
	}
	return nil
}

// Render executes the given template source with data and returns the result.
// Templates use "<<" and ">>" as delimiters so that literal {{ }} passes through.
func Render(name, source string, data *RenderData) (string, error) {
	defer perf.Track(nil, "atmospro.Render")()

	if err := data.Validate(); err != nil {
		return "", err
	}

	tmpl, err := template.New(name).
		Delims("<<", ">>").
		Parse(source)
	if err != nil {
		return "", fmt.Errorf("atmospro: parse %q: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateContext(data)); err != nil {
		return "", fmt.Errorf("atmospro: execute %q: %w", name, err)
	}
	return normalizeTrailingNewline(buf.String()), nil
}

// normalizeTrailingNewline ensures the rendered output ends with exactly one
// newline, matching POSIX text-file convention and the repo's end-of-file-fixer
// pre-commit hook. Templates with variable iteration counts can leave 0, 1, or
// 2+ trailing newlines depending on directive trimming; normalizing here keeps
// the renderer's output stable regardless.
func normalizeTrailingNewline(s string) string {
	if s == "" {
		return s
	}
	s = strings.TrimRight(s, "\n")
	return s + "\n"
}

// templateContext converts RenderData into a lowercase-keyed map so templates
// can use "<<.org>>" instead of "<<.Org>>". This matches the visual convention
// of Atmos's own YAML config.
func templateContext(d *RenderData) map[string]any {
	accounts := make([]map[string]any, 0, len(d.Accounts))
	for _, a := range d.Accounts {
		accounts = append(accounts, map[string]any{
			"tenant":     a.Tenant,
			"stage":      a.Stage,
			"account_id": a.AccountID,
			"is_root":    a.IsRoot,
		})
	}
	return map[string]any{
		"org":                   d.Org,
		"repo":                  d.Repo,
		"namespace":             d.Namespace,
		"target_org":            d.TargetOrg,
		"root_account_id":       d.RootAccountID,
		"accounts":              accounts,
		"probe_stack":           d.ProbeStack,
		"probe_tenant":          d.ProbeTenant,
		"probe_stage":           d.ProbeStage,
		"probe_account_id":      d.ProbeAccountID,
		"geodesic_detected":     d.GeodesicDetected,
		"spacelift_was_enabled": d.SpaceliftWasEnabled,
		"no_atmos_auth":         d.NoAtmosAuth,
	}
}

// templateToOutputPath maps a template path (relative to templates/) to its
// final output path (relative to repo root). Templates not in this map are
// copied verbatim to the same relative path (without the .tmpl suffix).
var templateToOutputPath = map[string]string{
	"mixins/atmos-pro.yaml.tmpl":                   "stacks/mixins/atmos-pro.yaml",
	"catalog/iam-role-defaults.yaml.tmpl":          "stacks/catalog/aws/iam-role/defaults.yaml",
	"catalog/iam-role-gha-tf.yaml.tmpl":            "stacks/catalog/aws/iam-role/gha-tf.yaml",
	"component/iam-role-component.yaml.tmpl":       "components/terraform/aws/iam-role/component.yaml",
	"profiles/github-plan.yaml.tmpl":               "profiles/github-plan/atmos.yaml",
	"profiles/github-apply.yaml.tmpl":              "profiles/github-apply/atmos.yaml",
	"workflows/atmos-pro.yaml.tmpl":                ".github/workflows/atmos-pro.yaml",
	"workflows/atmos-pro-list-instances.yaml.tmpl": ".github/workflows/atmos-pro-list-instances.yaml",
	"workflows/atmos-terraform-plan.yaml.tmpl":     ".github/workflows/atmos-terraform-plan.yaml",
	"workflows/atmos-terraform-apply.yaml.tmpl":    ".github/workflows/atmos-terraform-apply.yaml",
	"workflows/oidc-test.yaml.tmpl":                ".github/workflows/oidc-test.yaml",
	"docs/atmos-pro.md.tmpl":                       "docs/atmos-pro.md",
	"docs/atmos-pro-pr-body.md.tmpl":               ".github/PULL_REQUEST_BODY.md",
	// tfstate-backend-edit.yaml.tmpl is a merge fragment, not a full file.
	// It has no direct output path; the caller merges it into an existing file.
	"tfstate-backend-edit.yaml.tmpl": "",
}

// OutputPathFor returns the final output path for a given template path.
// Returns an empty string for merge-fragment templates (e.g., tfstate-backend-edit).
func OutputPathFor(templatePath string) string {
	if out, ok := templateToOutputPath[templatePath]; ok {
		return out
	}
	// Fallback: strip .tmpl suffix.
	return strings.TrimSuffix(templatePath, ".tmpl")
}

// RenderAll walks a templates directory (typically embedded via fs.FS), renders
// every *.tmpl file against data, and returns a map of output-path → contents.
// The merge-fragment templates (those with empty output paths in templateToOutputPath)
// are keyed under "_fragments/<original-path>" so callers can identify them.
func RenderAll(templatesFS fs.FS, data *RenderData) (map[string]string, error) {
	defer perf.Track(nil, "atmospro.RenderAll")()

	if err := data.Validate(); err != nil {
		return nil, err
	}

	out := make(map[string]string)
	err := fs.WalkDir(templatesFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".tmpl" {
			return nil
		}
		source, err := fs.ReadFile(templatesFS, path)
		if err != nil {
			return fmt.Errorf("atmospro: read %q: %w", path, err)
		}
		rendered, err := Render(path, string(source), data)
		if err != nil {
			return err
		}
		outPath := OutputPathFor(path)
		if outPath == "" {
			outPath = "_fragments/" + path
		}
		out[outPath] = rendered
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SortedKeys returns the output paths from a RenderAll result in deterministic order.
// Useful for diff-stable snapshots.
func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
