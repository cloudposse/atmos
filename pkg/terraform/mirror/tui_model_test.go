package mirror

import (
	"errors"
	"testing"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMirrorModel_PlatformsFloor(t *testing.T) {
	assert.Equal(t, 1, newMirrorModel(0, false).platforms, "platform count floors at 1")
	assert.Equal(t, 1, newMirrorModel(-5, false).platforms)
	assert.Equal(t, 3, newMirrorModel(3, false).platforms)
}

func TestMirrorModel_Init(t *testing.T) {
	m := newMirrorModel(1, false)
	assert.NotNil(t, m.Init(), "Init starts the spinner ticker")
}

func TestMirrorModel_SetWidthClamps(t *testing.T) {
	m := newMirrorModel(1, false)

	m.Update(tea.WindowSizeMsg{Width: 50})
	assert.Equal(t, 50, m.width)

	m.Update(tea.WindowSizeMsg{Width: mirrorMaxWidth + 1000})
	assert.Equal(t, mirrorMaxWidth, m.width, "width is clamped to the maximum")
}

func TestIsQuitKey(t *testing.T) {
	for _, key := range []string{"ctrl+c", "esc", "q"} {
		assert.True(t, isQuitKey(key), key)
	}
	for _, key := range []string{"a", "enter", "Q", ""} {
		assert.False(t, isQuitKey(key), key)
	}
}

func TestMirrorModel_QuitKeyReturnsQuit(t *testing.T) {
	m := newMirrorModel(1, false)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestMirrorModel_HandlePkgDone(t *testing.T) {
	m := newMirrorModel(2, false)
	model, cmd := m.Update(pkgDoneMsg{provider: "hashicorp/aws", platform: "linux_amd64"})
	require.Same(t, m, model)
	require.NotNil(t, cmd, "advancing the progress bar returns a command")
	assert.Equal(t, 1, m.count)
	assert.Equal(t, "hashicorp/aws linux_amd64", m.current)

	m.Update(pkgDoneMsg{provider: "hashicorp/null", platform: "darwin_arm64"})
	assert.Equal(t, 2, m.count)
	assert.Equal(t, "hashicorp/null darwin_arm64", m.current)
}

func TestMirrorModel_HandleComponentDone_Error(t *testing.T) {
	m := newMirrorModel(1, false)
	m.Update(componentDoneMsg{target: Target{Component: "vpc", Stack: "plat-ue2-prod"}, err: errors.New("boom")})
	require.Len(t, m.failed, 1)
	assert.Equal(t, "vpc (plat-ue2-prod)", m.failed[0])
}

func TestMirrorModel_HandleComponentDone_FallbackNoPackages(t *testing.T) {
	m := newMirrorModel(1, false)
	// No per-package events emitted: the component still produces a fallback line and
	// is not recorded as failed.
	_, _ = m.Update(componentDoneMsg{target: Target{Component: "vpc", Stack: "prod"}, mirrored: 0})
	assert.Empty(t, m.failed)
}

func TestMirrorModel_AllDoneQuits(t *testing.T) {
	m := newMirrorModel(1, false)
	_, cmd := m.Update(allDoneMsg{})
	assert.True(t, m.done)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestMirrorModel_UpdateAnimations(t *testing.T) {
	m := newMirrorModel(1, false)
	// Spinner tick and progress frame are forwarded to the sub-models without panic.
	model, _ := m.Update(spinner.TickMsg{})
	assert.Same(t, m, model)
	model, _ = m.Update(progress.FrameMsg{})
	assert.Same(t, m, model)
}

func TestMirrorModel_View(t *testing.T) {
	m := newMirrorModel(1, false)
	m.setWidth(80)

	// Before any package, the label defaults to "providers".
	assert.Contains(t, m.View(), "Mirroring")
	assert.Contains(t, m.View(), "providers")

	m.Update(pkgDoneMsg{provider: "hashicorp/aws", platform: "linux_amd64"})
	view := m.View()
	assert.Contains(t, view, "Mirroring")
	assert.Contains(t, view, "hashicorp/aws")

	// A done model renders nothing.
	m.done = true
	assert.Empty(t, m.View())
}

func TestMaxInt(t *testing.T) {
	assert.Equal(t, 5, maxInt(5, 3))
	assert.Equal(t, 5, maxInt(3, 5))
	assert.Equal(t, 0, maxInt(0, 0))
}

func TestMaxWidthOr(t *testing.T) {
	assert.Equal(t, mirrorMaxWidth, maxWidthOr(0))
	assert.Equal(t, mirrorMaxWidth, maxWidthOr(-1))
	assert.Equal(t, 42, maxWidthOr(42))
}
