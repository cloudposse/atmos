package installer

import (
	"context"
	"errors"
	"path"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errUnavailable = errors.New("unavailable")

type fakeSystem struct {
	goos          string
	executable    string
	home          string
	env           map[string]string
	links         map[string]string
	managers      map[string]bool
	outputs       map[string]string
	info          *debug.BuildInfo
	executableErr error
	resolveErr    error
	absErr        error
	probe         func(context.Context, string, ...string) ([]byte, error)
	calls         []string
}

func newSystem(executable string) *fakeSystem {
	return &fakeSystem{
		goos: "linux", executable: executable, home: "/home/test",
		env: map[string]string{}, links: map[string]string{}, outputs: map[string]string{},
		managers: map[string]bool{
			"brew": true, "mise": true, "asdf": true, "scoop": true,
			"aqua": true, "nix": true, "go": true, "dpkg-query": true, "rpm": true,
			"apk": true, "apt-get": true, "dnf": true, "yum": true, "atmos": true,
		},
	}
}

func (s *fakeSystem) GOOS() string                        { return s.goos }
func (s *fakeSystem) Getenv(key string) string            { return s.env[key] }
func (s *fakeSystem) Executable() (string, error)         { return s.executable, s.executableErr }
func (s *fakeSystem) UserHomeDir() (string, error)        { return s.home, nil }
func (s *fakeSystem) BuildInfo() (*debug.BuildInfo, bool) { return s.info, s.info != nil }
func (s *fakeSystem) EvalSymlinks(p string) (string, error) {
	if s.resolveErr != nil {
		return "", s.resolveErr
	}
	if target, ok := s.links[p]; ok {
		return target, nil
	}
	return p, nil
}

func (s *fakeSystem) Abs(p string) (string, error) {
	if s.absErr != nil {
		return "", s.absErr
	}
	if path.IsAbs(p) || (len(p) > 1 && p[1] == ':') {
		return p, nil
	}
	return path.Join("/work", p), nil
}

func (s *fakeSystem) LookPath(name string) (string, error) {
	if s.managers[name] {
		return "/commands/" + name, nil
	}
	return "", errUnavailable
}

func (s *fakeSystem) Output(ctx context.Context, executable string, args ...string) ([]byte, error) {
	key := path.Base(executable) + " " + strings.Join(args, " ")
	s.calls = append(s.calls, key)
	if s.probe != nil {
		return s.probe(ctx, executable, args...)
	}
	if output, ok := s.outputs[key]; ok {
		return []byte(output), nil
	}
	return nil, errUnavailable
}

func TestDetectInstallationLayouts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, goos, executable string
		env                    map[string]string
		roots                  []string
		want                   Kind
		global                 bool
	}{
		{name: "Apple Silicon", goos: "darwin", executable: "/opt/homebrew/Cellar/atmos/1.2.3/bin/atmos", want: Homebrew},
		{name: "Intel", goos: "darwin", executable: "/usr/local/Cellar/atmos/1.2.3/bin/atmos", want: Homebrew},
		{name: "Linuxbrew", executable: "/home/linuxbrew/.linuxbrew/Cellar/atmos/1.2.3/bin/atmos", want: Homebrew},
		{name: "custom brew prefix", executable: "/custom/brew/Cellar/atmos/1.2.3/bin/atmos", env: map[string]string{"HOMEBREW_PREFIX": "/custom/brew"}, want: Homebrew},
		{name: "custom cellar", executable: "/custom/packages/atmos/1.2.3/bin/atmos", env: map[string]string{"HOMEBREW_CELLAR": "/custom/packages"}, want: Homebrew},
		{name: "mise", executable: "/home/test/.local/share/mise/installs/atmos/1.2.3/atmos", want: Mise},
		{name: "mise XDG", executable: "/data/mise/installs/atmos/1.2.3/bin/atmos", env: map[string]string{"XDG_DATA_HOME": "/data"}, want: Mise},
		{name: "mise custom data", executable: "/data/installs/atmos/1.2.3/atmos", env: map[string]string{"MISE_DATA_DIR": "/data"}, want: Mise},
		{name: "mise custom installs", executable: "/installs/atmos/1.2.3/atmos", env: map[string]string{"MISE_INSTALLS_DIR": "/installs"}, want: Mise},
		{name: "asdf", executable: "/home/test/.asdf/installs/atmos/1.2.3/bin/atmos", want: ASDF},
		{name: "asdf custom", executable: "/tools/installs/atmos/1.2.3/bin/atmos", env: map[string]string{"ASDF_DATA_DIR": "/tools"}, want: ASDF},
		{name: "scoop user", goos: "windows", executable: `C:\Users\Test\scoop\apps\atmos\1.2.3\atmos.exe`, env: map[string]string{"SCOOP": `C:\Users\Test\scoop`}, want: Scoop},
		{name: "scoop global case insensitive", goos: "windows", executable: `C:\PROGRAMDATA\Scoop\Apps\Atmos\1.2.3\ATMOS.EXE`, want: Scoop, global: true},
		{name: "scoop global custom", goos: "windows", executable: `D:\Tools\apps\atmos\1.2.3\atmos.exe`, env: map[string]string{"SCOOP_GLOBAL": `D:\Tools`}, want: Scoop, global: true},
		{name: "mise windows", goos: "windows", executable: `C:\Data\mise\installs\atmos\1.2.3\atmos.exe`, env: map[string]string{"LOCALAPPDATA": `C:\Data`}, want: Mise},
		{name: "aqua", executable: "/home/test/.local/share/aquaproj-aqua/pkgs/github_release/github.com/cloudposse/atmos/v1.2.3/atmos_linux_amd64/atmos", want: Aqua},
		{name: "aqua custom", executable: "/aqua/pkgs/github_release/github.com/cloudposse/atmos/v1.2.3/archive/atmos", env: map[string]string{"AQUA_ROOT_DIR": "/aqua"}, want: Aqua},
		{name: "aqua raw asset", executable: "/aqua/pkgs/github_release/github.com/cloudposse/atmos/v1.2.3/atmos_1.2.3_linux_amd64/atmos_1.2.3_linux_amd64", env: map[string]string{"AQUA_ROOT_DIR": "/aqua"}, want: Aqua},
		{name: "aqua windows raw asset", goos: "windows", executable: `C:\Data\aquaproj-aqua\pkgs\github_release\github.com\cloudposse\atmos\v1.2.3\atmos_1.2.3_windows_amd64.exe\atmos_1.2.3_windows_amd64.exe`, env: map[string]string{"LOCALAPPDATA": `C:\Data`}, want: Aqua},
		{name: "mise windows XDG", goos: "windows", executable: `D:\Data\mise\installs\atmos\1.2.3\atmos.exe`, env: map[string]string{"LOCALAPPDATA": `C:\Data`, "XDG_DATA_HOME": `D:\Data`}, want: Mise},
		{name: "nix", executable: "/nix/store/01234567890123456789012345678901-atmos-1.2.3/bin/atmos", want: Nix},
		{name: "native legacy", executable: "/home/test/.atmos/bin/cloudposse/atmos/1.2.3/atmos", want: Native},
		{name: "native cache", executable: "/home/test/.cache/atmos/toolchain/bin/cloudposse/atmos/1.2.3/atmos", want: Native},
		{name: "native XDG override", executable: "/cache/atmos/toolchain/bin/cloudposse/atmos/1.2.3/atmos", env: map[string]string{"ATMOS_XDG_CACHE_HOME": "/cache", "XDG_CACHE_HOME": "/ignored"}, want: Native},
		{name: "native custom", executable: "/project/tools/bin/cloudposse/atmos/1.2.3/atmos", roots: []string{"/project/tools"}, want: Native},
		{name: "native relative", executable: "/work/tools/bin/cloudposse/atmos/1.2.3/atmos", roots: []string{"tools"}, want: Native},
		{name: "native fallback", executable: "/work/.tools/bin/cloudposse/atmos/1.2.3/atmos", want: Native},
		{name: "native windows cache", goos: "windows", executable: `C:\Data\cache\atmos\toolchain\bin\cloudposse\atmos\1.2.3\atmos.exe`, env: map[string]string{"LOCALAPPDATA": `C:\Data`}, want: Native},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newSystem(tt.executable)
			if tt.goos != "" {
				s.goos = tt.goos
			}
			if tt.env != nil {
				s.env = tt.env
			}
			got := Detect(t.Context(), WithSystem(s), WithNativeRoots(tt.roots...))
			assert.Equal(t, tt.want, got.Kind)
			assert.Equal(t, tt.global, got.Global)
			assert.Equal(t, tt.executable, got.Executable)
			assert.NotEmpty(t, got.Manager)
			assert.Empty(t, s.calls, "strong path evidence should not query other package managers")
		})
	}
}

func TestDetectSymlinksAndCompetingManagers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		target string
		want   Kind
	}{
		{"/opt/homebrew/Cellar/atmos/1.2.3/bin/atmos", Homebrew},
		{"/home/test/.local/share/mise/installs/atmos/1.2.3/bin/atmos", Mise},
		{"/home/test/.cache/atmos/toolchain/bin/cloudposse/atmos/1.2.3/atmos", Native},
		{"/downloads/atmos", Unknown},
	}
	for _, tt := range tests {
		t.Run(string(tt.want), func(t *testing.T) {
			t.Parallel()
			s := newSystem("/opt/homebrew/bin/atmos")
			s.goos = "darwin"
			s.links[s.executable] = tt.target
			got := Detect(t.Context(), WithSystem(s))
			assert.Equal(t, tt.want, got.Kind)
			assert.Equal(t, tt.target, got.Executable)
		})
	}
}

func TestDetectRejectsMisleadingPaths(t *testing.T) {
	t.Parallel()
	for _, executable := range []string{
		"/usr/local/bin/atmos", "/opt/homebrew/bin/atmos", "/tmp/Cellar/atmos/1.2.3/bin/atmos",
		"/home/test/.asdf-other/installs/atmos/1.2.3/bin/atmos", "/home/test/.asdf/installs/atmos-other/1.2.3/bin/atmos",
		"/home/test/.local/share/mise/installs/atmos/1.2.3/other", "/nix/store/short-atmos-1.2.3/bin/atmos",
		"/nix/store/01234567890123456789012345678901-other-1.2.3/bin/atmos", "/home/test/go/bin/atmos",
		"/home/test/.atmos/bin/other/atmos/1.2.3/atmos", "/downloads/atmos",
		"/home/test/.local/share/aquaproj-aqua/pkgs/github_release/github.com/cloudposse/atmos/v1.2.3/asset/README",
		"/home/test/.local/share/aquaproj-aqua/pkgs/github_release/github.com/cloudposse/atmos/v1.2.3/asset/atmos_9.9.9_linux_amd64",
		"/home/test/.local/share/aquaproj-aqua/pkgs/github_release/github.com/cloudposse/atmos/v1.2.3/asset/atmos_1.2.3_windows_amd64.exe",
	} {
		t.Run(executable, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, Unknown, Detect(t.Context(), WithSystem(newSystem(executable))).Kind)
		})
	}
}

func TestDetectPackageOwnership(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, probe, output string
		kind                Kind
		manager             string
	}{
		{"deb", "dpkg-query --search /usr/bin/atmos", "atmos: /usr/bin/atmos\n", DEB, "apt-get"},
		{"deb architecture", "dpkg-query --search /usr/bin/atmos", "atmos:amd64: /usr/bin/atmos\n", DEB, "apt-get"},
		{"rpm", "rpm -qf --queryformat %{NAME} /usr/bin/atmos", "atmos", RPM, "dnf"},
		{"apk", "apk info --who-owns /usr/bin/atmos", "/usr/bin/atmos is owned by atmos-1.2.3-r0\n", APK, "apk"},
		{"other deb", "dpkg-query --search /usr/bin/atmos", "other: /usr/bin/atmos", Unknown, ""},
		{"other rpm", "rpm -qf --queryformat %{NAME} /usr/bin/atmos", "atmos-helper", Unknown, ""},
		{"other apk", "apk info --who-owns /usr/bin/atmos", "/usr/bin/atmos is owned by atmos-helper-1.2.3-r0", Unknown, ""},
		{"wrong path", "dpkg-query --search /usr/bin/atmos", "atmos: /usr/bin/other", Unknown, ""},
		{"ambiguous deb", "dpkg-query --search /usr/bin/atmos", "atmos, other: /usr/bin/atmos", Unknown, ""},
		{"multiline", "dpkg-query --search /usr/bin/atmos", "atmos: /usr/bin/atmos\nother: /usr/bin/atmos", Unknown, ""},
		{"malformed", "apk info --who-owns /usr/bin/atmos", "atmos installed", Unknown, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newSystem("/usr/bin/atmos")
			s.outputs[tt.probe] = tt.output
			got := Detect(t.Context(), WithSystem(s))
			assert.Equal(t, tt.kind, got.Kind)
			assert.Equal(t, tt.manager, got.Manager)
			if tt.kind != Unknown {
				assert.Equal(t, "atmos", got.PackageName)
				hint := got.UpgradeHint("1.2.4")
				assert.Contains(t, hint.Condition, "If Atmos is available from your configured repositories")
				assert.Contains(t, hint.Message, "download and install")
				assert.Equal(t, ReleasesURL, hint.URL)
			}
		})
	}
}

func TestDetectAmbiguousOwnership(t *testing.T) {
	t.Parallel()
	s := newSystem("/usr/bin/atmos")
	s.outputs["dpkg-query --search /usr/bin/atmos"] = "atmos: /usr/bin/atmos"
	s.outputs["rpm -qf --queryformat %{NAME} /usr/bin/atmos"] = "atmos"
	assert.Equal(t, Unknown, Detect(t.Context(), WithSystem(s)).Kind)
	s = newSystem("/tools/installs/atmos/1.2.3/atmos")
	s.env = map[string]string{"MISE_DATA_DIR": "/tools", "ASDF_DATA_DIR": "/tools"}
	assert.Equal(t, Unknown, Detect(t.Context(), WithSystem(s)).Kind)
}

func TestDetectGoBuild(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, executable, module, version string
		env                               map[string]string
		want                              Kind
	}{
		{"go install", "/home/test/go/bin/atmos", "github.com/cloudposse/atmos", "v1.2.3", nil, Go},
		{"GOBIN", "/custom/atmos", "github.com/cloudposse/atmos", "v1.2.3", map[string]string{"GOBIN": "/custom"}, Go},
		{"GOPATH", "/custom/bin/atmos", "github.com/cloudposse/atmos", "v1.2.3", map[string]string{"GOPATH": "/other:/custom"}, Go},
		{"release elsewhere", "/usr/local/bin/atmos", "github.com/cloudposse/atmos", "v1.2.3", nil, Unknown},
		{"source build", "/home/test/go/bin/atmos", "github.com/cloudposse/atmos", "(devel)", nil, Unknown},
		{"different module", "/home/test/go/bin/atmos", "example.com/atmos", "v1.2.3", nil, Unknown},
		{"GOBIN override", "/home/test/go/bin/atmos", "github.com/cloudposse/atmos", "v1.2.3", map[string]string{"GOBIN": "/custom"}, Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newSystem(tt.executable)
			s.info = &debug.BuildInfo{Main: debug.Module{Path: tt.module, Version: tt.version}}
			if tt.env != nil {
				s.env = tt.env
			}
			assert.Equal(t, tt.want, Detect(t.Context(), WithSystem(s)).Kind)
		})
	}
}

func TestDetectFailuresAndDeadline(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"executable", "symlink", "missing managers", "probe failure", "deadline"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			s := newSystem("/usr/bin/atmos")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch failure {
			case "executable":
				s.executableErr = errUnavailable
			case "symlink":
				s.resolveErr = errUnavailable
			case "missing managers":
				s.managers = nil
			case "deadline":
				s.probe = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
					deadline, ok := ctx.Deadline()
					require.True(t, ok)
					assert.LessOrEqual(t, time.Until(deadline), time.Second)
					cancel()
					return nil, ctx.Err()
				}
			}
			assert.Equal(t, Unknown, Detect(ctx, WithSystem(s)).Kind)
			if failure == "deadline" {
				assert.Len(t, s.calls, 1)
			}
		})
	}
}

func TestDetectMissingManagerAndYumFallback(t *testing.T) {
	t.Parallel()
	s := newSystem("/usr/local/Cellar/atmos/1.2.3/bin/atmos")
	s.goos, s.managers = "darwin", nil
	got := Detect(t.Context(), WithSystem(s))
	assert.Equal(t, Homebrew, got.Kind)
	assert.Empty(t, got.Manager)
	assert.Empty(t, got.UpgradeHint("1.2.4").Command)
	s = newSystem("/usr/bin/atmos")
	s.managers["dnf"] = false
	s.outputs["rpm -qf --queryformat %{NAME} /usr/bin/atmos"] = "atmos"
	assert.Equal(t, "sudo yum update atmos", Detect(t.Context(), WithSystem(s)).UpgradeHint("1.2.4").Command)
}

func TestDetectTimeout(t *testing.T) {
	t.Parallel()
	s := newSystem("/usr/bin/atmos")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	s.probe = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	got := Detect(ctx, WithSystem(s))
	assert.Equal(t, Unknown, got.Kind)
	assert.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	assert.LessOrEqual(t, len(s.calls), 1, "do not start more probes after the shared deadline")
}
