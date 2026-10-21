package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/projectroot"
	"github.com/cloudposse/atmos/pkg/perf"
)

const maxPolicyDocumentBytes = 1024 * 1024

type planPolicyDocument struct {
	directory  string
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	} `yaml:"metadata"`
	Spec struct {
		Policies map[string]planPolicyItem `yaml:"policies"`
		Hooks    map[string]map[string]any `yaml:"hooks"`
		Tools    map[string]string         `yaml:"tools"`
	} `yaml:"spec"`
}

// Bundle contains governing rules and hook definitions, never workspace hooks.
// Hook payloads use the existing hook schema and are decoded by that executor.
type Bundle struct {
	Rules []Rule
	Hooks map[string]json.RawMessage
	Tools map[string]string
}

// defaultDirectoryName is the policy directory discovered at the project root.
const defaultDirectoryName = "policy.d"

// DirectoryFromEnvironment resolves an explicit override or the default policy.d directory.
// The default is the policy.d next to the configuration that atmos.yaml discovery
// selects, using the same precedence: --config/--config-path (then ATMOS_CONFIG and
// ATMOS_CONFIG_PATH), ATMOS_CLI_CONFIG_PATH, the working directory, then the Git
// root unless ATMOS_GIT_ROOT_BASEPATH=false, then the nearest ancestor holding
// atmos.yaml, which config loading also reads. The first of those directories
// holding a policy.d wins; the working directory is always considered.
// Discovery does not establish trust. A missing default is allowed; an explicit
// missing directory is returned so loading fails rather than falling back.
func DirectoryFromEnvironment() (string, error) {
	defer perf.Track(nil, "policy.DirectoryFromEnvironment")()
	settings := viper.New()
	if err := settings.BindEnv("directory", "ATMOS_POLICY_DIR"); err != nil {
		return "", err
	}
	if directory := settings.GetString("directory"); directory != "" {
		return directory, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("%w: resolve working directory: %w", errUtils.ErrPolicyInvalid, err)
	}
	for _, root := range projectroot.SearchDirs(cwd, os.Args) {
		directory, err := defaultDirectory(filepath.Join(root, defaultDirectoryName))
		if err != nil || directory != "" {
			return directory, err
		}
	}
	return "", nil
}

// defaultDirectory returns candidate when it is an existing directory, and an
// empty path when it does not exist.
func defaultDirectory(candidate string) (string, error) {
	//nolint:gosec // The candidate is a discovered project-root directory; it is only stat-ed here and validated by LoadDirectory.
	info, err := os.Stat(candidate)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: inspect default directory: %w", errUtils.ErrPolicyInvalid, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: default policy.d is not a directory", errUtils.ErrPolicyInvalid)
	}
	return candidate, nil
}

// RulesFor returns rules for one input capability without changing their order.
func (b *Bundle) RulesFor(kind InputKind) []Rule {
	defer perf.Track(nil, "policy.Bundle.RulesFor")()

	var rules []Rule
	for i := range b.Rules {
		if b.Rules[i].On == kind {
			rules = append(rules, b.Rules[i])
		}
	}
	return rules
}

type planPolicyItem struct {
	OPA          *opaPolicyConfig    `yaml:"opa"`
	Schema       *schemaPolicyConfig `yaml:"schema"`
	Files        *FileSelector       `yaml:"files"`
	AllowedRoots []string            `yaml:"allowed_roots"`
	Match        *Selector           `yaml:"match"`
	On           InputKind           `yaml:"on"`
	Engine       string              `yaml:"engine"`
	Rule         string              `yaml:"rule"`
	Action       string              `yaml:"action"`
	Reason       string              `yaml:"reason"`
	With         map[string]any      `yaml:"with"`
	RequiredCI   []CIRequirement     `yaml:"required_ci"`
	Hooks        []HookRequirement   `yaml:"hooks"`
	Approvals    []approvalItem      `yaml:"approvals"`
}

type approvalItem struct {
	ID       string         `yaml:"id"`
	Provider string         `yaml:"provider"`
	Spec     map[string]any `yaml:"spec"`
}

// LoadPlanDirectory loads plan manifests and their local evaluator resources.
// An explicit directory is required. No discovery, imports, templates or trust claims
// are made. Unsupported fields/capabilities are errors, not ignored declarations.
func LoadPlanDirectory(directory string) ([]Rule, error) {
	defer perf.Track(nil, "policy.LoadPlanDirectory")()
	bundle, err := LoadDirectory(directory)
	if err != nil {
		return nil, err
	}
	for i := range bundle.Rules {
		if bundle.Rules[i].On != TerraformPlan {
			return nil, fmt.Errorf("%w: expected terraform.plan rule", errUtils.ErrPolicyInvalid)
		}
	}
	return bundle.Rules, nil
}

// LoadDirectory loads explicit, protected local policy documents. It performs
// no remote imports, template expansion, hook execution or automatic trust discovery.
// OPA modules and JSON Schemas are materialized from within the policy directory.
func LoadDirectory(directory string) (*Bundle, error) {
	defer perf.Track(nil, "policy.LoadDirectory")()

	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("%w: read policy directory: %w", errUtils.ErrPolicyInvalid, err)
	}
	bundle := &Bundle{Hooks: map[string]json.RawMessage{}, Tools: map[string]string{}}
	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}
		if !entry.Type().IsRegular() {
			return nil, fmt.Errorf("%w: policy documents must be regular files", errUtils.ErrPolicyInvalid)
		}
		document, err := readPolicyDocument(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("%w: document %q: %w", errUtils.ErrPolicyInvalid, entry.Name(), err)
		}
		if err := bundle.addDocument(document); err != nil {
			return nil, err
		}
	}
	if err := bundle.validate(); err != nil {
		return nil, err
	}
	return bundle, nil
}

func (b *Bundle) validate() error {
	if len(b.Rules) == 0 {
		return fmt.Errorf("%w: configured directory contains no policies", errUtils.ErrPolicyInvalid)
	}
	if err := validateRuleIdentities(b.Rules); err != nil {
		return err
	}
	for i := range b.Rules {
		for _, hook := range b.Rules[i].Effects.Hooks {
			if _, exists := b.Hooks[hook.Ref]; !exists {
				return fmt.Errorf("%w: missing governing hook %s", errUtils.ErrPolicyInvalid, hook.Ref)
			}
		}
	}
	return nil
}

func (b *Bundle) addDocument(document *planPolicyDocument) error {
	loaded, err := document.rules()
	if err != nil {
		return err
	}
	b.Rules = append(b.Rules, loaded...)
	for name, definition := range document.Spec.Hooks {
		id := document.Metadata.Name + "/" + name
		if _, exists := b.Hooks[id]; exists {
			return fmt.Errorf("%w: duplicate hook %s", errUtils.ErrPolicyInvalid, id)
		}
		data, err := json.Marshal(definition)
		if err != nil {
			return fmt.Errorf("%w: invalid hook %s", errUtils.ErrPolicyInvalid, id)
		}
		b.Hooks[id] = data
	}
	for name, version := range document.Spec.Tools {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
			return fmt.Errorf("%w: tool names and versions must be nonempty", errUtils.ErrPolicyInvalid)
		}
		if previous, exists := b.Tools[name]; exists && previous != version {
			return fmt.Errorf("%w: conflicting tool pin %s", errUtils.ErrPolicyInvalid, name)
		}
		b.Tools[name] = version
	}
	return nil
}

func readPlanPolicyDocument(path string) ([]Rule, error) {
	document, err := readPolicyDocument(path)
	if err != nil {
		return nil, err
	}
	return document.rules()
}

func readPolicyDocument(path string) (*planPolicyDocument, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPolicyDocumentBytes+1))
	if err != nil || len(data) > maxPolicyDocumentBytes {
		return nil, fmt.Errorf("%w: policy document unreadable or too large", errUtils.ErrPolicyInvalid)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var document planPolicyDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%w: decode document: %w", errUtils.ErrPolicyInvalid, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: exactly one YAML document per file is required", errUtils.ErrPolicyInvalid)
	}
	document.directory = filepath.Dir(path)
	return &document, nil
}

func (d *planPolicyDocument) rules() ([]Rule, error) {
	if d.APIVersion != "atmos/v1" || d.Kind != "Policy" || strings.TrimSpace(d.Metadata.Name) == "" || len(d.Spec.Policies) == 0 {
		return nil, fmt.Errorf("%w: expected atmos/v1 Policy with metadata.name and spec.policies", errUtils.ErrPolicyInvalid)
	}
	names := make([]string, 0, len(d.Spec.Policies))
	for name := range d.Spec.Policies {
		names = append(names, name)
	}
	sort.Strings(names)
	rules := make([]Rule, 0, len(names))
	for _, name := range names {
		item := d.Spec.Policies[name]
		rule, err := item.resolve(d.Metadata.Name, name)
		if err != nil {
			return nil, err
		}
		rule.AllowedRoots, err = resolveRoots(item.AllowedRoots, d.directory)
		if err != nil {
			return nil, err
		}
		if err := item.resolveEvaluator(d.directory, &rule); err != nil {
			return nil, fmt.Errorf("%w: rule %s: %w", errUtils.ErrPolicyInvalid, rule.ID, err)
		}
		rule.Description = d.Metadata.Description
		rules = append(rules, rule)
	}
	return rules, nil
}

func (p *planPolicyItem) resolve(document, name string) (Rule, error) {
	for _, requirement := range p.RequiredCI {
		if requirement.Provider == "" || requirement.Schema == "" {
			return Rule{}, fmt.Errorf("%w: required_ci needs provider and schema", errUtils.ErrPolicyInvalid)
		}
		if _, err := requiredDataset(&CISnapshot{}, requirement); err != nil {
			return Rule{}, err
		}
	}
	if err := p.validateEffects(name); err != nil {
		return Rule{}, err
	}
	if err := p.resolveHookReferences(document); err != nil {
		return Rule{}, err
	}
	return p.resolveExpression(document, name)
}

func (p *planPolicyItem) validateEffects(name string) error {
	if len(p.AllowedRoots) > 0 && p.On != Source {
		return fmt.Errorf("%w: allowed_roots requires on: source", errUtils.ErrPolicyInvalid)
	}
	if strings.TrimSpace(name) == "" || !slices.Contains([]InputKind{TerraformPlan, Operation, AtmosAffected, Function, Command, ComponentCommand, Source, File}, p.On) || !slices.Contains([]string{"deny", "hooks", "require_approval"}, p.Action) {
		return fmt.Errorf("%w: unsupported policy input kind or action", errUtils.ErrPolicyInvalid)
	}
	if slices.Contains([]InputKind{AtmosAffected, Function, Command, Source, File}, p.On) && p.Match != nil {
		return fmt.Errorf("%w: this input has no instance selector; use component.command for instance admission", errUtils.ErrPolicyInvalid)
	}
	if (p.Action == "require_approval") != (len(p.Approvals) > 0) {
		return fmt.Errorf("%w: approvals require action: require_approval and at least one requirement", errUtils.ErrPolicyInvalid)
	}
	if err := validateFileSelector(p.On, p.Files); err != nil {
		return err
	}
	return p.validateHookEffects()
}

func (p *planPolicyItem) validateHookEffects() error {
	if p.Action == "hooks" && (p.On != Operation || len(p.Hooks) == 0) || p.Action != "hooks" && len(p.Hooks) != 0 {
		return fmt.Errorf("%w: hook effects require an operation rule with action: hooks", errUtils.ErrPolicyInvalid)
	}
	return nil
}

func (p *planPolicyItem) resolveHookReferences(document string) error {
	for i := range p.Hooks {
		if p.Hooks[i].Ref == "" || p.Hooks[i].Event == "" {
			return fmt.Errorf("%w: hook ref and event required", errUtils.ErrPolicyInvalid)
		}
		if !strings.Contains(p.Hooks[i].Ref, "/") {
			p.Hooks[i].Ref = document + "/" + p.Hooks[i].Ref
		}
	}
	return nil
}

func (p *planPolicyItem) resolveExpression(document, name string) (Rule, error) {
	if err := p.Match.validate(); err != nil {
		return Rule{}, err
	}
	if p.Engine == "" {
		p.Engine = CELEngine
	}
	if err := validateExpression(p.Engine, p.Rule); err != nil {
		return Rule{}, err
	}
	parameters := json.RawMessage(`{}`)
	if p.With != nil {
		var err error
		parameters, err = json.Marshal(p.With)
		if err != nil {
			return Rule{}, fmt.Errorf("%w: parameters must be JSON-compatible", errUtils.ErrPolicyInvalid)
		}
	}
	var approvals []ApprovalRequirement
	for _, item := range p.Approvals {
		data, err := json.Marshal(item.Spec)
		if err != nil {
			return Rule{}, fmt.Errorf("%w: invalid approval spec", errUtils.ErrPolicyInvalid)
		}
		requirement := ApprovalRequirement{ID: item.ID, Provider: item.Provider, Spec: data}
		if _, err := ParseGitHubApproval(requirement); err != nil {
			return Rule{}, err
		}
		approvals = append(approvals, requirement)
	}
	return Rule{
		ID: document + "/" + name, On: p.On, Engine: p.Engine, Expression: p.Rule, Match: p.Match,
		Files: p.Files, Parameters: parameters, RequiredCI: p.RequiredCI, Effects: Effects{Deny: p.Action == "deny", Reason: p.Reason, Hooks: p.Hooks, Approvals: approvals},
	}, nil
}

func validateExpression(engine, expression string) error {
	if len(expression) > maxExpressionBytes {
		return fmt.Errorf("%w: expression exceeds size limit", errUtils.ErrPolicyInvalid)
	}
	var err error
	switch engine {
	case CELEngine:
		_, err = compileCEL(expression)
	case StarlarkEngine:
		_, err = compileStarlark(expression, nil)
	case OPAEngine, JSONSchemaEngine:
		// Resources are compiled after resolving their document-relative paths.
		if expression != "" {
			return fmt.Errorf("%w: use opa or schema configuration instead of rule", errUtils.ErrPolicyInvalid)
		}
	default:
		return fmt.Errorf("%w: supported engines are cel, starlark, opa, and jsonschema", errUtils.ErrPolicyEngineUnknown)
	}
	return err
}
