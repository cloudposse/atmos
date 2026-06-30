package emulator

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// errBoom is a sentinel used to assert error propagation from the seams.
var errBoom = errors.New("boom")

// fakeManager is an in-test emulatorManager that records calls and returns
// canned results, so the executor/resolver/yamlfunc paths can be exercised
// without a container runtime. It mirrors the fakeResolver pattern used in
// pkg/auth/identities/emulator.
type fakeManager struct {
	upEndpoint emu.Endpoint
	upErr      error
	downErr    error
	resetErr   error
	psStatuses []emu.Status
	psErr      error
	logsErr    error
	execErr    error
	resEndp    emu.Endpoint
	resProfile emu.Profile
	resErr     error

	upCalls    int
	downCalls  int
	resetCalls int
	logsCalls  int
	resCalls   int
	gotStack   string
	gotName    string
	gotEnv     map[string]string
	gotCommand []string
	gotFollow  bool
}

func (f *fakeManager) Up(_ context.Context, _ *emu.Spec, stack, name string, env map[string]string) (emu.Endpoint, error) {
	f.upCalls++
	f.gotStack, f.gotName, f.gotEnv = stack, name, env
	return f.upEndpoint, f.upErr
}

func (f *fakeManager) Down(_ context.Context, stack, name string) error {
	f.downCalls++
	f.gotStack, f.gotName = stack, name
	return f.downErr
}

func (f *fakeManager) Reset(_ context.Context, _ *emu.Spec, stack, name string) error {
	f.resetCalls++
	f.gotStack, f.gotName = stack, name
	return f.resetErr
}

func (f *fakeManager) Ps(_ context.Context, stack string) ([]emu.Status, error) {
	f.gotStack = stack
	return f.psStatuses, f.psErr
}

func (f *fakeManager) Logs(_ context.Context, stack, name string, follow bool) error {
	f.logsCalls++
	f.gotStack, f.gotName, f.gotFollow = stack, name, follow
	return f.logsErr
}

func (f *fakeManager) Exec(_ context.Context, stack, name string, command []string) error {
	f.gotStack, f.gotName, f.gotCommand = stack, name, command
	return f.execErr
}

func (f *fakeManager) Resolve(_ context.Context, _ *emu.Spec, stack, name string) (emu.Endpoint, emu.Profile, error) {
	f.resCalls++
	f.gotStack, f.gotName = stack, name
	return f.resEndp, f.resProfile, f.resErr
}

// validSection is a minimal component section whose driver is registered (the
// built-in drivers are registered via the package's blank import), so prepare's
// FromComponentSection + Validate succeed.
func validSection() map[string]any { return map[string]any{"driver": "floci/aws"} }

// stubPrepare installs the prepare() seams so that:
//   - initCliConfig succeeds with the given runtime config,
//   - setupComponentAuthForCLI returns authErr (only consulted when Identity is set),
//   - processStacks sets the component section to `section`.
//
// It returns the installed fake manager. All seams are restored on cleanup.
func stubPrepare(t *testing.T, section map[string]any, authErr error, mgr *fakeManager) {
	t.Helper()

	origInit, origAuth, origProc, origNew := initCliConfig, setupComponentAuthForCLI, processStacks, newManager
	t.Cleanup(func() {
		initCliConfig = origInit
		setupComponentAuthForCLI = origAuth
		processStacks = origProc
		newManager = origNew
	})

	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		var cfg schema.AtmosConfiguration
		cfg.Container.Runtime.Provider = "podman"
		cfg.Container.Runtime.AutoStart = true
		return cfg, nil
	}
	setupComponentAuthForCLI = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
		return nil, authErr
	}
	processStacks = func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
		info.ComponentSection = section
		return info, nil
	}
	newManager = func(_ string, _ bool) emulatorManager { return mgr }
}

func baseInfo() *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "aws", ComponentEnvList: []string{"FOO=bar"}}
}

// stubConfirmReset replaces the reset confirmation prompt seam with a canned
// (confirmed, err) result so the non-force reset path is testable without a TTY.
func stubConfirmReset(t *testing.T, confirmed bool, err error) {
	t.Helper()
	orig := confirmReset
	t.Cleanup(func() { confirmReset = orig })
	confirmReset = func(string) (bool, error) { return confirmed, err }
}

func TestPrepare_ReadsRuntimeAndSpec(t *testing.T) {
	stubPrepare(t, validSection(), nil, &fakeManager{})

	r, err := prepare(baseInfo())
	require.NoError(t, err)
	assert.Equal(t, "podman", r.runtimePref)
	assert.True(t, r.autoStart)
	assert.Equal(t, "dev", r.stack)
	assert.Equal(t, "aws", r.component)
	assert.Equal(t, "floci/aws", r.spec.Driver)
	assert.Equal(t, map[string]string{"FOO": "bar"}, r.env)
}

func TestPrepare_InitConfigError(t *testing.T) {
	stubPrepare(t, validSection(), nil, &fakeManager{})
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, errBoom
	}
	_, err := prepare(baseInfo())
	require.ErrorIs(t, err, errBoom)
}

func TestPrepare_AuthError(t *testing.T) {
	stubPrepare(t, validSection(), errBoom, &fakeManager{})
	info := baseInfo()
	info.Identity = "aws/emulator" // triggers the auth setup branch.
	_, err := prepare(info)
	require.ErrorIs(t, err, errBoom)
}

func TestPrepare_AbstractSectionRejected(t *testing.T) {
	section := map[string]any{"metadata": map[string]any{"type": "abstract"}}
	stubPrepare(t, section, nil, &fakeManager{})
	_, err := prepare(baseInfo())
	require.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
}

func TestPrepare_InvalidSpecRejected(t *testing.T) {
	stubPrepare(t, map[string]any{}, nil, &fakeManager{}) // no driver -> Validate fails.
	_, err := prepare(baseInfo())
	require.ErrorIs(t, err, errUtils.ErrEmulatorConfigInvalid)
}

func TestExecuteUp_Success(t *testing.T) {
	mgr := &fakeManager{upEndpoint: emu.Endpoint{Host: "localhost", Ports: map[int]int{4566: 54321}}}
	stubPrepare(t, validSection(), nil, mgr)

	require.NoError(t, ExecuteUp(context.Background(), baseInfo()))
	assert.Equal(t, 1, mgr.upCalls)
	assert.Equal(t, "dev", mgr.gotStack)
	assert.Equal(t, "aws", mgr.gotName)
	assert.Equal(t, map[string]string{"FOO": "bar"}, mgr.gotEnv)
}

func TestExecuteUp_SuccessNoURL(t *testing.T) {
	mgr := &fakeManager{upEndpoint: emu.Endpoint{}} // no ports -> URL("http") == "".
	stubPrepare(t, validSection(), nil, mgr)

	require.NoError(t, ExecuteUp(context.Background(), baseInfo()))
	assert.Equal(t, 1, mgr.upCalls)
}

func TestExecuteUp_DryRun(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)
	info := baseInfo()
	info.DryRun = true

	require.NoError(t, ExecuteUp(context.Background(), info))
	assert.Equal(t, 0, mgr.upCalls, "dry-run must not touch the manager")
}

func TestExecuteUp_PrepareError(t *testing.T) {
	stubPrepare(t, validSection(), nil, &fakeManager{})
	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, errBoom
	}
	require.ErrorIs(t, ExecuteUp(context.Background(), baseInfo()), errBoom)
}

func TestExecuteUp_ManagerError(t *testing.T) {
	mgr := &fakeManager{upErr: errBoom}
	stubPrepare(t, validSection(), nil, mgr)

	err := ExecuteUp(context.Background(), baseInfo())
	require.ErrorIs(t, err, errBoom)
	assert.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
}

func TestExecuteDown_Success(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)

	require.NoError(t, ExecuteDown(context.Background(), baseInfo()))
	assert.Equal(t, 1, mgr.downCalls)
	assert.Equal(t, "aws", mgr.gotName)
}

func TestExecuteDown_DryRun(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)
	info := baseInfo()
	info.DryRun = true

	require.NoError(t, ExecuteDown(context.Background(), info))
	assert.Equal(t, 0, mgr.downCalls)
}

func TestExecuteDown_ManagerError(t *testing.T) {
	mgr := &fakeManager{downErr: errBoom}
	stubPrepare(t, validSection(), nil, mgr)

	err := ExecuteDown(context.Background(), baseInfo())
	require.ErrorIs(t, err, errBoom)
	assert.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
}

func TestExecuteReset_Force(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)

	// force=true skips the confirmation prompt and resets directly.
	require.NoError(t, ExecuteReset(context.Background(), baseInfo(), true))
	assert.Equal(t, 1, mgr.resetCalls)
	assert.Equal(t, "dev", mgr.gotStack)
	assert.Equal(t, "aws", mgr.gotName)
}

func TestExecuteReset_ConfirmedPrompt(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)
	stubConfirmReset(t, true, nil)

	// Non-force path: the prompt seam returns confirmed=true, so reset runs.
	require.NoError(t, ExecuteReset(context.Background(), baseInfo(), false))
	assert.Equal(t, 1, mgr.resetCalls)
}

func TestExecuteReset_DeclinedPrompt(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)
	stubConfirmReset(t, false, nil)

	// Declined confirmation must NOT touch the manager (negative-path guard).
	require.NoError(t, ExecuteReset(context.Background(), baseInfo(), false))
	assert.Equal(t, 0, mgr.resetCalls, "declining confirmation must not reset")
}

func TestExecuteReset_PromptError(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)
	stubConfirmReset(t, false, errBoom)

	require.ErrorIs(t, ExecuteReset(context.Background(), baseInfo(), false), errBoom)
	assert.Equal(t, 0, mgr.resetCalls)
}

func TestExecuteReset_DryRun(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)
	info := baseInfo()
	info.DryRun = true

	require.NoError(t, ExecuteReset(context.Background(), info, true))
	assert.Equal(t, 0, mgr.resetCalls, "dry-run must not touch the manager")
}

func TestExecuteReset_ManagerError(t *testing.T) {
	mgr := &fakeManager{resetErr: errBoom}
	stubPrepare(t, validSection(), nil, mgr)

	err := ExecuteReset(context.Background(), baseInfo(), true)
	require.ErrorIs(t, err, errBoom)
	assert.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
}

func TestExecutePs_ListsStatuses(t *testing.T) {
	mgr := &fakeManager{psStatuses: []emu.Status{
		{Name: "aws", Image: "floci/aws:latest", Status: "running", ID: "abc"},
		{Name: "gcp", Image: "floci/gcp:latest", Status: "running", ID: "def"},
	}}
	stubPrepare(t, validSection(), nil, mgr)

	require.NoError(t, ExecutePs(context.Background(), baseInfo()))
	assert.Equal(t, "dev", mgr.gotStack)
}

func TestExecutePs_Empty(t *testing.T) {
	mgr := &fakeManager{psStatuses: nil}
	stubPrepare(t, validSection(), nil, mgr)
	require.NoError(t, ExecutePs(context.Background(), baseInfo()))
}

func TestExecutePs_Error(t *testing.T) {
	mgr := &fakeManager{psErr: errBoom}
	stubPrepare(t, validSection(), nil, mgr)
	err := ExecutePs(context.Background(), baseInfo())
	require.ErrorIs(t, err, errBoom)
	assert.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
}

// stubListSeams installs the seams ExecuteList consults directly (it bypasses
// prepare(), so only initCliConfig and newManager are needed). All seams are
// restored on cleanup.
func stubListSeams(t *testing.T, mgr *fakeManager) {
	t.Helper()

	origInit, origNew := initCliConfig, newManager
	t.Cleanup(func() {
		initCliConfig = origInit
		newManager = origNew
	})

	initCliConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		var cfg schema.AtmosConfiguration
		cfg.Container.Runtime.Provider = "podman"
		return cfg, nil
	}
	newManager = func(_ string, _ bool) emulatorManager { return mgr }
}

func TestExecuteList_ListsStatuses(t *testing.T) {
	mgr := &fakeManager{psStatuses: []emu.Status{
		{Name: "aws", Stack: "dev", Image: "docker.io/floci/floci@sha256:abcdef", Status: "running", ID: "628b3ae8a73c92b361"},
		{Name: "gcp", Stack: "dev", Image: "floci/gcp:latest", Status: "exited", ID: "deadbeef0001"},
	}}
	stubListSeams(t, mgr)

	require.NoError(t, ExecuteList(context.Background(), baseInfo()))
	assert.Equal(t, "dev", mgr.gotStack)
}

func TestExecuteList_AllStacksWhenStackEmpty(t *testing.T) {
	mgr := &fakeManager{psStatuses: []emu.Status{
		{Name: "aws", Stack: "dev", Image: "floci/floci:latest", Status: "running", ID: "abc"},
	}}
	stubListSeams(t, mgr)

	info := &schema.ConfigAndStacksInfo{} // no stack set.
	require.NoError(t, ExecuteList(context.Background(), info))
	assert.Empty(t, mgr.gotStack, "empty stack is passed through so Ps lists all stacks")
}

func TestExecuteList_Empty(t *testing.T) {
	mgr := &fakeManager{psStatuses: nil}
	stubListSeams(t, mgr)
	require.NoError(t, ExecuteList(context.Background(), baseInfo()))
}

func TestExecuteList_Error(t *testing.T) {
	mgr := &fakeManager{psErr: errBoom}
	stubListSeams(t, mgr)
	err := ExecuteList(context.Background(), baseInfo())
	require.ErrorIs(t, err, errBoom)
	assert.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
}

func TestShortImage(t *testing.T) {
	assert.Equal(t, "floci/floci", shortImage("docker.io/floci/floci@sha256:c88ec20bf221"))
	assert.Equal(t, "floci/aws:latest", shortImage("floci/aws:latest"))
	assert.Equal(t, "floci/floci", shortImage("floci/floci@sha256:deadbeef"))
}

func TestShortID(t *testing.T) {
	assert.Equal(t, "628b3ae8a73c", shortID("628b3ae8a73c92b361b8b5a9fe584cd"))
	assert.Equal(t, "abc", shortID("abc"))
}

func TestStatusDot(t *testing.T) {
	styles := theme.GetCurrentStyles()
	const dot = "●"
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{name: "running is success", status: "running", want: styles.Success.Render(dot)},
		{name: "up is success", status: "Up 3 minutes", want: styles.Success.Render(dot)},
		{name: "healthy is success", status: "Up (healthy)", want: styles.Success.Render(dot)},
		// "unhealthy" must NOT match the "healthy" substring -> muted.
		{name: "unhealthy is muted", status: "Up (unhealthy)", want: styles.Muted.Render(dot)},
		{name: "exited is muted", status: "Exited (0)", want: styles.Muted.Render(dot)},
		{name: "dead is muted", status: "Dead", want: styles.Muted.Render(dot)},
		{name: "unknown is muted", status: "created", want: styles.Muted.Render(dot)},
		{name: "empty is muted", status: "", want: styles.Muted.Render(dot)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusDot(tt.status, styles)
			assert.Contains(t, got, dot, "always renders the dot rune")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExecuteList_TTYStyledTable(t *testing.T) {
	// Force the TTY branch of renderEmulatorList so the styled-table + statusDot
	// path (not just the plain tab-separated path) is exercised.
	orig := viper.Get("force-tty")
	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", orig) })

	mgr := &fakeManager{psStatuses: []emu.Status{
		{Name: "aws", Stack: "dev", Image: "docker.io/floci/floci@sha256:abcdef", Status: "running", ID: "628b3ae8a73c92b361"},
		{Name: "gcp", Stack: "dev", Image: "floci/gcp:latest", Status: "exited", ID: "deadbeef0001"},
	}}
	stubListSeams(t, mgr)

	require.NoError(t, ExecuteList(context.Background(), baseInfo()))
	assert.Equal(t, "dev", mgr.gotStack)
}

func TestExecuteList_TTYEmptyWithStack(t *testing.T) {
	// TTY branch is irrelevant when empty, but exercise the stack-scoped empty message.
	orig := viper.Get("force-tty")
	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", orig) })

	mgr := &fakeManager{psStatuses: nil}
	stubListSeams(t, mgr)
	require.NoError(t, ExecuteList(context.Background(), baseInfo()))
}

func TestExecuteLogs_Success(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)

	require.NoError(t, ExecuteLogs(context.Background(), baseInfo()))
	assert.Equal(t, 1, mgr.logsCalls)
	assert.False(t, mgr.gotFollow)
}

func TestExecuteLogs_Error(t *testing.T) {
	mgr := &fakeManager{logsErr: errBoom}
	stubPrepare(t, validSection(), nil, mgr)
	err := ExecuteLogs(context.Background(), baseInfo())
	require.ErrorIs(t, err, errBoom)
	assert.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
}

func TestExecuteExec_Success(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)

	require.NoError(t, ExecuteExec(context.Background(), baseInfo(), []string{"sh", "-c", "echo hi"}))
	assert.Equal(t, []string{"sh", "-c", "echo hi"}, mgr.gotCommand)
}

func TestExecuteExec_DryRun(t *testing.T) {
	mgr := &fakeManager{}
	stubPrepare(t, validSection(), nil, mgr)
	info := baseInfo()
	info.DryRun = true

	require.NoError(t, ExecuteExec(context.Background(), info, []string{"sh", "-c", "echo hi"}))
	assert.Nil(t, mgr.gotCommand, "dry-run must not touch the manager")
}

func TestExecuteExec_Error(t *testing.T) {
	mgr := &fakeManager{execErr: errBoom}
	stubPrepare(t, validSection(), nil, mgr)
	err := ExecuteExec(context.Background(), baseInfo(), nil)
	require.ErrorIs(t, err, errBoom)
	assert.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
}

func TestExecuteExec_PrepareError(t *testing.T) {
	stubPrepare(t, map[string]any{}, nil, &fakeManager{}) // invalid spec.
	err := ExecuteExec(context.Background(), baseInfo(), nil)
	require.ErrorIs(t, err, errUtils.ErrEmulatorConfigInvalid)
}

func TestIsAbstractSection(t *testing.T) {
	assert.True(t, isAbstractSection(map[string]any{"metadata": map[string]any{"type": "abstract"}}))
	assert.False(t, isAbstractSection(map[string]any{"metadata": map[string]any{"type": "real"}}))
	assert.False(t, isAbstractSection(map[string]any{"driver": "floci/aws"}))
	assert.False(t, isAbstractSection(map[string]any{}))
}
