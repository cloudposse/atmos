package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
		name    string
		input   string
		want    string
		wantErr bool
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
			// Unset variables must fail loudly, not silently drop a path
			// segment and redirect the write to an unintended location.
			name:    "unset env var is an error",
			input:   "/opt/kube-${ATMOS_TEST_KUBE_UNSET}config",
			wantErr: true,
		},
		{
			// A KUBECONFIG-style multi-file list reduces to its first entry,
			// matching kubectl's write target and the AWS CLI default.
			name:  "multi-file list reduces to first entry",
			input: strings.Join([]string{filepath.Join(home, "primary"), filepath.Join(home, "secondary")}, string(os.PathListSeparator)),
			want:  filepath.Join(home, "primary"),
		},
		{
			// A leading separator yields an empty first entry; kubectl skips
			// empty filenames, so the first usable file must be selected.
			name:  "leading separator skips empty entry",
			input: string(os.PathListSeparator) + filepath.Join(home, "primary"),
			want:  filepath.Join(home, "primary"),
		},
		{
			// A list with only separators has no usable entry and must error
			// rather than write through an empty path.
			name:    "list with only separators is an error",
			input:   string(os.PathListSeparator) + string(os.PathListSeparator),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandKubeconfigPath(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrKubeconfigPath)
				return
			}
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
