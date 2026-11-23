package prompter

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

// useMockPrompter installs a mock prompter for the duration of a test. The controller verifies the
// expectations when the test ends.
func useMockPrompter(t *testing.T) *mocks.MockPrompter {
	t.Helper()
	orig := ActivePrompter
	pr := mocks.NewMockPrompter(gomock.NewController(t))
	SetPrompter(pr)
	t.Cleanup(func() { SetPrompter(orig) })
	return pr
}

func TestPackageFunctionsDelegateToActivePrompter(t *testing.T) {
	pr := useMockPrompter(t)
	// Each package function delegates exactly once, in this order, with the caller's arguments.
	gomock.InOrder(
		pr.EXPECT().RequestSecurityCode("000000").Return("123456", nil).Times(1),
		pr.EXPECT().Choose("Pick", []string{"a", "b"}).Return(1, nil).Times(1),
		pr.EXPECT().StringRequired("Username").Return("required", nil).Times(1),
		pr.EXPECT().String("Region", "us-east-1").Return("value", nil).Times(1),
		pr.EXPECT().Password("Password").Return("secret", nil).Times(1),
		pr.EXPECT().Display("hello").Times(1),
	)

	code, err := RequestSecurityCode("000000")
	require.NoError(t, err)
	assert.Equal(t, "123456", code)

	index, err := Choose("Pick", []string{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, 1, index)

	required, err := StringRequired("Username")
	require.NoError(t, err)
	assert.Equal(t, "required", required)

	value, err := String("Region", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "value", value)

	secret, err := Password("Password")
	require.NoError(t, err)
	assert.Equal(t, "secret", secret)

	Display("hello")
}

func TestChooseWithDefault(t *testing.T) {
	tests := []struct {
		name        string
		def         string
		options     []string
		wantDefault string
	}{
		{name: "explicit default kept", def: "b", options: []string{"a", "b"}, wantDefault: "b"},
		{name: "empty default uses first option", def: "", options: []string{"a", "b"}, wantDefault: "a"},
		{name: "empty default without options", def: "", options: nil, wantDefault: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := useMockPrompter(t)
			// The active prompter receives the prompt, the resolved default and the options exactly once.
			pr.EXPECT().ChooseWithDefault("Pick a role", tt.wantDefault, tt.options).Return("picked", nil).Times(1)

			got, err := ChooseWithDefault("Pick a role", tt.def, tt.options)
			require.NoError(t, err)
			assert.Equal(t, "picked", got)
		})
	}
}

func TestNewPinentryPrompter(t *testing.T) {
	p := NewPinentryPrompter("pinentry-mac")

	runner, ok := p.Runner.(*RealPinentryRunner)
	require.True(t, ok)
	assert.Equal(t, "pinentry-mac", runner.PinentryBin)
	assert.IsType(t, &CliPrompter{}, p.DefaultPrompter)
}

func TestCliPrompterDisplay(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	// Both ends are released even when an assertion fails; closing an already closed end is harmless.
	t.Cleanup(func() {
		_ = r.Close()
		_ = w.Close()
	})
	origStderr := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr })

	NewCli().Display("Open your authenticator app")
	// Close the write end before reading so the read reaches EOF.
	require.NoError(t, w.Close())
	os.Stderr = origStderr

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "Open your authenticator app\n", string(out))
}

func TestRealPinentryRunner(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	tests := []struct {
		mode    string
		want    string
		wantErr string
	}{
		{mode: "ok", want: "424242"},
		{mode: "cancel", wantErr: "Operation cancelled"},
		{mode: "crash", wantErr: "exit status 2"},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			t.Setenv(fakePinentryEnv, tt.mode)

			out, err := NewRealPinentryRunner(exe).Run(expectedPinentryInput)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, out)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, out)
		})
	}
}

func TestPinentryPrompterEndToEnd(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	tests := []struct {
		mode    string
		want    string
		wantErr bool
	}{
		{mode: "ok", want: "424242"},
		{mode: "cancel", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			t.Setenv(fakePinentryEnv, tt.mode)
			got, err := NewPinentryPrompter(exe).RequestSecurityCode("000000")
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRealPinentryRunnerMissingBinary(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "pinentry-does-not-exist")

	out, err := NewRealPinentryRunner(missing).Run(expectedPinentryInput)
	require.Error(t, err)
	assert.Empty(t, out)
}
