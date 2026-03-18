package browser

import (
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRunner records command invocations for testing.
type mockRunner struct {
	mu        sync.Mutex
	calls     []mockCall
	returnErr error
}

type mockCall struct {
	Name string
	Args []string
}

func (m *mockRunner) Run(name string, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, mockCall{Name: name, Args: args})
	return m.returnErr
}

func (m *mockRunner) lastCall() mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return mockCall{}
	}
	return m.calls[len(m.calls)-1]
}

func TestDefaultOpener_Open(t *testing.T) {
	runner := &mockRunner{}
	opener := &defaultOpener{runner: runner}

	err := opener.Open("https://example.com")
	require.NoError(t, err)

	call := runner.lastCall()
	switch runtime.GOOS {
	case "darwin":
		assert.Equal(t, "open", call.Name)
		assert.Equal(t, []string{"https://example.com"}, call.Args)
	case "linux":
		assert.Equal(t, "xdg-open", call.Name)
		assert.Equal(t, []string{"https://example.com"}, call.Args)
	case "windows":
		assert.Equal(t, "rundll32", call.Name)
		assert.Equal(t, []string{"url.dll,FileProtocolHandler", "https://example.com"}, call.Args)
	}
}

func TestIsolatedOpener_Open_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}

	runner := &mockRunner{}
	opener := &isolatedOpener{
		chrome: &ChromeInfo{
			Path:         "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			UseMacOSOpen: true,
			AppName:      "Google Chrome",
		},
		sessionDir: "/tmp/test-session",
		runner:     runner,
	}

	err := opener.Open("https://example.com")
	require.NoError(t, err)

	call := runner.lastCall()
	assert.Equal(t, "open", call.Name)
	assert.Equal(t, "-na", call.Args[0])
	assert.Equal(t, "Google Chrome", call.Args[1])
	assert.Equal(t, "--args", call.Args[2])
	assert.Equal(t, "--user-data-dir=/tmp/test-session", call.Args[3])
	assert.Equal(t, "https://example.com", call.Args[4])
}

func TestIsolatedOpener_Open_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-specific test")
	}

	runner := &mockRunner{}
	opener := &isolatedOpener{
		chrome: &ChromeInfo{
			Path: "/usr/bin/google-chrome",
		},
		sessionDir: "/tmp/test-session",
		runner:     runner,
	}

	err := opener.Open("https://example.com")
	require.NoError(t, err)

	call := runner.lastCall()
	assert.Equal(t, "/usr/bin/google-chrome", call.Name)
	assert.Equal(t, "--user-data-dir=/tmp/test-session", call.Args[0])
	assert.Equal(t, "https://example.com", call.Args[1])
}

func TestNew_DefaultOpener(t *testing.T) {
	runner := &mockRunner{}
	opener := New(WithCommandRunner(runner))
	assert.IsType(t, &defaultOpener{}, opener, "without isolation, should return defaultOpener")
}

func TestNew_IsolatedOpener_FallsBackWithoutChrome(t *testing.T) {
	// DetectChrome may or may not find Chrome depending on the test environment.
	// The key behavior: New() should never panic and should always return a valid Opener.
	runner := &mockRunner{}
	opener := New(WithIsolatedSession("/tmp/test-session"), WithCommandRunner(runner))
	assert.NotNil(t, opener, "should always return a non-nil opener")
}
