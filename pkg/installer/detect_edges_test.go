package installer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectCustomCellarProbe(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, output string
		want         Kind
	}{
		{"verified", "/custom/Cellar\n", Homebrew},
		{"different cellar", "/other/Cellar", Unknown},
		{"malformed", "warning: unavailable\n/custom/Cellar", Unknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newSystem("/custom/Cellar/atmos/1.2.3/bin/atmos")
			s.goos = "darwin"
			s.outputs["brew --cellar"] = tt.output
			got := Detect(context.Background(), WithSystem(s))
			assert.Equal(t, tt.want, got.Kind)
			assert.Equal(t, []string{"brew --cellar"}, s.calls)
		})
	}
}

func TestDetectAquaMalformedAssets(t *testing.T) {
	t.Parallel()
	for _, relative := range []string{
		"v1.2.3/atmos",
		"v1.2.3/archive/atmos_1.2.3_linux_unknown",
	} {
		t.Run(relative, func(t *testing.T) {
			t.Parallel()
			s := newSystem("/aqua/pkgs/github_release/github.com/cloudposse/atmos/" + relative)
			s.env["AQUA_ROOT_DIR"] = "/aqua"
			assert.Equal(t, Unknown, Detect(context.Background(), WithSystem(s)).Kind)
		})
	}
}

func TestDetectUnavailableRoots(t *testing.T) {
	t.Parallel()
	t.Run("working directory unavailable", func(t *testing.T) {
		t.Parallel()
		s := newSystem("/work/tools/bin/cloudposse/atmos/1.2.3/atmos")
		s.absErr = errUnavailable
		got := Detect(context.Background(), WithSystem(s), WithNativeRoots("tools"))
		assert.Equal(t, Unknown, got.Kind)
		assert.Empty(t, got.UpgradeHint("1.2.4").Command)
	})
	t.Run("home unavailable without configured root", func(t *testing.T) {
		t.Parallel()
		s := newSystem("/work/.asdf/installs/atmos/1.2.3/atmos")
		s.home = ""
		assert.Equal(t, Unknown, Detect(context.Background(), WithSystem(s)).Kind)
	})
	t.Run("configured root works without home", func(t *testing.T) {
		t.Parallel()
		s := newSystem("/tools/installs/atmos/1.2.3/atmos")
		s.home = ""
		s.env["ASDF_DATA_DIR"] = "/tools"
		assert.Equal(t, ASDF, Detect(context.Background(), WithSystem(s)).Kind)
	})
}

func TestDetectCanceledCellarProbeStopsDetection(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newSystem("/custom/Cellar/atmos/1.2.3/bin/atmos")
	s.probe = func(context.Context, string, ...string) ([]byte, error) {
		cancel()
		return []byte("/custom/Cellar"), nil
	}
	got := Detect(ctx, WithSystem(s))
	assert.Equal(t, Unknown, got.Kind)
	assert.Empty(t, got.UpgradeHint("1.2.4").Command)
	assert.Equal(t, []string{"brew --cellar"}, s.calls, "cancellation must prevent package probes")
}
