package azure

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Environment variable consulted for a non-interactive activation justification
	// (CI, `atmos auth exec`, MCP startup). Read through viper's ATMOS_ env binding.
	pimJustificationEnvVar   = "ATMOS_PIM_JUSTIFICATION"
	pimJustificationViperKey = "pim_justification"

	// Principal config field keys for an azure/pim-role identity.
	principalRoleDefinitionIDKey = "role_definition_id"
	principalScopeKey            = "scope"
	principalDurationKey         = "duration"
	principalJustificationKey    = "justification"

	// Defaults that bound the wait for a pending activation (for example awaiting an
	// approver): ~10 minutes by default.
	defaultPIMPollInterval    = 10 * time.Second
	defaultPIMMaxPollAttempts = 60
	subscriptionScopeSegments = 2 // "", "subscriptions" before the id in /subscriptions/{id}.

	// Structured-log key for a PIM request resource name.
	logKeyRequest = "request"
)

// pimRoleIdentity activates a PIM-eligible Azure resource role for the principal
// established by the parent identity. It mints no new credentials: the activation
// elevates the existing principal server-side, so Authenticate returns the parent
// credentials unchanged with the role now active.
type pimRoleIdentity struct {
	name             string
	config           *schema.Identity
	roleDefinitionID string
	scope            string
	duration         string // Raw Go-style duration from config; converted to ISO-8601 at request time.
	justification    string // Optional config default.
	realm            string

	// Injection seams (dependency-injection convention) so tests can mock ARM, the
	// request-name generator, the TTY check, the justification prompt, the env lookup,
	// and the clock.
	newClient           func(doer httpDoer, token, scope, baseURL string) PIMClient
	newRequestName      func() string
	isTTY               func() bool
	promptFunc          func(identityName string) (string, error)
	lookupJustification func() string
	sleep               func(time.Duration)
	pollInterval        time.Duration
	maxPollAttempts     int
}

// NewPIMRoleIdentity creates a new Azure PIM role-activation identity.
func NewPIMRoleIdentity(name string, config *schema.Identity) (authTypes.Identity, error) {
	defer perf.Track(nil, "azure.NewPIMRoleIdentity")()

	if name == "" {
		return nil, fmt.Errorf("%w: identity name is empty", errUtils.ErrInvalidIdentityConfig)
	}
	if config == nil {
		return nil, fmt.Errorf("%w: identity config is nil", errUtils.ErrInvalidIdentityConfig)
	}
	if config.Kind != authTypes.IdentityKindAzurePIMRole {
		return nil, fmt.Errorf("%w: invalid identity kind for Azure PIM role identity: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}

	i := &pimRoleIdentity{
		name:   name,
		config: config,
		newClient: func(doer httpDoer, token, scope, baseURL string) PIMClient {
			return newARMPIMClient(doer, token, scope, baseURL)
		},
		newRequestName:      uuid.NewString,
		isTTY:               defaultIsTTY,
		promptFunc:          defaultJustificationPrompt,
		lookupJustification: func() string { return viper.GetString(pimJustificationViperKey) },
		sleep:               time.Sleep,
		pollInterval:        defaultPIMPollInterval,
		maxPollAttempts:     defaultPIMMaxPollAttempts,
	}
	i.extractPrincipal()
	return i, nil
}

// extractPrincipal reads the principal fields from config into the identity.
func (i *pimRoleIdentity) extractPrincipal() {
	if i.config.Principal == nil {
		return
	}
	if v, ok := i.config.Principal[principalRoleDefinitionIDKey].(string); ok {
		i.roleDefinitionID = v
	}
	if v, ok := i.config.Principal[principalScopeKey].(string); ok {
		i.scope = v
	}
	if v, ok := i.config.Principal[principalDurationKey].(string); ok {
		i.duration = v
	}
	if v, ok := i.config.Principal[principalJustificationKey].(string); ok {
		i.justification = v
	}
}

// Kind returns the identity kind.
func (i *pimRoleIdentity) Kind() string {
	return authTypes.IdentityKindAzurePIMRole
}

// SetRealm sets the credential isolation realm for this identity.
func (i *pimRoleIdentity) SetRealm(realm string) {
	i.realm = realm
}

// GetProviderName returns the provider or parent identity this identity chains from.
func (i *pimRoleIdentity) GetProviderName() (string, error) {
	if i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}
	if i.config.Via != nil && i.config.Via.Identity != "" {
		return i.config.Via.Identity, nil
	}
	return "", fmt.Errorf("%w: Azure PIM role identity %q requires via.provider or via.identity", errUtils.ErrInvalidIdentityConfig, i.name)
}

// Validate validates the identity configuration.
func (i *pimRoleIdentity) Validate() error {
	defer perf.Track(nil, "azure.pimRoleIdentity.Validate")()

	if i.roleDefinitionID == "" {
		return errUtils.Build(errUtils.ErrMissingPrincipal).
			WithExplanationf("Missing 'role_definition_id' for Azure PIM role identity '%s'", i.name).
			WithHint("Add 'role_definition_id' to the principal with the full role definition id").
			WithHint("Example: /providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c").
			WithExitCode(2).
			Err()
	}
	if i.scope == "" {
		return errUtils.Build(errUtils.ErrMissingPrincipal).
			WithExplanationf("Missing 'scope' for Azure PIM role identity '%s'", i.name).
			WithHint("Add 'scope' to the principal (a subscription, resource group, or resource ARM id)").
			WithHint("Example: /subscriptions/00000000-0000-0000-0000-000000000000").
			WithExitCode(2).
			Err()
	}
	if i.config.Via == nil || (i.config.Via.Provider == "" && i.config.Via.Identity == "") {
		return fmt.Errorf("%w: Azure PIM role identity requires via.provider or via.identity", errUtils.ErrInvalidIdentityConfig)
	}
	return nil
}

// Authenticate activates the eligible PIM role and returns the parent credentials unchanged.
func (i *pimRoleIdentity) Authenticate(ctx context.Context, baseCreds authTypes.ICredentials) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "azure.pimRoleIdentity.Authenticate")()

	if err := i.Validate(); err != nil {
		return nil, err
	}

	azureCreds, ok := baseCreds.(*authTypes.AzureCredentials)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithExplanationf("Azure PIM role identity '%s' requires Azure credentials from the parent identity", i.name).
			WithHint("Chain this identity from an Azure identity via 'via.identity' or 'via.provider'").
			WithExitCode(2).
			Err()
	}

	principalID, err := azureCloud.ExtractObjectIDFromToken(azureCreds.AccessToken)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithCause(err).
			WithExplanationf("Could not determine the principal object id for PIM activation on identity '%s'", i.name).
			WithHint("The parent identity must provide a management access token (JWT) carrying an 'oid' claim").
			WithExitCode(1).
			Err()
	}

	baseURL := azureCloud.GetCloudEnvironment(azureCreds.CloudEnvironment).ResourceManagerEndpoint()
	client := i.newClient(nil, azureCreds.AccessToken, i.scope, baseURL)

	// Step 1: short-circuit if the role is already active at scope.
	active, err := client.ActiveAssignmentExists(ctx, i.roleDefinitionID)
	if err != nil {
		return nil, err
	}
	if active {
		log.Debug("PIM role already active, skipping activation", azureCloud.LogFieldIdentity, i.name, "scope", i.scope)
		return baseCreds, nil
	}

	// Step 2: the principal must be eligible; a missing eligibility is actionable.
	eligibilityID, eligible, err := client.FindEligibility(ctx, i.roleDefinitionID)
	if err != nil {
		return nil, err
	}
	if !eligible {
		return nil, errUtils.Build(errUtils.ErrAzurePIMNotEligible).
			WithExplanationf("Principal is not eligible for role '%s' at scope '%s'", i.roleDefinitionID, i.scope).
			WithHint("This identity activates an existing PIM eligibility; it does not grant one").
			WithHint("Ask an administrator to create a PIM-eligible assignment for this role and scope").
			WithContext(azureCloud.LogFieldIdentity, i.name).
			WithExitCode(1).
			Err()
	}

	if err := i.activate(ctx, client, principalID, eligibilityID); err != nil {
		return nil, err
	}

	// Pass-through: the role is now active on the existing principal; token is unchanged.
	return baseCreds, nil
}

// activate issues (or resumes) the self-activation request and waits for it to provision.
func (i *pimRoleIdentity) activate(ctx context.Context, client PIMClient, principalID, eligibilityID string) error {
	// Step 3: resume an in-flight request rather than creating a duplicate.
	requestName, pending, err := client.FindPendingRequest(ctx, i.roleDefinitionID)
	if err != nil {
		return err
	}

	var status string
	if pending {
		log.Debug("Attaching to pending PIM activation request", azureCloud.LogFieldIdentity, i.name, logKeyRequest, requestName)
		if status, err = client.GetRequestStatus(ctx, requestName); err != nil {
			return err
		}
	} else if requestName, status, err = i.submitActivation(ctx, client, principalID, eligibilityID); err != nil {
		return err
	}

	return i.waitForActivation(ctx, client, requestName, status)
}

// submitActivation resolves the justification and files a fresh self-activation request.
func (i *pimRoleIdentity) submitActivation(ctx context.Context, client PIMClient, principalID, eligibilityID string) (string, string, error) {
	justification, err := i.resolveJustification()
	if err != nil {
		return "", "", err
	}
	requestName := i.newRequestName()
	isoDuration := i.isoDuration()
	result, err := client.CreateActivationRequest(ctx, &ActivationRequest{
		RequestName:           requestName,
		PrincipalID:           principalID,
		RoleDefinitionID:      i.roleDefinitionID,
		EligibilityScheduleID: eligibilityID,
		Justification:         justification,
		Duration:              isoDuration,
	})
	if err != nil {
		// ARM enforces the role's PIM activation-policy maximum server-side and rejects a
		// too-long window. Atmos does not yet pre-flight that cap (tracked as a follow-up),
		// so surface an actionable hint alongside ARM's own message.
		b := errUtils.Build(errUtils.ErrAzurePIMActivationFailed).
			WithCause(err).
			WithExplanationf("Could not file the PIM activation request for identity '%s'", i.name).
			WithHint("If the role enforces a shorter window, lower 'duration' to within the role's PIM activation-policy maximum").
			WithContext(azureCloud.LogFieldIdentity, i.name).
			WithExitCode(1)
		if isoDuration != "" {
			b = b.WithContext(principalDurationKey, isoDuration)
		}
		return "", "", b.Err()
	}
	log.Debug("Submitted PIM activation request", azureCloud.LogFieldIdentity, i.name, logKeyRequest, requestName, "status", result.Status)
	return requestName, result.Status, nil
}

// waitForActivation polls a request to a terminal state, bounded by maxPollAttempts.
func (i *pimRoleIdentity) waitForActivation(ctx context.Context, client PIMClient, requestName, status string) error {
	for attempt := 0; ; attempt++ {
		switch {
		case status == pimStatusProvisioned:
			log.Debug("PIM role activated", azureCloud.LogFieldIdentity, i.name, logKeyRequest, requestName)
			return nil
		case !IsPIMPendingStatus(status):
			return errUtils.Build(errUtils.ErrAzurePIMActivationFailed).
				WithExplanationf("PIM activation request '%s' ended in status '%s'", requestName, status).
				WithHint("Check the request in the Azure portal under Privileged Identity Management").
				WithContext(azureCloud.LogFieldIdentity, i.name).
				WithExitCode(1).
				Err()
		case attempt >= i.maxPollAttempts:
			return errUtils.Build(errUtils.ErrAzurePIMActivationTimeout).
				WithExplanationf("Timed out waiting for PIM activation request '%s' (last status '%s')", requestName, status).
				WithHint("Activation may require approval; a later invocation will resume this pending request").
				WithContext(azureCloud.LogFieldIdentity, i.name).
				WithExitCode(1).
				Err()
		}

		log.Debug("Waiting for PIM activation", azureCloud.LogFieldIdentity, i.name, logKeyRequest, requestName, "status", status, "attempt", attempt)
		i.sleep(i.pollInterval)

		var err error
		status, err = client.GetRequestStatus(ctx, requestName)
		if err != nil {
			return err
		}
	}
}

// resolveJustification resolves the activation justification from config, environment, or
// an interactive prompt, refusing clearly when none is available non-interactively.
func (i *pimRoleIdentity) resolveJustification() (string, error) {
	if strings.TrimSpace(i.justification) != "" {
		return i.justification, nil
	}
	if v := strings.TrimSpace(i.lookupJustification()); v != "" {
		return v, nil
	}
	if i.isTTY() {
		if v, err := i.promptFunc(i.name); err == nil {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return trimmed, nil
			}
		}
	}
	return "", errUtils.Build(errUtils.ErrAzurePIMJustificationRequired).
		WithExplanationf("Activating role for identity '%s' requires a justification", i.name).
		WithHint(fmt.Sprintf("Set the %s environment variable, or add 'justification' to the identity principal", pimJustificationEnvVar)).
		WithContext(azureCloud.LogFieldIdentity, i.name).
		WithExitCode(2).
		Err()
}

// isoDuration converts the configured Go-style duration to the ISO-8601 form ARM expects,
// or returns "" (policy default) when unset or unparseable.
func (i *pimRoleIdentity) isoDuration() string {
	if i.duration == "" {
		return ""
	}
	d, err := time.ParseDuration(i.duration)
	if err != nil {
		log.Warn("Invalid duration for PIM activation, using policy default", principalDurationKey, i.duration)
		return ""
	}
	return goDurationToISO8601(d)
}

// Environment returns environment variables for this identity (subscription from scope).
func (i *pimRoleIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)
	if sub := subscriptionIDFromScope(i.scope); sub != "" {
		env["AZURE_SUBSCRIPTION_ID"] = sub
		env["ARM_SUBSCRIPTION_ID"] = sub
	}
	for _, e := range i.config.Env {
		env[e.Key] = e.Value
	}
	return env, nil
}

// PrepareEnvironment prepares environment variables for external processes.
func (i *pimRoleIdentity) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(environ))
	for k, v := range environ {
		result[k] = v
	}
	if sub := subscriptionIDFromScope(i.scope); sub != "" {
		result["AZURE_SUBSCRIPTION_ID"] = sub
		result["ARM_SUBSCRIPTION_ID"] = sub
	}
	for _, e := range i.config.Env {
		result[e.Key] = e.Value
	}
	return result, nil
}

// PostAuthenticate sets up Azure files and auth context using the pass-through credentials.
func (i *pimRoleIdentity) PostAuthenticate(ctx context.Context, params *authTypes.PostAuthenticateParams) error {
	defer perf.Track(nil, "azure.pimRoleIdentity.PostAuthenticate")()

	if params == nil || params.Credentials == nil {
		return fmt.Errorf("%w: PostAuthenticate requires credentials", errUtils.ErrInvalidAuthConfig)
	}

	if err := azureCloud.SetupFiles(params.ProviderName, params.IdentityName, params.Credentials, "", i.realm); err != nil {
		return fmt.Errorf("failed to setup Azure files: %w", err)
	}

	subscriptionID := subscriptionIDFromScope(i.scope)
	if azureCreds, ok := params.Credentials.(*authTypes.AzureCredentials); ok {
		if subscriptionID == "" {
			subscriptionID = azureCreds.SubscriptionID
		}
		if err := azureCloud.UpdateAzureCLIFiles(params.Credentials, azureCreds.TenantID, subscriptionID, azureCreds.CloudEnvironment, i.realm); err != nil {
			log.Debug("Failed to update Azure CLI files", "error", err)
		}
	}

	if err := azureCloud.SetAuthContext(&azureCloud.SetAuthContextParams{
		AuthContext:  params.AuthContext,
		StackInfo:    params.StackInfo,
		ProviderName: params.ProviderName,
		IdentityName: params.IdentityName,
		Credentials:  params.Credentials,
		BasePath:     "",
		Realm:        i.realm,
	}); err != nil {
		return fmt.Errorf("failed to set Azure auth context: %w", err)
	}

	if err := azureCloud.SetEnvironmentVariables(params.AuthContext, params.StackInfo); err != nil {
		return fmt.Errorf("failed to set Azure environment variables: %w", err)
	}
	return nil
}

// Logout is a no-op: this identity mints no credentials of its own.
func (i *pimRoleIdentity) Logout(ctx context.Context) error {
	log.Debug("Logout Azure PIM role identity", azureCloud.LogFieldIdentity, i.name)
	return nil
}

// CredentialsExist reports false: a pass-through identity has no distinct credential storage.
func (i *pimRoleIdentity) CredentialsExist() (bool, error) {
	return false, nil
}

// LoadCredentials returns nil: this identity does not persist its own credentials.
func (i *pimRoleIdentity) LoadCredentials(ctx context.Context) (authTypes.ICredentials, error) {
	return nil, nil
}

// Paths returns no paths: this identity mints no new credential files.
func (i *pimRoleIdentity) Paths() ([]authTypes.Path, error) {
	return []authTypes.Path{}, nil
}

// subscriptionIDFromScope extracts the subscription id from an ARM scope, or "" if the
// scope is not subscription-rooted.
func subscriptionIDFromScope(scope string) string {
	parts := strings.Split(strings.Trim(scope, "/"), "/")
	if len(parts) >= subscriptionScopeSegments && strings.EqualFold(parts[0], "subscriptions") {
		return parts[1]
	}
	return ""
}

// defaultIsTTY reports whether stdin is an interactive terminal.
func defaultIsTTY() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// defaultJustificationPrompt reads a one-line justification from stdin.
func defaultJustificationPrompt(identityName string) (string, error) {
	fmt.Fprintf(os.Stderr, "Enter a justification to activate PIM role for identity %q: ", identityName)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
