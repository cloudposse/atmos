package terraform_backend

import (
	"encoding/json"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// GetTerraformWorkspace returns the `workspace` section for a component in a stack.
func GetTerraformWorkspace(sections *map[string]any) string {
	defer perf.Track(nil, "terraform_backend.GetTerraformWorkspace")()

	if workspace, ok := (*sections)[cfg.WorkspaceSectionName].(string); ok {
		return workspace
	}
	return ""
}

// ComponentEnvKeysAWS is the whitelist of env vars that in-process AWS backend
// readers honor from a component's `env` section. Mirrors the credential- and
// endpoint-relevant keys tofu's S3 backend reads from the process environment,
// so the in-process reader matches subprocess behavior for `!terraform.state`.
//
// Symmetric with the env overlay that `!terraform.output` already applies via
// `pkg/terraform/output/environment.go::SetupEnvironment` (the loop that
// writes `config.Env` over the subprocess environment).
//
// AWS_STS_REGIONAL_ENDPOINTS is intentionally NOT in this list — it was a v1
// SDK toggle; SDK v2 always uses regional endpoints by default and the legacy
// toggle is a no-op. Including it would silently fail to honor.
var ComponentEnvKeysAWS = []string{
	"AWS_PROFILE",
	"AWS_REGION",
	"AWS_DEFAULT_REGION",
	"AWS_CONFIG_FILE",
	"AWS_SHARED_CREDENTIALS_FILE",
	"AWS_ENDPOINT_URL_S3",
	"AWS_ENDPOINT_URL_STS",
	"AWS_USE_FIPS_ENDPOINT",
}

// ExtractComponentEnvOverlay returns a whitelisted overlay of the component's
// `env` section. Keys outside the whitelist are ignored — only credential- and
// endpoint-related env vars are exposed to in-process backend clients.
//
// Returns nil when no relevant keys are present so callers preserve their
// existing default-credential-chain behavior unchanged. This is the
// backward-compatibility hinge: components that don't set any whitelisted key
// see no observable difference.
func ExtractComponentEnvOverlay(sections *map[string]any, whitelist []string) map[string]string {
	defer perf.Track(nil, "terraform_backend.ExtractComponentEnvOverlay")()

	if sections == nil {
		return nil
	}

	envSection, ok := (*sections)[cfg.EnvSectionName].(map[string]any)
	if !ok || len(envSection) == 0 {
		return nil
	}
	overlay := make(map[string]string, len(whitelist))
	for _, k := range whitelist {
		if v, present := envSection[k]; present {
			overlay[k] = fmt.Sprintf("%v", v)
		}
	}
	if len(overlay) == 0 {
		return nil
	}
	return overlay
}

// GetTerraformComponent returns the `component` section for a component in a stack.
func GetTerraformComponent(sections *map[string]any) string {
	defer perf.Track(nil, "terraform_backend.GetTerraformComponent")()

	if workspace, ok := (*sections)[cfg.ComponentSectionName].(string); ok {
		return workspace
	}
	return ""
}

// GetComponentBackend returns the `backend` section for a component in a stack.
func GetComponentBackend(sections *map[string]any) map[string]any {
	defer perf.Track(nil, "terraform_backend.GetComponentBackend")()

	if remoteStateBackend, ok := (*sections)[cfg.BackendSectionName].(map[string]any); ok {
		return remoteStateBackend
	}
	return nil
}

// GetComponentBackendType returns the `backend_type` section for a component in a stack.
func GetComponentBackendType(sections *map[string]any) string {
	defer perf.Track(nil, "terraform_backend.GetComponentBackendType")()

	if backendType, ok := (*sections)[cfg.BackendTypeSectionName].(string); ok {
		return backendType
	}
	return ""
}

// GetBackendAttribute returns an attribute from a section in the backend.
func GetBackendAttribute(section *map[string]any, attribute string) string {
	defer perf.Track(nil, "terraform_backend.GetBackendAttribute")()

	if i, ok := (*section)[attribute].(string); ok {
		return i
	}
	return ""
}

// GetTerraformBackendVariable returns the output from the configured backend.
func GetTerraformBackendVariable(
	atmosConfig *schema.AtmosConfiguration,
	values map[string]any,
	variable string,
) (any, error) {
	defer perf.Track(atmosConfig, "terraform_backend.GetTerraformBackendVariable")()

	val := variable
	if !strings.HasPrefix(variable, ".") {
		val = "." + val
	}

	res, err := u.EvaluateYqExpression(atmosConfig, values, val)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// RawTerraformState represents a raw Terraform state file.
type RawTerraformState struct {
	Version          int    `json:"version"`           // Internal format version
	TerraformVersion string `json:"terraform_version"` // CLI version used
	Outputs          map[string]struct {
		Value any `json:"value"` // Can be any JSON type
		Type  any `json:"type"`  // HCL type representation
	} `json:"outputs"`
	Resources interface{} `json:"resources,omitempty"`
}

// ProcessTerraformStateFile processes a Terraform state file.
func ProcessTerraformStateFile(data []byte) (map[string]any, error) {
	defer perf.Track(nil, "terraform_backend.ProcessTerraformStateFile")()

	if len(data) == 0 {
		return nil, nil
	}

	var rawState RawTerraformState
	if err := json.Unmarshal(data, &rawState); err != nil {
		return nil, err
	}

	rawOutputs := rawState.Outputs
	result := make(map[string]any, len(rawOutputs))

	for key, output := range rawOutputs {
		result[key] = output.Value
	}

	return result, nil
}

// GetTerraformBackend reads and processes the Terraform state file from the configured backend.
func GetTerraformBackend(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
	authContext *schema.AuthContext,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "terraform_backend.GetTerraformBackend")()

	RegisterTerraformBackends()

	backendType := GetComponentBackendType(componentSections)
	if backendType == "" {
		backendType = cfg.BackendTypeLocal
	}

	readBackendStateFunc := GetTerraformBackendReadFunc(backendType)
	if readBackendStateFunc == nil {
		return nil, fmt.Errorf("%w: `%s`\nsupported backends: `local`, `s3`, `gcs`, `azurerm`", errUtils.ErrUnsupportedBackendType, backendType)
	}

	content, err := readBackendStateFunc(atmosConfig, componentSections, authContext)
	if err != nil {
		return nil, err
	}

	data, err := ProcessTerraformStateFile(content)
	if err != nil {
		return nil, fmt.Errorf("%w\n%v", errUtils.ErrProcessTerraformStateFile, err)
	}

	return data, nil
}
