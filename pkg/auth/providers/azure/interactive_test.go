package azure

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestNewInteractiveProvider(t *testing.T) {
	tests := []struct {
		name        string
		config      *schema.Provider
		expectError bool
		errorType   error
	}{
		{
			name: "valid config",
			config: &schema.Provider{
				Kind: "azure/interactive",
				Spec: map[string]interface{}{
					"tenant_id": "tenant-123",
				},
			},
		},
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name: "wrong kind",
			config: &schema.Provider{
				Kind: "azure/device-code",
				Spec: map[string]interface{}{
					"tenant_id": "tenant-123",
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderKind,
		},
		{
			name: "missing tenant_id",
			config: &schema.Provider{
				Kind: "azure/interactive",
				Spec: map[string]interface{}{},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name: "invalid cloud environment",
			config: &schema.Provider{
				Kind: "azure/interactive",
				Spec: map[string]interface{}{
					"tenant_id":         "tenant-123",
					"cloud_environment": "not-a-cloud",
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewInteractiveProvider("azure-interactive", tt.config)
			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errorType)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "azure/interactive", p.Kind())
			assert.Equal(t, "azure-interactive", p.Name())
			assert.Equal(t, authTypes.AzureAuthMethodInteractive, p.authMethod)
		})
	}
}

func TestInteractiveProvider_CredentialsAuthMethodAndAccountSource(t *testing.T) {
	p, err := NewInteractiveProvider("azure-interactive", &schema.Provider{
		Kind: "azure/interactive",
		Spec: map[string]interface{}{
			"tenant_id": "tenant-123",
		},
	})
	require.NoError(t, err)

	// Interactive credentials carry the interactive auth method, and the MSAL
	// cache Account entry mirrors az's label for the browser flow.
	assert.Equal(t, authTypes.AzureAuthMethodInteractive, p.credentialsAuthMethod())
	assert.Equal(t, "authorization_code", p.accountSource())

	// The plain device-code provider keeps its own labels.
	dc, err := NewDeviceCodeProvider("azure-device-code", &schema.Provider{
		Kind: "azure/device-code",
		Spec: map[string]interface{}{
			"tenant_id": "tenant-123",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, authTypes.AzureAuthMethodDeviceCode, dc.credentialsAuthMethod())
	assert.Equal(t, "device_code", dc.accountSource())
}

func newTestInteractiveProvider(t *testing.T) *interactiveProvider {
	t.Helper()
	p, err := NewInteractiveProvider("azure-interactive", &schema.Provider{
		Kind: "azure/interactive",
		Spec: map[string]interface{}{"tenant_id": "tenant-123"},
	})
	require.NoError(t, err)
	return p
}

func TestInteractiveProvider_Authenticate_Success(t *testing.T) {
	// Sandbox HOME so MSAL and Azure CLI cache files land in a temp dir.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	p := newTestInteractiveProvider(t)
	now := time.Now().UTC()

	// Inject the interactive acquisition (a live IdP + browser cannot run in tests).
	p.checkInteractive = func() bool { return true }
	p.acquireInteractive = func(ctx context.Context, client *public.Client, scopes []string) (public.AuthResult, error) {
		require.Equal(t, []string{p.cloudEnv.ManagementScope}, scopes)
		return public.AuthResult{
			AccessToken: "interactive-token",
			ExpiresOn:   now.Add(1 * time.Hour),
			Account:     public.Account{HomeAccountID: "home-oid.home-tenant"},
		}, nil
	}

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)

	azureCreds, ok := creds.(*authTypes.AzureCredentials)
	require.True(t, ok)
	assert.Equal(t, "interactive-token", azureCreds.AccessToken)
	assert.Equal(t, authTypes.AzureAuthMethodInteractive, azureCreds.AuthMethod)
	assert.Equal(t, "home-oid.home-tenant", azureCreds.HomeAccountID,
		"MSAL home account ID from the interactive result must reach the credentials")
}

func TestInteractiveProvider_Authenticate_AcquisitionError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	p := newTestInteractiveProvider(t)
	p.checkInteractive = func() bool { return true }
	p.acquireInteractive = func(ctx context.Context, client *public.Client, scopes []string) (public.AuthResult, error) {
		return public.AuthResult{}, errUtils.ErrAuthenticationFailed
	}

	_, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

func TestInteractiveProvider_Authenticate_Headless(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	// Under `go test` stderr is not a TTY, so the default checkInteractive
	// refuses to open a browser.
	p := newTestInteractiveProvider(t)

	_, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Contains(t, err.Error(), "interactive session")
}

func TestDeviceCodeProvider_CredentialsAuthMethod_DefaultsWhenUnset(t *testing.T) {
	// Instances constructed directly (without a constructor) default to device_code.
	p := &deviceCodeProvider{}
	assert.Equal(t, authTypes.AzureAuthMethodDeviceCode, p.credentialsAuthMethod())
	assert.Equal(t, "device_code", p.accountSource())
}

// promptStreams implements iolib.Streams over in-memory buffers so UI output
// can be asserted.
type promptStreams struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (s *promptStreams) Input() io.Reader     { return s.stdin }
func (s *promptStreams) Output() io.Writer    { return s.stdout }
func (s *promptStreams) Error() io.Writer     { return s.stderr }
func (s *promptStreams) RawOutput() io.Writer { return s.stdout }
func (s *promptStreams) RawError() io.Writer  { return s.stderr }

func TestDisplayBrowserPrompt(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(&promptStreams{
		stdin:  &bytes.Buffer{},
		stdout: stdout,
		stderr: stderr,
	}))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(func() {
		// Restore a formatter bound to the real streams for subsequent tests.
		realCtx, initErr := iolib.NewContext()
		require.NoError(t, initErr)
		ui.InitFormatter(realCtx)
	})

	displayBrowserPrompt()

	out := stderr.String()
	assert.Contains(t, out, "Azure Authentication Required")
	assert.Contains(t, out, "Opening your browser to complete sign-in")
	assert.Empty(t, stdout.String(), "human prompts go to the UI channel (stderr), not stdout")
}

func TestInteractiveProvider_DefaultClientID(t *testing.T) {
	p, err := NewInteractiveProvider("azure-interactive", &schema.Provider{
		Kind: "azure/interactive",
		Spec: map[string]interface{}{
			"tenant_id": "tenant-123",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, defaultAzureClientID, p.clientID, "defaults to the Azure CLI public client, matching az login")
}
