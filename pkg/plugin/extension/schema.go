package extension

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
)

const schemaOrigin = "https://plugin-schema.atmos.invalid/"

var credentialType = regexp.MustCompile(`^[a-z][a-z0-9.-]*/v[1-9][0-9]*$`)

// CompileSchema resolves references only within the package; validation never fetches the network.
func CompileSchema(directory, path string) (*jsonschema.Schema, error) {
	defer perf.Track(nil, "extension.CompileSchema")()

	if !manifest.LocalPath(path) {
		return nil, invalidMetadata(fmt.Sprintf("Schema path %q must be relative and stay inside the plugin directory.", path),
			"Use a relative path such as `schemas/config.schema.json`.")
	}
	compiler := jsonschema.NewCompiler()
	compiler.LoadURL = func(location string) (io.ReadCloser, error) {
		if !strings.HasPrefix(location, schemaOrigin) {
			return nil, invalidMetadata("A schema references a location outside the plugin package; validation never fetches the network.",
				"Keep every `$ref` pointed at a file inside the plugin directory.")
		}
		parsed, err := url.Parse(location)
		if err != nil {
			return nil, invalidMetadata("A schema reference is not a valid location.", "Use relative file paths in `$ref`.")
		}
		raw, err := manifest.ReadFile(directory, strings.TrimPrefix(parsed.Path, "/"))
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(raw)), nil
	}
	result, err := compiler.Compile(schemaOrigin + path)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	return result, nil
}

// ValidateAuth verifies the package's configuration schema and credential declarations.
func ValidateAuth(binding Binding) error {
	defer perf.Track(nil, "extension.ValidateAuth")()

	schema, ok := binding.Metadata["schema"].(string)
	if !ok {
		return invalidMetadata("The export has no `schema` path.", "Set `schema:` to a JSON Schema file inside the plugin, for example `schemas/config.schema.json`.")
	}
	if _, err := CompileSchema(binding.Directory, schema); err != nil {
		return err
	}
	outputs, ok := binding.Metadata["credentials"].([]any)
	if !ok || len(outputs) == 0 {
		return invalidMetadata("The export declares no `credentials`.", "List the credential types it returns, for example `credentials: [oidc/v1]`.")
	}
	for _, output := range outputs {
		if name, ok := output.(string); !ok || !credentialType.MatchString(name) {
			return invalidMetadata(fmt.Sprintf("Credential type %v is not a versioned name.", output), "Use `<type>/v<N>`, for example `oidc/v1`.")
		}
	}
	return nil
}

// ValidateCommand checks command metadata without running its executable.
func ValidateCommand(binding Binding) error {
	defer perf.Track(nil, "extension.ValidateCommand")()

	description, ok := binding.Metadata["description"].(string)
	if !ok || strings.TrimSpace(description) == "" {
		return invalidMetadata("The command has no `description`.", "Add a short `description:` explaining what the command does.")
	}
	if path, exists := binding.Metadata["schema"]; exists {
		name, ok := path.(string)
		if !ok {
			return invalidMetadata("The command `schema` is not a string path.", "Set `schema:` to a JSON Schema file path inside the plugin.")
		}
		_, err := CompileSchema(binding.Directory, name)
		return err
	}
	return nil
}

// invalidMetadata reports a plugin.yaml export that fails static validation.
func invalidMetadata(explanation, hint string) error {
	return errUtils.Build(errUtils.ErrPluginInvalidConfig).WithExplanation(explanation).WithHint(hint).Err()
}
