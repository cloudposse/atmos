package azure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	crerrors "github.com/cockroachdb/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards: a rename of these fields must fail the build, not silently
// weaken the tests (per the testing-strategy sentinel convention).
var (
	_ = schema.Identity{Kind: authTypes.IdentityKindAzurePIMRole}
	_ = authTypes.AzureCredentials{AccessToken: ""}
)

const (
	testRoleDefID    = "/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c"
	testScope        = "/subscriptions/00000000-0000-0000-0000-000000000000"
	testPrincipalOID = "11111111-2222-3333-4444-555555555555"
	testEligID       = "/subscriptions/00000000-0000-0000-0000-000000000000/providers/Microsoft.Authorization/roleEligibilitySchedules/elig-1"
)

// mockPIMClient is an injectable PIMClient for unit tests.
type mockPIMClient struct {
	activeExists bool
	activeErr    error

	eligScheduleID string
	eligFound      bool
	eligErr        error

	pendingName  string
	pendingFound bool
	pendingErr   error

	createResult ActivationResult
	createErr    error
	createdReq   *ActivationRequest
	createCalls  int

	statuses    []string
	statusErr   error
	statusCalls int

	policyMaxDuration time.Duration
	policyMaxFound    bool
	policyMaxErr      error
	policyMaxCalls    int
}

func (m *mockPIMClient) ActiveAssignmentExists(_ context.Context, _ string) (bool, error) {
	return m.activeExists, m.activeErr
}

func (m *mockPIMClient) FindEligibility(_ context.Context, _ string) (string, bool, error) {
	return m.eligScheduleID, m.eligFound, m.eligErr
}

func (m *mockPIMClient) FindPendingRequest(_ context.Context, _ string) (string, bool, error) {
	return m.pendingName, m.pendingFound, m.pendingErr
}

func (m *mockPIMClient) CreateActivationRequest(_ context.Context, req *ActivationRequest) (ActivationResult, error) {
	m.createCalls++
	reqCopy := *req
	m.createdReq = &reqCopy
	return m.createResult, m.createErr
}

func (m *mockPIMClient) GetRequestStatus(_ context.Context, _ string) (string, error) {
	if m.statusErr != nil {
		return "", m.statusErr
	}
	if m.statusCalls < len(m.statuses) {
		s := m.statuses[m.statusCalls]
		m.statusCalls++
		return s, nil
	}
	if len(m.statuses) > 0 {
		return m.statuses[len(m.statuses)-1], nil
	}
	return "", nil
}

func (m *mockPIMClient) PolicyMaxDuration(_ context.Context, _ string) (time.Duration, bool, error) {
	m.policyMaxCalls++
	return m.policyMaxDuration, m.policyMaxFound, m.policyMaxErr
}

// makeJWT builds a header.payload.signature token whose payload carries the given claims.
func makeJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	body, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(body)
	return header + "." + payload + ".sig"
}

func testAzureCreds() *authTypes.AzureCredentials {
	return &authTypes.AzureCredentials{
		AccessToken:    makeJWT(map[string]any{"oid": testPrincipalOID}),
		TenantID:       "tenant-123",
		SubscriptionID: "00000000-0000-0000-0000-000000000000",
		Expiration:     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}
}

func defaultPrincipal() map[string]any {
	return map[string]any{
		"role_definition_id": testRoleDefID,
		"scope":              testScope,
		"duration":           "8h",
		"justification":      "planned change window",
	}
}

// newTestIdentity builds a pim-role identity wired to the mock client with deterministic seams.
func newTestIdentity(t *testing.T, principal map[string]any, mock *mockPIMClient) *pimRoleIdentity {
	t.Helper()
	cfg := &schema.Identity{
		Kind:      authTypes.IdentityKindAzurePIMRole,
		Via:       &schema.IdentityVia{Identity: "azure-dev"},
		Principal: principal,
	}
	id, err := NewPIMRoleIdentity("prod-contributor", cfg)
	require.NoError(t, err)
	p, ok := id.(*pimRoleIdentity)
	require.True(t, ok)

	p.newClient = func(_ httpDoer, _, _, _ string) PIMClient { return mock }
	p.newRequestName = func() string { return "test-request-guid" }
	p.isTTY = func() bool { return false }
	p.promptFunc = func(string) (string, error) { return "", errors.New("no prompt in test") }
	p.lookupJustification = func() string { return "" }
	p.warn = func(string) {} // no-op by default; warning tests override to capture.
	p.pollInterval = 0
	return p
}

func TestNewPIMRoleIdentity(t *testing.T) {
	tests := []struct {
		name    string
		idName  string
		config  *schema.Identity
		wantErr error
	}{
		{
			name:    "empty name",
			idName:  "",
			config:  &schema.Identity{Kind: authTypes.IdentityKindAzurePIMRole},
			wantErr: errUtils.ErrInvalidIdentityConfig,
		},
		{
			name:    "nil config",
			idName:  "x",
			config:  nil,
			wantErr: errUtils.ErrInvalidIdentityConfig,
		},
		{
			name:    "wrong kind",
			idName:  "x",
			config:  &schema.Identity{Kind: "azure/subscription"},
			wantErr: errUtils.ErrInvalidIdentityKind,
		},
		{
			name:    "valid",
			idName:  "x",
			config:  &schema.Identity{Kind: authTypes.IdentityKindAzurePIMRole, Principal: defaultPrincipal()},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewPIMRoleIdentity(tt.idName, tt.config)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, id)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, id)
			assert.Equal(t, authTypes.IdentityKindAzurePIMRole, id.Kind())
		})
	}
}

func TestPIMRole_EligibleAndActivates(t *testing.T) {
	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		createResult:   ActivationResult{RequestName: "test-request-guid", Status: pimStatusProvisioned},
	}
	id := newTestIdentity(t, defaultPrincipal(), mock)
	base := testAzureCreds()

	got, err := id.Authenticate(context.Background(), base)
	require.NoError(t, err)

	// Pass-through: exact same credentials returned.
	assert.Same(t, base, got)

	// The activation request carried the resolved principal, role, eligibility, and ISO duration.
	require.NotNil(t, mock.createdReq)
	assert.Equal(t, testPrincipalOID, mock.createdReq.PrincipalID)
	assert.Equal(t, testRoleDefID, mock.createdReq.RoleDefinitionID)
	assert.Equal(t, testEligID, mock.createdReq.EligibilityScheduleID)
	assert.Equal(t, "planned change window", mock.createdReq.Justification)
	assert.Equal(t, "PT8H", mock.createdReq.Duration)
	assert.Equal(t, "test-request-guid", mock.createdReq.RequestName)
}

func TestPIMRole_AlreadyActive_NoOp(t *testing.T) {
	mock := &mockPIMClient{activeExists: true}
	id := newTestIdentity(t, defaultPrincipal(), mock)
	base := testAzureCreds()

	got, err := id.Authenticate(context.Background(), base)
	require.NoError(t, err)
	assert.Same(t, base, got)

	// No activation was attempted.
	assert.Equal(t, 0, mock.createCalls)
	assert.Nil(t, mock.createdReq)
}

func TestPIMRole_PendingThenProvisioned(t *testing.T) {
	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		createResult:   ActivationResult{RequestName: "test-request-guid", Status: "PendingApproval"},
		statuses:       []string{"PendingApproval", pimStatusProvisioned},
	}
	id := newTestIdentity(t, defaultPrincipal(), mock)
	base := testAzureCreds()

	got, err := id.Authenticate(context.Background(), base)
	require.NoError(t, err)
	assert.Same(t, base, got)
	assert.GreaterOrEqual(t, mock.statusCalls, 2)
}

func TestPIMRole_NotEligible(t *testing.T) {
	mock := &mockPIMClient{eligFound: false}
	id := newTestIdentity(t, defaultPrincipal(), mock)

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.ErrorIs(t, err, errUtils.ErrAzurePIMNotEligible)
	assert.Equal(t, 0, mock.createCalls)
}

func TestPIMRole_NonInteractiveWithoutJustification(t *testing.T) {
	principal := defaultPrincipal()
	delete(principal, "justification")

	mock := &mockPIMClient{eligFound: true, eligScheduleID: testEligID}
	id := newTestIdentity(t, principal, mock)

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.ErrorIs(t, err, errUtils.ErrAzurePIMJustificationRequired)
	assert.Equal(t, 0, mock.createCalls, "no request should be filed without a justification")
}

func TestPIMRole_JustificationFromFlagOrEnv(t *testing.T) {
	principal := defaultPrincipal()
	delete(principal, "justification")

	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		createResult:   ActivationResult{Status: pimStatusProvisioned},
	}
	id := newTestIdentity(t, principal, mock)
	// The seam resolves --justification / ATMOS_AUTH_JUSTIFICATION (via the "justification"
	// viper key) in production.
	id.lookupJustification = func() string { return "flag-or-env reason" }

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.NoError(t, err)
	require.NotNil(t, mock.createdReq)
	assert.Equal(t, "flag-or-env reason", mock.createdReq.Justification)
}

func TestPIMRole_FlagOrEnvOverridesConfigJustification(t *testing.T) {
	// New precedence: a per-invocation --justification/ATMOS_AUTH_JUSTIFICATION overrides
	// the configured principal.justification default.
	principal := defaultPrincipal() // has justification: "planned change window"
	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		createResult:   ActivationResult{Status: pimStatusProvisioned},
	}
	id := newTestIdentity(t, principal, mock)
	id.lookupJustification = func() string { return "per-invocation reason" }

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.NoError(t, err)
	require.NotNil(t, mock.createdReq)
	assert.Equal(t, "per-invocation reason", mock.createdReq.Justification,
		"flag/env must override the configured principal.justification")
}

func TestPIMRole_DefaultJustificationLookupReadsViperKey(t *testing.T) {
	// The default seam reads the "justification" viper key (which the --justification global
	// flag and ATMOS_AUTH_JUSTIFICATION env both resolve to; that binding is covered by the
	// pkg/flags tests).
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set(justificationViperKey, "from-viper-key")

	cfg := &schema.Identity{
		Kind:      authTypes.IdentityKindAzurePIMRole,
		Via:       &schema.IdentityVia{Identity: "azure-dev"},
		Principal: defaultPrincipal(),
	}
	id, err := NewPIMRoleIdentity("x", cfg)
	require.NoError(t, err)
	assert.Equal(t, "from-viper-key", id.(*pimRoleIdentity).lookupJustification())
}

func TestPIMRole_JustificationFromPrompt(t *testing.T) {
	principal := defaultPrincipal()
	delete(principal, "justification")

	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		createResult:   ActivationResult{Status: pimStatusProvisioned},
	}
	id := newTestIdentity(t, principal, mock)
	id.isTTY = func() bool { return true }
	id.promptFunc = func(string) (string, error) { return "typed reason", nil }

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.NoError(t, err)
	require.NotNil(t, mock.createdReq)
	assert.Equal(t, "typed reason", mock.createdReq.Justification)
}

func TestPIMRole_ResumePendingRequest(t *testing.T) {
	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		pendingFound:   true,
		pendingName:    "existing-request",
		statuses:       []string{pimStatusProvisioned},
	}
	id := newTestIdentity(t, defaultPrincipal(), mock)
	// Guard: resuming must not generate a new request name.
	id.newRequestName = func() string {
		t.Fatalf("newRequestName must not be called when resuming a pending request")
		return ""
	}

	got, err := id.Authenticate(context.Background(), testAzureCreds())
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, 0, mock.createCalls, "resume must not create a duplicate request")
}

func TestPIMRole_ActivationTimeout(t *testing.T) {
	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		createResult:   ActivationResult{Status: "PendingApproval"},
		statuses:       []string{"PendingApproval"},
	}
	id := newTestIdentity(t, defaultPrincipal(), mock)
	id.maxPollAttempts = 1

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.ErrorIs(t, err, errUtils.ErrAzurePIMActivationTimeout)
}

func TestPIMRole_CreateRequestFailsWithDurationHint(t *testing.T) {
	// ARM rejects a too-long window; the error carries both sentinels (the activation
	// failure and the underlying request failure) plus an actionable duration hint.
	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		createErr:      fmt.Errorf("%w: PUT returned 400: ExpirationTooLong", errUtils.ErrAzurePIMRequestFailed),
	}
	id := newTestIdentity(t, defaultPrincipal(), mock)

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.ErrorIs(t, err, errUtils.ErrAzurePIMActivationFailed)
	require.ErrorIs(t, err, errUtils.ErrAzurePIMRequestFailed, "underlying ARM cause is preserved")

	joined := strings.Join(crerrors.GetAllHints(err), " ")
	assert.Contains(t, joined, "duration", "activation-create failure must hint at lowering the duration")
}

func TestPIMRole_ActivationFailed(t *testing.T) {
	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		createResult:   ActivationResult{Status: "Denied"},
	}
	id := newTestIdentity(t, defaultPrincipal(), mock)

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.ErrorIs(t, err, errUtils.ErrAzurePIMActivationFailed)
}

func TestPIMRole_PollRespectsContextCancellation(t *testing.T) {
	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		createResult:   ActivationResult{Status: "PendingApproval"},
		statuses:       []string{"PendingApproval"},
	}
	id := newTestIdentity(t, defaultPrincipal(), mock)
	// A long interval forces the poll wait to resolve via ctx cancellation, not the timer.
	id.pollInterval = 10 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := id.Authenticate(ctx, testAzureCreds())
	require.ErrorIs(t, err, context.Canceled, "a canceled context must stop the poll wait promptly")
}

func TestPIMRole_InvalidBaseCreds(t *testing.T) {
	mock := &mockPIMClient{}
	id := newTestIdentity(t, defaultPrincipal(), mock)

	_, err := id.Authenticate(context.Background(), &authTypes.AWSCredentials{AccessKeyID: "x"})
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

func TestPIMRole_InvalidToken(t *testing.T) {
	mock := &mockPIMClient{}
	id := newTestIdentity(t, defaultPrincipal(), mock)
	base := testAzureCreds()
	base.AccessToken = "not-a-jwt"

	_, err := id.Authenticate(context.Background(), base)
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

func TestPIMRole_ClientErrorPropagates(t *testing.T) {
	sentinel := errors.New("arm boom")
	mock := &mockPIMClient{activeErr: sentinel}
	id := newTestIdentity(t, defaultPrincipal(), mock)

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.ErrorIs(t, err, sentinel)
}

func TestPIMRole_Validate(t *testing.T) {
	tests := []struct {
		name      string
		principal map[string]any
		via       *schema.IdentityVia
		wantErr   error
	}{
		{
			name:      "missing role_definition_id",
			principal: map[string]any{"scope": testScope},
			via:       &schema.IdentityVia{Identity: "azure-dev"},
			wantErr:   errUtils.ErrMissingPrincipal,
		},
		{
			name:      "missing scope",
			principal: map[string]any{"role_definition_id": testRoleDefID},
			via:       &schema.IdentityVia{Identity: "azure-dev"},
			wantErr:   errUtils.ErrMissingPrincipal,
		},
		{
			name:      "missing via",
			principal: defaultPrincipal(),
			via:       nil,
			wantErr:   errUtils.ErrInvalidIdentityConfig,
		},
		{
			name:      "valid via provider",
			principal: defaultPrincipal(),
			via:       &schema.IdentityVia{Provider: "azure-interactive"},
			wantErr:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &schema.Identity{Kind: authTypes.IdentityKindAzurePIMRole, Via: tt.via, Principal: tt.principal}
			id, err := NewPIMRoleIdentity("x", cfg)
			require.NoError(t, err)
			verr := id.(*pimRoleIdentity).Validate()
			if tt.wantErr != nil {
				require.ErrorIs(t, verr, tt.wantErr)
				return
			}
			require.NoError(t, verr)
		})
	}
}

func TestValidateScope(t *testing.T) {
	valid := []string{
		"/subscriptions/00000000-0000-0000-0000-000000000000",
		"/subscriptions/x/resourceGroups/rg1",
		"/providers/Microsoft.Management/managementGroups/mg1",
	}
	for _, s := range valid {
		t.Run("valid/"+s, func(t *testing.T) {
			require.NoError(t, validateScope(s))
		})
	}

	invalid := []string{
		"@collector.example/subscriptions/x",  // URL user-information injection.
		"//collector.example/subscriptions/x", // Protocol-relative.
		"https://collector.example/x",         // Full URL.
		"subscriptions/x",                     // Missing leading slash.
		"/subscriptions/x@evil.example",       // Embedded '@'.
		"/subscriptions/x y",                  // Whitespace.
		"/subscriptions/x?api-version=evil",   // Query injection.
	}
	for _, s := range invalid {
		t.Run("invalid/"+s, func(t *testing.T) {
			require.ErrorIs(t, validateScope(s), errUtils.ErrAzurePIMInvalidScope)
		})
	}
}

func TestPIMRole_RejectsMaliciousScope(t *testing.T) {
	principal := defaultPrincipal()
	principal["scope"] = "@collector.example/subscriptions/x"
	mock := &mockPIMClient{eligFound: true, eligScheduleID: testEligID}
	id := newTestIdentity(t, principal, mock)
	// Guard: a rejected scope must never reach the ARM client.
	id.newClient = func(_ httpDoer, _, _, _ string) PIMClient {
		t.Fatalf("client must not be constructed for an invalid scope")
		return nil
	}

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.ErrorIs(t, err, errUtils.ErrAzurePIMInvalidScope)
}

func TestPIMRole_GetProviderName(t *testing.T) {
	tests := []struct {
		name    string
		via     *schema.IdentityVia
		want    string
		wantErr bool
	}{
		{name: "via provider", via: &schema.IdentityVia{Provider: "azure-interactive"}, want: "azure-interactive"},
		{name: "via identity", via: &schema.IdentityVia{Identity: "azure-dev"}, want: "azure-dev"},
		{name: "neither", via: &schema.IdentityVia{}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &schema.Identity{Kind: authTypes.IdentityKindAzurePIMRole, Via: tt.via, Principal: defaultPrincipal()}
			id, err := NewPIMRoleIdentity("x", cfg)
			require.NoError(t, err)
			got, gerr := id.GetProviderName()
			if tt.wantErr {
				require.Error(t, gerr)
				return
			}
			require.NoError(t, gerr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPIMRole_KindAndSetRealm(t *testing.T) {
	mock := &mockPIMClient{}
	id := newTestIdentity(t, defaultPrincipal(), mock)
	assert.Equal(t, authTypes.IdentityKindAzurePIMRole, id.Kind())
	id.SetRealm("r1")
	assert.Equal(t, "r1", id.realm)
}

func TestPIMRole_EnvironmentAndPrepareEnvironment(t *testing.T) {
	principal := defaultPrincipal()
	cfg := &schema.Identity{
		Kind:      authTypes.IdentityKindAzurePIMRole,
		Via:       &schema.IdentityVia{Identity: "azure-dev"},
		Principal: principal,
		Env:       []schema.EnvironmentVariable{{Key: "FOO", Value: "bar"}},
	}
	id, err := NewPIMRoleIdentity("x", cfg)
	require.NoError(t, err)

	env, err := id.Environment()
	require.NoError(t, err)
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", env["AZURE_SUBSCRIPTION_ID"])
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", env["ARM_SUBSCRIPTION_ID"])
	assert.Equal(t, "bar", env["FOO"])

	prepared, err := id.PrepareEnvironment(context.Background(), map[string]string{"EXISTING": "1"})
	require.NoError(t, err)
	assert.Equal(t, "1", prepared["EXISTING"])
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", prepared["ARM_SUBSCRIPTION_ID"])
	assert.Equal(t, "bar", prepared["FOO"])
}

func TestPIMRole_PassThroughMethods(t *testing.T) {
	mock := &mockPIMClient{}
	id := newTestIdentity(t, defaultPrincipal(), mock)

	exists, err := id.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists)

	creds, err := id.LoadCredentials(context.Background())
	require.NoError(t, err)
	assert.Nil(t, creds)

	paths, err := id.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)

	require.NoError(t, id.Logout(context.Background()))
}

func TestPIMRole_PostAuthenticate(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	homedir.Reset()
	homedir.DisableCache = true
	t.Cleanup(func() {
		homedir.Reset()
		homedir.DisableCache = false
	})

	mock := &mockPIMClient{}
	id := newTestIdentity(t, defaultPrincipal(), mock)
	id.SetRealm("test-realm")

	authContext := &schema.AuthContext{}
	stackInfo := &schema.ConfigAndStacksInfo{}
	err := id.PostAuthenticate(context.Background(), &authTypes.PostAuthenticateParams{
		AuthContext:  authContext,
		StackInfo:    stackInfo,
		ProviderName: "azure-interactive",
		IdentityName: "prod-contributor",
		Credentials:  testAzureCreds(),
		Realm:        "test-realm",
	})
	require.NoError(t, err)

	credsPath := filepath.Join(tmpHome, ".azure", "atmos", "test-realm", "azure-interactive", "credentials.json")
	_, statErr := os.Stat(credsPath)
	assert.NoError(t, statErr, "credentials.json should be written by SetupFiles")

	require.NotNil(t, authContext.Azure)
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", authContext.Azure.SubscriptionID)
}

func TestPIMRole_PostAuthenticate_Guards(t *testing.T) {
	mock := &mockPIMClient{}
	id := newTestIdentity(t, defaultPrincipal(), mock)

	require.ErrorIs(t, id.PostAuthenticate(context.Background(), nil), errUtils.ErrInvalidAuthConfig)
	require.ErrorIs(t, id.PostAuthenticate(context.Background(), &authTypes.PostAuthenticateParams{}), errUtils.ErrInvalidAuthConfig)
}

func TestPIMRole_IsoDuration(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"8h", "PT8H"},
		{"90m", "PT1H30M"},
		{"30m", "PT30M"},
		{"45s", "PT45S"},
		{"1h30m15s", "PT1H30M15S"},
		{"", ""},
		{"not-a-duration", ""},
		{"0s", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			id := &pimRoleIdentity{duration: tt.in, warn: func(string) {}}
			assert.Equal(t, tt.want, id.isoDuration())
		})
	}
}

func TestPIMRole_InvalidDurationStillActivates(t *testing.T) {
	principal := defaultPrincipal()
	principal["duration"] = "garbage"
	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		createResult:   ActivationResult{Status: pimStatusProvisioned},
	}
	id := newTestIdentity(t, principal, mock)

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.NoError(t, err)
	require.NotNil(t, mock.createdReq)
	// Invalid duration falls back to policy default (empty).
	assert.Empty(t, mock.createdReq.Duration)
}

func TestPIMRole_PostAuthenticate_NonSubscriptionScopeFallsBackToCreds(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	homedir.Reset()
	homedir.DisableCache = true
	t.Cleanup(func() {
		homedir.Reset()
		homedir.DisableCache = false
	})

	principal := defaultPrincipal()
	// Management-group scope has no subscription id, so PostAuthenticate falls back to the
	// subscription carried by the pass-through credentials.
	principal["scope"] = "/providers/Microsoft.Management/managementGroups/mg1"
	mock := &mockPIMClient{}
	id := newTestIdentity(t, principal, mock)
	id.SetRealm("test-realm")

	creds := testAzureCreds()
	creds.SubscriptionID = "sub-from-creds"
	authContext := &schema.AuthContext{}
	err := id.PostAuthenticate(context.Background(), &authTypes.PostAuthenticateParams{
		AuthContext:  authContext,
		StackInfo:    &schema.ConfigAndStacksInfo{},
		ProviderName: "azure-interactive",
		IdentityName: "prod-contributor",
		Credentials:  creds,
		Realm:        "test-realm",
	})
	require.NoError(t, err)
	require.NotNil(t, authContext.Azure)
	assert.Equal(t, "sub-from-creds", authContext.Azure.SubscriptionID)
}

func TestDefaultJustificationPrompt(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig })

	_, _ = io.WriteString(w, "my reason\n")
	require.NoError(t, w.Close())

	got, err := defaultJustificationPrompt("prod-contributor")
	require.NoError(t, err)
	assert.Equal(t, "my reason", got)
}

func TestDefaultIsTTY(t *testing.T) {
	// In the test harness stdin is not an interactive terminal; the call must not panic
	// and should report false.
	assert.False(t, defaultIsTTY())
}

func TestPIMRole_ConsumesJustification(t *testing.T) {
	id := newTestIdentity(t, defaultPrincipal(), &mockPIMClient{})
	assert.True(t, id.ConsumesJustification(), "azure/pim-role records a supplied justification")
}

func TestPIMRole_WarnsWhenAlreadyActiveWithSuppliedJustification(t *testing.T) {
	tests := []struct {
		name          string
		supplied      string
		expectWarning bool
	}{
		{name: "supplied justification warns", supplied: "per-invocation reason", expectWarning: true},
		{name: "no supplied justification is silent", supplied: "", expectWarning: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPIMClient{activeExists: true}
			id := newTestIdentity(t, defaultPrincipal(), mock)
			id.lookupJustification = func() string { return tt.supplied }
			var warnings []string
			id.warn = func(msg string) { warnings = append(warnings, msg) }

			_, err := id.Authenticate(context.Background(), testAzureCreds())
			require.NoError(t, err)
			assert.Equal(t, 0, mock.createCalls)
			if tt.expectWarning {
				require.Len(t, warnings, 1)
				assert.Contains(t, warnings[0], "already active")
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}

func TestPIMRole_WarnsWhenResumingWithSuppliedJustification(t *testing.T) {
	tests := []struct {
		name          string
		supplied      string
		expectWarning bool
	}{
		{name: "supplied justification warns", supplied: "per-invocation reason", expectWarning: true},
		{name: "no supplied justification is silent", supplied: "", expectWarning: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPIMClient{
				eligFound:      true,
				eligScheduleID: testEligID,
				pendingFound:   true,
				pendingName:    "existing-request",
				statuses:       []string{pimStatusProvisioned},
			}
			id := newTestIdentity(t, defaultPrincipal(), mock)
			id.lookupJustification = func() string { return tt.supplied }
			var warnings []string
			id.warn = func(msg string) { warnings = append(warnings, msg) }

			_, err := id.Authenticate(context.Background(), testAzureCreds())
			require.NoError(t, err)
			assert.Equal(t, 0, mock.createCalls, "resume must not create a new request")
			if tt.expectWarning {
				require.Len(t, warnings, 1)
				assert.Contains(t, warnings[0], "pending")
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}

func TestPIMRole_DurationClampedAtPolicyMax(t *testing.T) {
	mock := &mockPIMClient{
		eligFound:         true,
		eligScheduleID:    testEligID,
		policyMaxFound:    true,
		policyMaxDuration: 4 * time.Hour,
		statuses:          []string{pimStatusProvisioned},
		createResult:      ActivationResult{RequestName: "test-req", Status: pimStatusProvisioned},
	}
	// defaultPrincipal has duration "8h", which exceeds the 4h policy max.
	id := newTestIdentity(t, defaultPrincipal(), mock)
	var warnings []string
	id.warn = func(msg string) { warnings = append(warnings, msg) }

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.NoError(t, err)

	assert.Equal(t, 1, mock.policyMaxCalls)
	assert.Equal(t, 1, mock.createCalls)
	assert.Equal(t, "PT4H", mock.createdReq.Duration, "duration must be clamped to policy maximum PT4H")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "exceeds the role's policy maximum")
	assert.Contains(t, warnings[0], "capping activation to PT4H")
}

func TestPIMRole_DurationWithinPolicyMax_NotClamped(t *testing.T) {
	mock := &mockPIMClient{
		eligFound:         true,
		eligScheduleID:    testEligID,
		policyMaxFound:    true,
		policyMaxDuration: 4 * time.Hour,
		statuses:          []string{pimStatusProvisioned},
		createResult:      ActivationResult{RequestName: "test-req", Status: pimStatusProvisioned},
	}
	principal := defaultPrincipal()
	principal["duration"] = "2h" // 2h is within 4h max.
	id := newTestIdentity(t, principal, mock)
	var warnings []string
	id.warn = func(msg string) { warnings = append(warnings, msg) }

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.NoError(t, err)

	assert.Equal(t, 1, mock.policyMaxCalls)
	assert.Equal(t, 1, mock.createCalls)
	assert.Equal(t, "PT2H", mock.createdReq.Duration, "duration within policy max must not be clamped")
	assert.Empty(t, warnings)
}

func TestPIMRole_PolicyMaxNotFound_UsesConfiguredDuration(t *testing.T) {
	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		policyMaxFound: false,
		statuses:       []string{pimStatusProvisioned},
		createResult:   ActivationResult{RequestName: "test-req", Status: pimStatusProvisioned},
	}
	id := newTestIdentity(t, defaultPrincipal(), mock)
	var warnings []string
	id.warn = func(msg string) { warnings = append(warnings, msg) }

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.NoError(t, err)

	assert.Equal(t, 1, mock.policyMaxCalls)
	assert.Equal(t, 1, mock.createCalls)
	assert.Equal(t, "PT8H", mock.createdReq.Duration)
	assert.Empty(t, warnings)
}

func TestPIMRole_PolicyMaxQueryError(t *testing.T) {
	sentinel := errors.New("policy lookup failed")
	mock := &mockPIMClient{
		eligFound:      true,
		eligScheduleID: testEligID,
		policyMaxErr:   sentinel,
		statuses:       []string{pimStatusProvisioned},
		createResult:   ActivationResult{RequestName: "test-req", Status: pimStatusProvisioned},
	}
	id := newTestIdentity(t, defaultPrincipal(), mock)
	var warnings []string
	id.warn = func(msg string) { warnings = append(warnings, msg) }

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.NoError(t, err)

	assert.Equal(t, 1, mock.policyMaxCalls)
	assert.Equal(t, 1, mock.createCalls, "activation must proceed when policy query fails")
	assert.Equal(t, "PT8H", mock.createdReq.Duration)
	assert.Empty(t, warnings)
}

func TestPIMRole_EmptyDuration_SkipsPolicyMaxQuery(t *testing.T) {
	mock := &mockPIMClient{
		eligFound:         true,
		eligScheduleID:    testEligID,
		policyMaxFound:    true,
		policyMaxDuration: 4 * time.Hour,
		statuses:          []string{pimStatusProvisioned},
		createResult:      ActivationResult{RequestName: "test-req", Status: pimStatusProvisioned},
	}
	principal := defaultPrincipal()
	principal["duration"] = ""
	id := newTestIdentity(t, principal, mock)

	_, err := id.Authenticate(context.Background(), testAzureCreds())
	require.NoError(t, err)

	assert.Equal(t, 0, mock.policyMaxCalls, "empty duration must omit policy max query")
	assert.Equal(t, 1, mock.createCalls)
	assert.Equal(t, "", mock.createdReq.Duration)
}
