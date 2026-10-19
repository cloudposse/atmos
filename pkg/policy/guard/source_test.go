package guard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestSourceNormalization(t *testing.T) {
	for _, tc := range []struct{ raw, scheme, host string }{
		{"git::https://user:secret@EXAMPLE.com./org/repo?token=secret", "https", "example.com"},
		{"git@example.com:org/repo", "ssh", "example.com"},
		{"github.com/org/repo", "https", "github.com"},
		{"s3::https://bucket.example.com/object", "https", "bucket.example.com"},
	} {
		input, err := DescribeSource("download", tc.raw, "")
		require.NoError(t, err)
		require.Equal(t, tc.scheme, input.Scheme)
		require.Equal(t, tc.host, input.Host)
		require.True(t, input.Remote)
		require.NotContains(t, input.Path, "secret")
	}
}

func TestRemoteProvenanceSurvivesCacheAndMerge(t *testing.T) {
	ResetOrigins()
	t.Cleanup(ResetOrigins)
	root := t.TempDir()
	cached := filepath.Join(root, "remote")
	require.NoError(t, os.Mkdir(cached, 0o700))
	RegisterRemote(cached, "https://user:secret@example.com/repo?token=secret")
	remote := FileOrigin(filepath.Join(cached, "nested", "stack.yaml"))
	require.Equal(t, "remote", remote.Provenance)
	require.NotContains(t, remote.SourceFile, "secret")
	alias := filepath.Join(root, "alias")
	require.NoError(t, os.Symlink(cached, alias))
	require.Equal(t, "remote", FileOrigin(filepath.Join(alias, "nested", "stack.yaml")).Provenance)
	local := FileOrigin(root)
	require.Equal(t, "local", local.Provenance)
	RememberValue("!exec identical", remote)
	RememberValue("!exec identical", local)
	require.Equal(t, "remote", ValueOrigin("!exec identical").Provenance)
	require.Equal(t, "unknown", ValueOrigin("!exec missing").Provenance)
	require.Equal(t, "unknown", FileOrigin("").Provenance)
	ResetOrigins()
	require.Equal(t, "unknown", ValueOrigin("!exec identical").Provenance)
}

func TestFileOriginRequiresExistingLocalPath(t *testing.T) {
	ResetOrigins()
	t.Cleanup(ResetOrigins)
	root := t.TempDir()
	file := filepath.Join(root, "stack.yaml")
	require.NoError(t, os.WriteFile(file, []byte("vars: {}"), 0o600))
	origin := FileOrigin(file)
	require.Equal(t, "local", origin.Provenance)
	alias := filepath.Join(root, "alias.yaml")
	require.NoError(t, os.Symlink(file, alias))
	require.Equal(t, origin, FileOrigin(alias), "symlinks preserve the target's attribution")
	require.Equal(t, "unknown", FileOrigin(filepath.Join(root, "missing", "stack.yaml")).Provenance)
	require.Equal(t, "unknown", FileOrigin(file+"/child").Provenance)
	require.NoError(t, os.Remove(file))
	require.Equal(t, "unknown", FileOrigin(alias).Provenance, "dangling symlinks are not local evidence")
}

func TestValueFingerprintsPreserveConservativeAttribution(t *testing.T) {
	ResetOrigins()
	t.Cleanup(ResetOrigins)
	local := Origin{Provenance: "local", SourceFile: "stack.yaml"}
	unknown := Origin{Provenance: "unknown"}
	remote := Origin{Provenance: "remote", SourceFile: "https://example.com/stack.yaml"}
	const value = "password: private-configuration-value"
	RememberValue(value, local)
	require.Equal(t, local, ValueOrigin(value))
	RememberValue(value, unknown)
	RememberValue(value, local)
	require.Equal(t, unknown, ValueOrigin(value), "local attribution cannot override unknown provenance")
	RememberValue(value, remote)
	RememberValue(value, unknown)
	require.Equal(t, remote, ValueOrigin(value), "remote attribution remains the most restrictive")
	require.Equal(t, unknown, ValueOrigin(value+"-different"), "different values cannot share attribution")
	ResetOrigins()
	require.Equal(t, unknown, ValueOrigin(value), "invocations cannot inherit value attribution")
}

func TestRememberNodeRecordsEveryScalar(t *testing.T) {
	ResetOrigins()
	t.Cleanup(ResetOrigins)
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("name: top-level\nitems:\n  - first\n  - nested: !exec echo hi\n"), &doc))

	RememberNode(&doc, Origin{Provenance: "remote", SourceFile: "https://example.com/stack.yaml"})

	for _, value := range []string{"top-level", "first", "echo hi"} {
		require.Equal(t, "remote", ValueOrigin(value).Provenance, value)
	}
	require.Equal(t, "unknown", ValueOrigin("never included").Provenance)
	RememberNode(nil, Origin{Provenance: "local"}) // A nil node is ignored.
}

func TestDescribeSourceRejectsMalformedDestinations(t *testing.T) {
	for name, raw := range map[string]string{
		"invalid escape in path":  "https://example.com/%zz",
		"nonlocal file authority": "file://example.com/etc/passwd",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DescribeSource("include", raw, "")
			require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
			assert.NotContains(t, err.Error(), raw, "details of the destination are withheld")
		})
	}
}

func TestDescribeSourceLocalDestinations(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "stack.yaml")
	require.NoError(t, os.WriteFile(existing, []byte("vars: {}"), 0o600))
	canonical, err := filepath.EvalSymlinks(existing)
	require.NoError(t, err)

	input, err := DescribeSource("include", existing, "")
	require.NoError(t, err)
	assert.Equal(t, SourceInput{Operation: "include", Scheme: "file", Path: canonical, PathResolved: true, Origin: input.Origin}, input)

	missing := filepath.Join(root, "missing.yaml")
	input, err = DescribeSource("include", missing, "")
	require.NoError(t, err)
	assert.Equal(t, "file", input.Scheme)
	assert.False(t, input.Remote)
	assert.False(t, input.PathResolved, "nonexistent paths are reported unresolved")
	assert.Equal(t, filepath.Base(missing), filepath.Base(input.Path))

	for _, raw := range []string{"file://localhost/definitely-missing-root/x.yaml", "file:///definitely-missing-root/x.yaml"} {
		input, err = DescribeSource("include", raw, "")
		require.NoError(t, err, raw)
		assert.Equal(t, "file", input.Scheme, raw)
		assert.False(t, input.Remote, raw)
		assert.False(t, input.PathResolved, raw)
	}
}

func TestIsRemote(t *testing.T) {
	for raw, want := range map[string]bool{
		"git@example.com:org/repo":            true,
		"ssh://example.com/org/repo":          true,
		"git::https://example.com/org/repo":   true,
		"s3::https://bucket.example.com/key":  true,
		"git::file:///tmp/repo":               false,
		"file:///tmp/repo":                    false,
		"FILE:///tmp/repo":                    false,
		"github.com/org/repo":                 true,
		"gitlab.com/org/repo":                 true,
		"bitbucket.org/org/repo":              true,
		"./relative/stack.yaml":               false,
		"stack.yaml":                          false,
		"example.com/org/repo":                false,
		"user@host-without-colon":             false,
		"@example.com:org/leading-at-ignored": false,
	} {
		assert.Equal(t, want, isRemote(raw), raw)
	}
}

func TestSafeSource(t *testing.T) {
	root := t.TempDir()
	canonical, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)

	assert.Equal(t, "https://example.com/org/repo", safeSource("git::https://user:secret@example.com/org/repo?token=secret"))
	assert.Equal(t, "ssh://example.com/org/repo", safeSource("git@example.com:org/repo"))
	assert.Equal(t, canonical, safeSource(root), "local sources report their canonical path")
	assert.Equal(t, "[unavailable]", safeSource("https://example.com/%zz"))
}

func TestIsPolicyError(t *testing.T) {
	for _, sentinel := range []error{
		errUtils.ErrPolicyDenied, errUtils.ErrPolicyInvalid, errUtils.ErrPolicyEvaluation, errUtils.ErrPolicyInputUnavailable,
		errUtils.ErrPolicyInputInvalid, errUtils.ErrPolicyRequirements, errUtils.ErrPolicyEngineUnknown,
	} {
		assert.True(t, IsPolicyError(fmt.Errorf("optional import: %w", sentinel)), sentinel.Error())
	}
	assert.False(t, IsPolicyError(nil))
	assert.False(t, IsPolicyError(os.ErrNotExist))
	assert.False(t, IsPolicyError(errors.New("unrelated")))
}

// installGate wires a recording evaluator and capability lookup, restoring the unconfigured state afterwards.
func installGate(t *testing.T, enabled func(string) (bool, error), evaluate func(context.Context, string, json.RawMessage) error) {
	t.Helper()
	SetEnabled(enabled)
	SetEvaluator(evaluate)
	t.Cleanup(func() {
		SetEnabled(nil)
		SetEvaluator(nil)
	})
}

func TestSourceGate(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "stack.yaml")
	require.NoError(t, os.WriteFile(target, []byte("vars: {}"), 0o600))
	always := func(string) (bool, error) { return true, nil }

	t.Run("evaluates the described destination", func(t *testing.T) {
		var kinds []string
		var inputs []SourceInput
		installGate(t, always, func(_ context.Context, kind string, raw json.RawMessage) error {
			var input SourceInput
			require.NoError(t, json.Unmarshal(raw, &input))
			kinds = append(kinds, kind)
			inputs = append(inputs, input)
			return nil
		})
		require.NoError(t, Source(t.Context(), "include", target, ""))
		require.Equal(t, []string{"source"}, kinds)
		require.Len(t, inputs, 1)
		assert.Equal(t, "include", inputs[0].Operation)
		assert.Equal(t, "file", inputs[0].Scheme)
		assert.True(t, inputs[0].PathResolved)
	})

	t.Run("evaluator denial is returned", func(t *testing.T) {
		installGate(t, always, func(context.Context, string, json.RawMessage) error { return errUtils.ErrPolicyDenied })
		require.ErrorIs(t, Source(t.Context(), "include", target, ""), errUtils.ErrPolicyDenied)
	})

	t.Run("malformed destination never reaches the evaluator", func(t *testing.T) {
		called := false
		installGate(t, always, func(context.Context, string, json.RawMessage) error {
			called = true
			return nil
		})
		require.ErrorIs(t, Source(t.Context(), "include", "file://example.com/x", ""), errUtils.ErrPolicyInputInvalid)
		assert.False(t, called)
	})

	t.Run("capability disabled skips evaluation", func(t *testing.T) {
		called := false
		installGate(t, func(string) (bool, error) { return false, nil }, func(context.Context, string, json.RawMessage) error {
			called = true
			return errUtils.ErrPolicyDenied
		})
		require.NoError(t, Source(t.Context(), "include", target, ""))
		assert.False(t, called)
	})

	t.Run("capability lookup failure is returned", func(t *testing.T) {
		called := false
		installGate(t, func(string) (bool, error) { return false, errUtils.ErrPolicyInvalid }, func(context.Context, string, json.RawMessage) error {
			called = true
			return nil
		})
		require.ErrorIs(t, Source(t.Context(), "include", target, ""), errUtils.ErrPolicyInvalid)
		assert.False(t, called)
	})

	t.Run("no installed lookup means unconfigured", func(t *testing.T) {
		installGate(t, nil, nil)
		enabled, err := Enabled("source")
		require.NoError(t, err)
		assert.False(t, enabled)
		require.NoError(t, Source(t.Context(), "include", target, ""))
	})
}

func TestEvaluateEdgeCases(t *testing.T) {
	t.Run("nil context is replaced with a background context", func(t *testing.T) {
		var got context.Context
		installGate(t, nil, func(ctx context.Context, _ string, _ json.RawMessage) error {
			got = ctx
			return nil
		})
		var nilContext context.Context
		require.NoError(t, Function(nilContext, "exec", "yaml", Origin{Provenance: "local"}))
		require.NotNil(t, got)
	})

	t.Run("function input excludes arguments and carries origin", func(t *testing.T) {
		var raw json.RawMessage
		installGate(t, nil, func(_ context.Context, kind string, payload json.RawMessage) error {
			assert.Equal(t, "function", kind)
			raw = payload
			return nil
		})
		require.NoError(t, Function(t.Context(), "exec", "yaml", Origin{Provenance: "remote", SourceFile: "https://example.com/s.yaml"}))
		assert.JSONEq(t, `{"name":"exec","engine":"yaml","provenance":"remote","source_file":"https://example.com/s.yaml"}`, string(raw))
	})

	t.Run("no evaluator allows the call", func(t *testing.T) {
		installGate(t, nil, nil)
		require.NoError(t, evaluate(t.Context(), "function", FunctionInput{}))
	})

	t.Run("unserializable input is an error before evaluation", func(t *testing.T) {
		called := false
		installGate(t, nil, func(context.Context, string, json.RawMessage) error {
			called = true
			return nil
		})
		require.Error(t, evaluate(t.Context(), "function", make(chan int)))
		assert.False(t, called)
	})
}

func TestRegisterRemoteInitializesOriginTable(t *testing.T) {
	ResetOrigins()
	t.Cleanup(ResetOrigins)
	root := t.TempDir()
	state.Lock()
	state.origins = nil
	state.Unlock()

	RegisterRemote(root, "https://example.com/repo")

	origin := FileOrigin(filepath.Join(root, "child.yaml"))
	assert.Equal(t, "remote", origin.Provenance)
	assert.Equal(t, "https://example.com/repo", origin.SourceFile)
}

func TestRememberTreeWalksEveryContainer(t *testing.T) {
	ResetOrigins()
	t.Cleanup(ResetOrigins)
	remote := Origin{Provenance: "remote", SourceFile: "https://example.com/stack.yaml"}

	RememberTree(map[string]any{
		"string-map": "from-string-map",
		"any-map":    map[any]any{"key": "from-any-map"},
		"list":       []any{"first-in-list", map[string]any{"nested": "last-in-list"}},
		"number":     42,
	}, remote)

	for _, value := range []string{"from-string-map", "from-any-map", "first-in-list", "last-in-list"} {
		assert.Equal(t, remote, ValueOrigin(value), value)
	}
	assert.Equal(t, "unknown", ValueOrigin("42").Provenance, "non-string scalars are not retained")
}

func TestTrimWindowsDriveSlash(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/C:/Users/me/data.json", "C:/Users/me/data.json"},
		{"/d:/", "d:/"},
		{"/D:", "D:"},
		{"/tmp/data.json", "/tmp/data.json"},
		{"C:/already/clean", "C:/already/clean"},
		{"/1:/not-a-drive", "/1:/not-a-drive"},
		{"/CD:/not-a-drive", "/CD:/not-a-drive"},
		{"/C:file", "/C:file"},
		{"", ""},
	} {
		require.Equal(t, tc.want, trimWindowsDriveSlash(tc.in), tc.in)
	}
}

func TestDescribeSourceWindowsFileURLResolvesDrivePath(t *testing.T) {
	if runtime.GOOS != windowsOS {
		t.Skip("file URL drive paths only exist on Windows")
	}
	file := filepath.Join(t.TempDir(), "data.json")
	require.NoError(t, os.WriteFile(file, []byte("{}"), 0o600))

	input, err := DescribeSource("download", "file:///"+filepath.ToSlash(file), "")

	require.NoError(t, err)
	require.True(t, input.PathResolved)
	resolved, err := filepath.EvalSymlinks(file)
	require.NoError(t, err)
	require.Equal(t, resolved, input.Path)
}

// A file URL without an absolute path must be rejected: resolving it would describe the working
// directory, which is often inside an allowed root, instead of the destination a reader would use.
func TestDescribeSourceRejectsFileURLsWithoutAbsolutePath(t *testing.T) {
	for _, raw := range []string{"file:../../etc/passwd", "file:relative/path", "file://localhost", "file://"} {
		t.Run(raw, func(t *testing.T) {
			_, err := DescribeSource("download", raw, "")
			require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
		})
	}

	root := t.TempDir()
	for _, raw := range []string{"file://" + filepath.ToSlash(root), "file://localhost" + filepath.ToSlash(root)} {
		t.Run("accepts "+raw, func(t *testing.T) {
			if runtime.GOOS == windowsOS {
				t.Skip("Windows file URLs need a drive path, covered separately")
			}
			input, err := DescribeSource("download", raw, "")
			require.NoError(t, err)
			require.Equal(t, "file", input.Scheme)
			resolved, err := filepath.EvalSymlinks(root)
			require.NoError(t, err)
			require.Equal(t, resolved, input.Path)
		})
	}
}

// A scheme-like prefix that no reader treats as a remote is a local path. Describing it as a remote
// scheme would hide the real destination from rules that confine file reads.
func TestDescribeSourceSchemeLikeLocalPathsAreFiles(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, raw := range []string{"evil:/passwd", "x:/etc/passwd", "foo:bar"} {
		t.Run(raw, func(t *testing.T) {
			input, err := DescribeSource("download", raw, "")
			require.NoError(t, err)
			require.Equal(t, "file", input.Scheme)
			require.False(t, input.Remote)
			require.NotEmpty(t, input.Path)
		})
	}
}

func TestDescribeSourceKeepsRemoteClassification(t *testing.T) {
	for raw, scheme := range map[string]string{
		"https://example.com/a.yaml":         "https",
		"s3::https://example.com/bucket/key": "https",
		"git::https://example.com/org/repo":  "https",
		"git@example.com:org/repo.git":       "ssh",
		"github.com/org/repo":                "https",
	} {
		t.Run(raw, func(t *testing.T) {
			input, err := DescribeSource("download", raw, "")
			require.NoError(t, err)
			require.True(t, input.Remote)
			require.Equal(t, scheme, input.Scheme)
		})
	}
}

func TestSaveHooksRestoresTheWiringItFound(t *testing.T) {
	originalErr := errors.New("original evaluator")
	SetEnabled(func(string) (bool, error) { return true, nil })
	SetEvaluator(func(context.Context, string, json.RawMessage) error { return originalErr })
	restore := SaveHooks()

	SetEnabled(func(string) (bool, error) { return false, nil })
	SetEvaluator(func(context.Context, string, json.RawMessage) error { return nil })
	enabled, err := Enabled("source")
	require.NoError(t, err)
	require.False(t, enabled, "the fake is installed")

	restore()
	enabled, err = Enabled("source")
	require.NoError(t, err)
	require.True(t, enabled, "the saved lookup is back")
	require.ErrorIs(t, evaluate(context.Background(), "source", SourceInput{}), originalErr, "the saved evaluator is back")

	// Leave no hooks behind for other tests in the package.
	SetEnabled(nil)
	SetEvaluator(nil)
}

func TestDescribeSourceEnvDatasources(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, raw := range []string{"env:POLICY_VAR", "env:///POLICY_VAR", "ENV:POLICY_VAR"} {
		t.Run(raw, func(t *testing.T) {
			input, err := DescribeSource(OperationTemplateDatasource, raw, "")
			require.NoError(t, err)
			require.Equal(t, "env", input.Scheme)
			require.False(t, input.Remote)
			require.Equal(t, "POLICY_VAR", input.Path)
		})
	}

	// Outside a datasource the same text is an ordinary relative path, so a file include cannot hide behind it.
	input, err := DescribeSource("include", "env:POLICY_VAR", "")
	require.NoError(t, err)
	require.Equal(t, "file", input.Scheme)
	require.Contains(t, filepath.ToSlash(input.Path), "env:POLICY_VAR")
}

// S3 and GCS shorthand sources are remote, as the include reader's go-getter detection decides, so
// admission describes the https endpoint and provenance records them as remote.
func TestDescribeSourceS3AndGCSShorthandAreRemote(t *testing.T) {
	t.Chdir(t.TempDir())
	for raw, host := range map[string]string{
		"bucket.s3.amazonaws.com/key.yaml":            "s3.amazonaws.com",
		"www.googleapis.com/storage/v1/bucket/x.yaml": "www.googleapis.com",
	} {
		t.Run(raw, func(t *testing.T) {
			input, err := DescribeSource("include", raw, "")
			require.NoError(t, err)
			require.True(t, input.Remote)
			require.Equal(t, "https", input.Scheme)
			require.Equal(t, host, input.Host)
			require.Equal(t, "remote", FileOrigin(raw).Provenance)
		})
	}

	// Ordinary relative paths stay local.
	input, err := DescribeSource("include", "stacks/a.yaml", "")
	require.NoError(t, err)
	require.False(t, input.Remote)
	require.Equal(t, "file", input.Scheme)
}
