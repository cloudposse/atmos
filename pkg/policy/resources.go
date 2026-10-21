package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errUtils "github.com/cloudposse/atmos/errors"
)

type opaPolicyConfig struct {
	Query       string   `yaml:"query"`
	Modules     []string `yaml:"modules"`
	RegoVersion int      `yaml:"rego_version"`
}

type schemaPolicyConfig struct {
	Path string `yaml:"path"`
}

// resolveEvaluator materializes sources while loading the governing bundle.
func (p *planPolicyItem) resolveEvaluator(directory string, rule *Rule) error {
	if err := p.validateEvaluatorFields(rule.Engine); err != nil {
		return err
	}
	switch rule.Engine {
	case OPAEngine:
		config, err := loadOPAResources(directory, p.OPA)
		if err != nil {
			return err
		}
		rule.OPA = config
	case JSONSchemaEngine:
		config, err := loadSchemaResources(directory, p.Schema.Path)
		if err != nil {
			return err
		}
		rule.Schema = config
	}
	return nil
}

// readLocalResource confines reads to a bundle or file-check root, including symlinks.
func readLocalResource(directory, name string, limit int64) ([]byte, error) {
	if !filepath.IsLocal(name) || strings.Contains(name, "\\") {
		return nil, fmt.Errorf("%w: resource must be a relative path within its root: %q", errUtils.ErrPolicyInvalid, name)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("%w: open resource root: %w", errUtils.ErrPolicyInvalid, err)
	}
	defer root.Close()
	info, err := root.Stat(name)
	if err != nil {
		return nil, fmt.Errorf("%w: resource %q: %w", errUtils.ErrPolicyInvalid, name, err)
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("%w: resource %q must be a regular file within the size limit", errUtils.ErrPolicyInvalid, name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("%w: resource %q: %w", errUtils.ErrPolicyInvalid, name, err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, fmt.Errorf("%w: resource %q unreadable or too large", errUtils.ErrPolicyInvalid, name)
	}
	return raw, nil
}

// A private URI base lets relative references resolve without enabling URL retrieval.
const schemaBase = "https://atmos-policy.invalid/"

func loadSchemaResources(directory, name string) (*SchemaConfig, error) {
	if !filepath.IsLocal(name) || strings.Contains(name, "\\") {
		return nil, fmt.Errorf("%w: schema.path must stay inside the policy directory", errUtils.ErrPolicyInvalid)
	}
	entry := (&url.URL{Scheme: "https", Host: "atmos-policy.invalid", Path: "/" + filepath.ToSlash(filepath.Clean(name))}).String()
	config := &SchemaConfig{Entry: entry, Resources: map[string]json.RawMessage{}}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.LoadURL = func(uri string) (io.ReadCloser, error) {
		return config.loadResource(directory, uri)
	}
	if _, err := compiler.Compile(entry); err != nil {
		return nil, fmt.Errorf("%w: compile JSON Schema: %w", errUtils.ErrPolicyInvalid, err)
	}
	return config, nil
}

func loadOPAResources(directory string, source *opaPolicyConfig) (*OPAConfig, error) {
	config := &OPAConfig{Query: source.Query, RegoVersion: source.RegoVersion, Modules: map[string]string{}}
	total := 0
	for _, name := range source.Modules {
		raw, err := readLocalResource(directory, name, maxPolicyDocumentBytes)
		if err != nil {
			return nil, err
		}
		total += len(raw)
		if total > maxPolicyDocumentBytes {
			return nil, fmt.Errorf("%w: OPA modules exceed size limit", errUtils.ErrPolicyInvalid)
		}
		config.Modules[name] = string(raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), opaTimeout)
	defer cancel()
	if _, err := prepareOPA(ctx, config, nil); err != nil {
		return nil, err
	}
	return config, nil
}

func (config *SchemaConfig) loadResource(directory, uri string) (io.ReadCloser, error) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "atmos-policy.invalid" || parsed.RawQuery != "" || parsed.User != nil {
		return nil, fmt.Errorf("%w: schema references must resolve within the policy directory: %q", errUtils.ErrPolicyInvalid, uri)
	}
	raw, err := readLocalResource(directory, strings.TrimPrefix(parsed.Path, "/"), maxPolicyDocumentBytes)
	if err != nil {
		return nil, err
	}
	total := len(raw)
	for _, existing := range config.Resources {
		total += len(existing)
	}
	if total > maxPolicyDocumentBytes {
		return nil, fmt.Errorf("%w: schema resources exceed size limit", errUtils.ErrPolicyInvalid)
	}
	config.Resources[uri] = raw
	return io.NopCloser(bytes.NewReader(raw)), nil
}

func (p *planPolicyItem) validateEvaluatorFields(engine string) error {
	if (engine == OPAEngine) != (p.OPA != nil) {
		return fmt.Errorf("%w: opa configuration is required only for engine opa", errUtils.ErrPolicyInvalid)
	}
	if (engine == JSONSchemaEngine) != (p.Schema != nil) {
		return fmt.Errorf("%w: schema configuration is required only for engine jsonschema", errUtils.ErrPolicyInvalid)
	}
	if (p.OPA != nil || p.Schema != nil) && p.Rule != "" {
		return fmt.Errorf("%w: use opa or schema configuration instead of rule", errUtils.ErrPolicyInvalid)
	}
	return nil
}
