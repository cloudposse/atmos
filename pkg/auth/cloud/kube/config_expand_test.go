package kube

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExpandKubeconfigPath covers the ~ and environment-variable expansion that
// keeps committable, multi-developer integration configs portable. This is the
// central fix for the config-path half of issue #3386: before, a configured
// spec.cluster.kubeconfig.path of "~/..." or "${HOME}/..." was used verbatim.
func TestExpandKubeconfigPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	// A dedicated env var the test controls, so assertions don't depend on the
	// developer's or CI runner's actual environment.
	t.Setenv("ATMOS_TEST_KUBE_DIR", filepath.Join("custom", "kube"))

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty stays empty",
			input: "",
			want:  "",
		},
		{
			name:  "absolute path unchanged",
			input: filepath.Join(home, "already", "absolute"),
			want:  filepath.Join(home, "already", "absolute"),
		},
		{
			name:  "bare tilde resolves to home",
			input: "~",
			want:  home,
		},
		{
			name:  "tilde slash prefix resolves under home",
			input: "~/.config/atmos/kube/prod",
			want:  filepath.Join(home, ".config", "atmos", "kube", "prod"),
		},
		{
			name:  "dollar brace env var expands",
			input: "${HOME}/.config/atmos/kube/qa",
			want:  filepath.Join(home, ".config", "atmos", "kube", "qa"),
		},
		{
			name:  "bare dollar env var expands",
			input: "$HOME/kube/dev",
			want:  filepath.Join(home, "kube", "dev"),
		},
		{
			name:  "custom env var expands mid-path",
			input: "/opt/${ATMOS_TEST_KUBE_DIR}/config",
			want:  filepath.Join("/opt", "custom", "kube", "config"),
		},
		{
			name:  "unknown env var expands to empty like shell",
			input: "/opt/kube-${ATMOS_TEST_KUBE_UNSET}config",
			want:  filepath.Join("/opt", "kube-config"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandKubeconfigPath(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestNewKubeconfigManager_ExpandsPath confirms the expansion runs through the
// single chokepoint every caller uses (integration config path, --kubeconfig
// flag, KUBECONFIG/ATMOS_KUBECONFIG env), so GetPath reports the real target.
func TestNewKubeconfigManager_ExpandsPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	mgr, err := NewKubeconfigManager("~/.config/atmos/kube/prod", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "atmos", "kube", "prod"), mgr.GetPath())

	mgr, err = NewKubeconfigManager("${HOME}/.config/atmos/kube/qa", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "atmos", "kube", "qa"), mgr.GetPath())
}

// TestNewKubeconfigManager_EmptyPathUsesDefault guards that the expansion branch
// didn't disturb the empty-path default (XDG ~/.config/atmos/kube/config).
func TestNewKubeconfigManager_EmptyPathUsesDefault(t *testing.T) {
	mgr, err := NewKubeconfigManager("", "")
	require.NoError(t, err)

	defaultPath, err := DefaultKubeconfigPath()
	require.NoError(t, err)
	assert.Equal(t, defaultPath, mgr.GetPath())
}
