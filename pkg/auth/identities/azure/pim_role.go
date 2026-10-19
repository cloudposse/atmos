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
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// Justification is an auth-level concern (portable across implementations), supplied
	// per-invocation via the --justification global flag or the ATMOS_AUTH_JUSTIFICATION
	// environment variable. Both resolve to the "justification" viper key, so viper gives
	// flag > env precedence automatically.
	authJustificationEnvVar = "ATMOS_AUTH_JUSTIFICATION"
	justificationViperKey   = "justification"

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
	warn                func(msg string) // User-facing warning (ui.Warning); overridden in tests.
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
		lookupJustification: func() string { return viper.GetString(justificationViperKey) },
		warn:                ui.Warning, // User-facing advisory (stderr, visible regardless of log level).
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
	if err := validateScope(i.scope); err != nil {
		return err
	}
	if i.config.Via == nil || (i.config.Via.Provider == "" && i.config.Via.Identity == "") {
		return fmt.Errorf("%w: Azure PIM role identity requires via.provider or via.identity", errUtils.ErrInvalidIdentityConfig)
	}
	return nil
}

// validateScope rejects a scope that is not a plain ARM resource path. The scope is
// interpolated into the credential-bearing ARM request URL, so a value that injects URL
// authority or user-information (for example `@attacker.example/...` or `//host/...`) could
// redirect the parent bearer token to an attacker-controlled host. ARM scopes always begin
// with `/` and never contain `@`, whitespace, or a scheme.
func validateScope(scope string) error {
	switch {
	case !strings.HasPrefix(scope, "/"):
		return scopeError(scope, "scope must be an absolute ARM resource path beginning with '/'")
	case strings.HasPrefix(scope, "//"):
		return scopeError(scope, "scope must not begin with '//' (protocol-relative URL)")
	case strings.ContainsAny(scope, "@ \t\r\n?#\\"):
		return scopeError(scope, "scope must not contain '@', whitespace, '?', '#', or '\\'")
	case strings.Contains(scope, "://"):
		return scopeError(scope, "scope must be a path, not a URL")
	}
	return nil
}

func scopeError(scope, hint string) error {
	return errUtils.Build(errUtils.ErrAzurePIMInvalidScope).
		WithExplanationf("Invalid PIM 'scope': %q", scope).
		WithHint(hint).
		WithHint("Example: /subscriptions/00000000-0000-0000-0000-000000000000").
		WithExitCode(2).
		Err()
}

// Authenticate activates the eligible PIM role and returns the parent credentials unchanged.
func (i *pimRoleIdentity) Authenticate(ctx context.Context, baseCreds authTypes.ICredentials) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "azure.pimRoleIdentity.Authenticate")()

	if err := i.Validate(); err != nil {
		return nil, err
	}

	principalID, client, err := i.resolvePrincipalAndClient(baseCreds)
	if err != nil {
		return nil, err
	}

	// Step 1: short-circuit if the role is already active at scope.
	active, err := client.ActiveAssignmentExists(ctx, i.roleDefinitionID)
	if err != nil {
		return nil, err
	}
	if active {
		if i.justificationSupplied() {
			i.warn(fmt.Sprintf("PIM role for identity %q is already active; the supplied --justification was not recorded because no new activation request was filed.", i.name))
		}
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

// resolvePrincipalAndClient validates the parent Azure credentials, resolves the principal
// object id from the management token, and builds a PIM client targeting the correct ARM
// endpoint for the credential's cloud environment.
func (i *pimRoleIdentity) resolvePrincipalAndClient(baseCreds authTypes.ICredentials) (string, PIMClient, error) {
	azureCreds, ok := baseCreds.(*authTypes.AzureCredentials)
	if !ok {
		return "", nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithExplanationf("Azure PIM role identity '%s' requires Azure credentials from the parent identity", i.name).
			WithHint("Chain this identity from an Azure identity via 'via.identity' or 'via.provider'").
			WithExitCode(2).
			Err()
	}

	principalID, err := azureCloud.ExtractObjectIDFromToken(azureCreds.AccessToken)
	if err != nil {
		return "", nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithCause(err).
			WithExplanationf("Could not determine the principal object id for PIM activation on identity '%s'", i.name).
			WithHint("The parent identity must provide a management access token (JWT) carrying an 'oid' claim").
			WithExitCode(1).
			Err()
	}

	baseURL := azureCloud.GetCloudEnvironment(azureCreds.CloudEnvironment).ResourceManagerEndpoint()
	return principalID, i.newClient(nil, azureCreds.AccessToken, i.scope, baseURL), nil
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
		if i.justificationSupplied() {
			i.warn(fmt.Sprintf("Attached to an existing pending PIM request for identity %q; the supplied --justification was not applied (the original request's justification stands).", i.name))
		}
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

		// Wait between polls, but honor context cancellation (Ctrl-C or a canceled parent)
		// instead of blocking for the full interval in time.Sleep.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(i.pollInterval):
		}

		var err error
		status, err = client.GetRequestStatus(ctx, requestName)
		if err != nil {
			return err
		}
	}
}

// ConsumesJustification implements types.JustificationConsumer: this identity records a
// supplied justification when it files a new PIM activation request. The auth manager uses
// this to decide whether a supplied --justification would otherwise go unused.
func (i *pimRoleIdentity) ConsumesJustification() bool { return true }

// justificationSupplied reports whether a per-invocation justification was explicitly
// supplied via --justification / ATMOS_AUTH_JUSTIFICATION (the "justification" viper key),
// as opposed to a configured principal.justification default. Only an explicitly supplied
// value warrants an "unused" warning when activation is skipped or resumed.
func (i *pimRoleIdentity) justificationSupplied() bool {
	return strings.TrimSpace(i.lookupJustification()) != ""
}

// resolveJustification resolves the activation justification in precedence order:
// the --justification flag or ATMOS_AUTH_JUSTIFICATION env (both via the "justification"
// viper key, so a per-invocation reason wins), then the configured principal.justification
// default, then an interactive prompt - refusing clearly when none is available
// non-interactively.
func (i *pimRoleIdentity) resolveJustification() (string, error) {
	if v := strings.TrimSpace(i.lookupJustification()); v != "" {
		return v, nil
	}
	if strings.TrimSpace(i.justification) != "" {
		return i.justification, nil
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
		WithHint(fmt.Sprintf("Pass --justification=REASON, set %s, or add 'justification' to the identity principal", authJustificationEnvVar)).
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
		i.warn(fmt.Sprintf("Invalid duration %q for PIM activation on identity %q; using the role's policy default.", i.duration, i.name))
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

// defaultIsTTY reports whether the justification prompt can be used: the prompt text is written
// to stderr and the answer is read from stdin, so both must be terminals. A captured stderr
// (for example a parent process wrapping this one) would otherwise hide the prompt and hang.
func defaultIsTTY() bool {
	return (isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())) &&
		(isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()))
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
