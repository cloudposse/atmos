package atmos

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/auth"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// paramIdentity is the optional identity-name parameter shared by auth tools.
const paramIdentity = "identity"

// AuthWhoamiTool reports the currently active Atmos authentication identity.
type AuthWhoamiTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewAuthWhoamiTool creates a new auth whoami tool.
func NewAuthWhoamiTool(atmosConfig *schema.AtmosConfiguration) *AuthWhoamiTool {
	return &AuthWhoamiTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *AuthWhoamiTool) Name() string {
	return "atmos_auth_whoami"
}

// Description returns the tool description.
func (t *AuthWhoamiTool) Description() string {
	return "Show the currently active Atmos authentication identity and its credential status. " +
		"Read-only: uses cached credentials or non-interactive chain resolution, never triggers " +
		"an interactive login (SSO prompts, browser flows, etc.), and never returns raw credential material."
}

// Parameters returns the tool parameters.
func (t *AuthWhoamiTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramIdentity,
			Description: "Identity name to inspect. Defaults to the configured default identity when omitted.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute runs the tool.
func (t *AuthWhoamiTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	identityName, _ := params[paramIdentity].(string)

	if t.atmosConfig == nil {
		err := fmt.Errorf("%w: atmos configuration is not loaded", errUtils.ErrAIInvalidConfiguration)
		return &tools.Result{Success: false, Error: err}, err
	}

	authManager, err := auth.NewDefaultManager(&t.atmosConfig.Auth, t.atmosConfig.CliConfigPath)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	if identityName == "" {
		identityName, err = authManager.GetDefaultIdentity(false)
		if err != nil {
			return &tools.Result{Success: false, Error: err}, err
		}
	}

	whoami, err := authManager.Whoami(ctx, identityName)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	return buildAuthWhoamiResult(whoami), nil
}

// buildAuthWhoamiResult formats the whoami info into a tools.Result. Credential material is never
// included: WhoamiInfo.Credentials is tagged `json:"-" yaml:"-"` so it cannot leak via marshaling,
// and this function only copies specific, non-sensitive fields into the returned data map.
func buildAuthWhoamiResult(whoami *authTypes.WhoamiInfo) *tools.Result {
	data := map[string]interface{}{
		"identity":     whoami.Identity,
		"provider":     whoami.Provider,
		"valid":        whoamiCredentialsValid(whoami),
		"last_updated": whoami.LastUpdated,
	}
	if whoami.Realm != "" {
		data["realm"] = whoami.Realm
	}
	if whoami.Principal != "" {
		data["principal"] = whoami.Principal
	}
	if whoami.Account != "" {
		data["account"] = whoami.Account
	}
	if whoami.Region != "" {
		data["region"] = whoami.Region
	}
	if whoami.Expiration != nil {
		// Dereference: gopkg.in/yaml.v3 cannot marshal a *time.Time value stored in a
		// map[string]interface{} (it type-asserts to time.Time internally and panics on the pointer).
		data["expiration"] = *whoami.Expiration
	}

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		// Graceful fallback: a YAML marshal error doesn't fail the tool.
		return &tools.Result{
			Success: true,
			Output:  fmt.Sprintf("Current Authentication Status:\n%+v", data),
			Data:    data,
		}
	}

	return &tools.Result{
		Success: true,
		Output:  fmt.Sprintf("Current Authentication Status:\n\n%s", string(yamlBytes)),
		Data:    data,
	}
}

// whoamiCredentialsValid reports whether the resolved credentials are present and unexpired. This
// is a local, network-free check (IsExpired) rather than an authenticated backend call
// (ICredentials.Validate), so the tool never makes an outbound API call as a side effect.
func whoamiCredentialsValid(whoami *authTypes.WhoamiInfo) bool {
	if whoami == nil || whoami.Credentials == nil {
		return false
	}
	return !whoami.Credentials.IsExpired()
}

// RequiresPermission returns true if this tool needs permission.
func (t *AuthWhoamiTool) RequiresPermission() bool {
	return false // Read-only operation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *AuthWhoamiTool) IsRestricted() bool {
	return false
}
