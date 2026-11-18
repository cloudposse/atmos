package prompter

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingPrompter records the arguments it was called with.
type recordingPrompter struct {
	calls        []string
	lastPrompt   string
	lastDefault  string
	lastOptions  []string
	chooseResult string
}

func (r *recordingPrompter) RequestSecurityCode(pattern string) (string, error) {
	r.calls = append(r.calls, "RequestSecurityCode")
	r.lastPrompt = pattern
	return "123456", nil
}

func (r *recordingPrompter) ChooseWithDefault(pr, def string, options []string) (string, error) {
	r.calls = append(r.calls, "ChooseWithDefault")
	r.lastPrompt, r.lastDefault, r.lastOptions = pr, def, options
	return r.chooseResult, nil
}

func (r *recordingPrompter) Choose(pr string, options []string) (int, error) {
	r.calls = append(r.calls, "Choose")
	r.lastPrompt, r.lastOptions = pr, options
	return len(options) - 1, nil
}

func (r *recordingPrompter) StringRequired(pr string) (string, error) {
	r.calls = append(r.calls, "StringRequired")
	r.lastPrompt = pr
	return "required", nil
}

func (r *recordingPrompter) String(pr, def string) (string, error) {
	r.calls = append(r.calls, "String")
	r.lastPrompt, r.lastDefault = pr, def
	return "value", nil
}

func (r *recordingPrompter) Password(pr string) (string, error) {
	r.calls = append(r.calls, "Password")
	r.lastPrompt = pr
	return "secret", nil
}

func (r *recordingPrompter) Display(pr string) {
	r.calls = append(r.calls, "Display")
	r.lastPrompt = pr
}

// useRecordingPrompter installs a recording prompter for the duration of a test.
func useRecordingPrompter(t *testing.T) *recordingPrompter {
	t.Helper()
	orig := ActivePrompter
	rec := &recordingPrompter{chooseResult: "picked"}
	SetPrompter(rec)
	t.Cleanup(func() { SetPrompter(orig) })
	return rec
}

func TestPackageFunctionsDelegateToActivePrompter(t *testing.T) {
	rec := useRecordingPrompter(t)

	code, err := RequestSecurityCode("000000")
	require.NoError(t, err)
	assert.Equal(t, "123456", code)
	assert.Equal(t, "000000", rec.lastPrompt)

	index, err := Choose("Pick", []string{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, 1, index)
	assert.Equal(t, []string{"a", "b"}, rec.lastOptions)

	required, err := StringRequired("Username")
	require.NoError(t, err)
	assert.Equal(t, "required", required)
	assert.Equal(t, "Username", rec.lastPrompt)

	value, err := String("Region", "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "value", value)
	assert.Equal(t, "us-east-1", rec.lastDefault)

	secret, err := Password("Password")
	require.NoError(t, err)
	assert.Equal(t, "secret", secret)
	assert.Equal(t, "Password", rec.lastPrompt)

	Display("hello")
	assert.Equal(t, "hello", rec.lastPrompt)

	assert.Equal(t, []string{"RequestSecurityCode", "Choose", "StringRequired", "String", "Password", "Display"}, rec.calls)
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
			rec := useRecordingPrompter(t)

			got, err := ChooseWithDefault("Pick a role", tt.def, tt.options)
			require.NoError(t, err)
			assert.Equal(t, "picked", got)
			assert.Equal(t, "Pick a role", rec.lastPrompt)
			assert.Equal(t, tt.wantDefault, rec.lastDefault)
			assert.Equal(t, tt.options, rec.lastOptions)
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
	origStderr := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr })

	NewCli().Display("Open your authenticator app")
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
