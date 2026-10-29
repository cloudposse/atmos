package exec

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	data "github.com/cloudposse/atmos/pkg/template/gomplate"
)

const (
	// Denies every file-scheme source that lies outside the allowed roots.
	fileConfinementRule = "input.scheme == 'file' && !input.within_allowed_roots"
	// Never matches, so every call is admitted while function rules stay enabled.
	functionAllowAllRule = "input.name == 'no-such-function'"
)

// guardedNamespaceValue builds the policy-guarded template function map from the native gomplate map
// and returns the value produced by the namespace constructor, exactly as a template would see it.
func guardedNamespaceValue(t *testing.T, name, file string) any {
	t.Helper()

	funcs, err := policyTemplateFuncs(data.CreateFuncs(context.Background(), &data.Data{}), file, &data.Data{})
	require.NoError(t, err)
	constructor, ok := funcs[name].(func() (any, error))
	require.Truef(t, ok, "namespace %q is not a guarded constructor: %T", name, funcs[name])
	namespace, err := constructor()
	require.NoError(t, err)
	return namespace
}

func guardedFileNamespace(t *testing.T) *policyFileFuncs {
	t.Helper()

	namespace, ok := guardedNamespaceValue(t, "file", "").(*policyFileFuncs)
	require.True(t, ok, "file namespace must be the policy-guarded implementation")
	return namespace
}

func guardedNetNamespace(t *testing.T) *policyNetFuncs {
	t.Helper()

	namespace, ok := guardedNamespaceValue(t, "net", "").(*policyNetFuncs)
	require.True(t, ok, "net namespace must be the policy-guarded implementation")
	return namespace
}

func guardedEnvNamespace(t *testing.T) *policyEnvFuncs {
	t.Helper()

	namespace, ok := guardedNamespaceValue(t, "env", "").(*policyEnvFuncs)
	require.True(t, ok, "env namespace must be the policy-guarded implementation")
	return namespace
}

// fileNamespaceCall invokes one guarded file method and normalizes the result for table-driven assertions.
type fileNamespaceCall func(f *policyFileFuncs, path string) (any, error)

func fileNamespaceCalls() map[string]fileNamespaceCall {
	return map[string]fileNamespaceCall{
		"Read":    func(f *policyFileFuncs, path string) (any, error) { return f.Read(path) },
		"Stat":    func(f *policyFileFuncs, path string) (any, error) { return f.Stat(path) },
		"Exists":  func(f *policyFileFuncs, path string) (any, error) { return f.Exists(path) },
		"IsDir":   func(f *policyFileFuncs, path string) (any, error) { return f.IsDir(path) },
		"ReadDir": func(f *policyFileFuncs, path string) (any, error) { return f.ReadDir(path) },
		"Walk":    func(f *policyFileFuncs, path string) (any, error) { return f.Walk(path) },
		"Write":   func(f *policyFileFuncs, path string) (any, error) { return f.Write(path, "written") },
	}
}

// retryingTempDir returns t.TempDir plus a cleanup that empties it with retries first. On Windows,
// antivirus, indexing, and directory handles the file functions leave for the finalizer to close can
// briefly hold a path, which makes the single removal attempt in t.TempDir fail intermittently with
// "being used by another process". Collecting garbage before each attempt runs those finalizers.
// Cleanups run last-in first-out, so this one empties the directory before t.TempDir removes it.
func retryingTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			for range 20 {
				runtime.GC()
				if os.RemoveAll(path) == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	})
	return dir
}

func TestPolicyFileFuncsSourceConfinement(t *testing.T) {
	root := retryingTempDir(t)
	allowed := filepath.Join(root, "allowed")
	require.NoError(t, os.Mkdir(allowed, 0o700))
	inside := filepath.Join(allowed, "public.txt")
	writable := filepath.Join(allowed, "writable.txt")
	require.NoError(t, os.WriteFile(writable, []byte("old"), 0o600))
	outsideDir := filepath.Join(root, "outside")
	require.NoError(t, os.Mkdir(outsideDir, 0o700))
	outside := filepath.Join(outsideDir, "secret.txt")
	require.NoError(t, os.WriteFile(inside, []byte("public"), 0o600))
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
	templatePolicy(t, "source", fileConfinementRule, []string{allowed})
	namespace := guardedFileNamespace(t)
	calls := fileNamespaceCalls()

	t.Run("allowed paths reach the native implementation", func(t *testing.T) {
		got, err := calls["Read"](namespace, inside)
		require.NoError(t, err)
		assert.Equal(t, "public", got)

		info, err := namespace.Stat(inside)
		require.NoError(t, err)
		assert.Equal(t, "public.txt", info.Name())
		assert.Equal(t, int64(len("public")), info.Size())

		got, err = calls["Exists"](namespace, inside)
		require.NoError(t, err)
		assert.Equal(t, true, got)

		got, err = calls["IsDir"](namespace, allowed)
		require.NoError(t, err)
		assert.Equal(t, true, got)

		got, err = calls["IsDir"](namespace, inside)
		require.NoError(t, err)
		assert.Equal(t, false, got)

		got, err = calls["ReadDir"](namespace, allowed)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"public.txt", "writable.txt"}, got)

		walked, err := namespace.Walk(allowed)
		require.NoError(t, err)
		assert.Contains(t, walked, inside)

		// Native Write only accepts paths inside the working directory, so run from the allowed root.
		t.Chdir(allowed)
		_, err = namespace.Write("writable.txt", "written")
		require.NoError(t, err)
		content, err := os.ReadFile(writable)
		require.NoError(t, err)
		assert.Equal(t, "written", string(content))
	})

	t.Run("a path that cannot be resolved is denied because containment is unprovable", func(t *testing.T) {
		missing := filepath.Join(allowed, "missing.txt")
		exists, err := namespace.Exists(missing)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.False(t, exists)
		_, err = namespace.Write(filepath.Join(allowed, "new.txt"), "written")
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.NoFileExists(t, filepath.Join(allowed, "new.txt"))
	})

	t.Run("paths outside the allowed roots are denied before any I/O", func(t *testing.T) {
		written := filepath.Join(outsideDir, "must-not-exist.txt")
		paths := map[string]string{
			"Read": outside, "Stat": outside, "Exists": outside, "IsDir": outsideDir,
			"ReadDir": outsideDir, "Walk": outsideDir, "Write": written,
		}
		for method, path := range paths {
			got, err := calls[method](namespace, path)
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied, method)
			assert.NotEqual(t, "secret", got, method)
		}
		assert.NoFileExists(t, written, "a denied Write must not create the file")
		content, err := os.ReadFile(outside)
		require.NoError(t, err)
		assert.Equal(t, "secret", string(content), "denied calls leave the outside file untouched")
	})

	t.Run("denied Exists and IsDir return the zero value with the error", func(t *testing.T) {
		exists, err := namespace.Exists(outside)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.False(t, exists, "a denied probe must not leak that the file exists")
		isDir, err := namespace.IsDir(outsideDir)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.False(t, isDir)
	})

	t.Run("a relative path that resolves outside the roots is denied", func(t *testing.T) {
		escape := filepath.Join(allowed, "..", "outside", "secret.txt")
		got, err := namespace.Read(escape)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.Empty(t, got)
	})

	t.Run("a symlink inside the root that points outside is denied", func(t *testing.T) {
		link := filepath.Join(allowed, "link.txt")
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("symlinks are not available on this platform: %v", err)
		}
		got, err := namespace.Read(link)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.Empty(t, got)
	})
}

func TestPolicyFileFuncsFunctionAdmission(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("contents"), 0o600))
	written := filepath.Join(directory, "written.txt")
	templatePolicy(t, "function", "input.engine == 'go-template' && input.name.startsWith('file.')", nil)
	namespace := guardedFileNamespace(t)

	for method, call := range fileNamespaceCalls() {
		path := target
		if method == "Write" {
			path = written
		}
		got, err := call(namespace, path)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied, method)
		assert.NotEqual(t, "contents", got, method)
	}
	assert.NoFileExists(t, written, "a denied Write must not create the file")
}

func TestPolicyFileFuncsAdmittedWithoutSourceRules(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.txt")
	require.NoError(t, os.WriteFile(target, []byte("contents"), 0o600))
	// Only function rules are enabled, so the source gate is a no-op and the calls reach the native implementation.
	templatePolicy(t, "function", functionAllowAllRule, nil)
	namespace := guardedFileNamespace(t)

	got, err := namespace.Read(target)
	require.NoError(t, err)
	assert.Equal(t, "contents", got)
	_, err = namespace.Read(filepath.Join(directory, "missing.txt"))
	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrPolicyDenied, "a missing file is an ordinary error, not a policy outcome")
}

func TestPolicyNetFuncsFunctionAdmission(t *testing.T) {
	templatePolicy(t, "function", "input.engine == 'go-template' && input.name.startsWith('net.')", nil)
	namespace := guardedNetNamespace(t)

	denied := map[string]func() error{
		"LookupIP":        func() error { _, err := namespace.LookupIP("localhost"); return err },
		"LookupIPs":       func() error { _, err := namespace.LookupIPs("localhost"); return err },
		"LookupCNAME":     func() error { _, err := namespace.LookupCNAME("localhost"); return err },
		"LookupSRV":       func() error { _, err := namespace.LookupSRV("localhost"); return err },
		"LookupSRVs":      func() error { _, err := namespace.LookupSRVs("localhost"); return err },
		"LookupTXT":       func() error { _, err := namespace.LookupTXT("localhost"); return err },
		"ParseIP":         func() error { _, err := namespace.ParseIP("192.0.2.1"); return err },
		"ParseIPPrefix":   func() error { _, err := namespace.ParseIPPrefix("192.0.2.0/24"); return err },
		"ParseIPRange":    func() error { _, err := namespace.ParseIPRange("192.0.2.1-192.0.2.9"); return err },
		"CIDRHost":        func() error { _, err := namespace.CIDRHost(1, "192.0.2.0/24"); return err },
		"CIDRNetmask":     func() error { _, err := namespace.CIDRNetmask("192.0.2.0/24"); return err },
		"CIDRSubnets":     func() error { _, err := namespace.CIDRSubnets(2, "192.0.2.0/24"); return err },
		"CIDRSubnetSizes": func() error { _, err := namespace.CIDRSubnetSizes(2, 2, "192.0.2.0/24"); return err },
	}
	for method, call := range denied {
		require.ErrorIs(t, call(), errUtils.ErrPolicyDenied, method)
	}
}

func TestPolicyNetFuncsDNSSourceConfinement(t *testing.T) {
	templatePolicy(t, "source", "input.scheme == 'dns' && input.host == 'blocked.example.invalid'", nil)
	namespace := guardedNetNamespace(t)

	lookups := map[string]func(name string) error{
		"LookupIP":    func(name string) error { _, err := namespace.LookupIP(name); return err },
		"LookupIPs":   func(name string) error { _, err := namespace.LookupIPs(name); return err },
		"LookupCNAME": func(name string) error { _, err := namespace.LookupCNAME(name); return err },
		"LookupSRV":   func(name string) error { _, err := namespace.LookupSRV(name); return err },
		"LookupSRVs":  func(name string) error { _, err := namespace.LookupSRVs(name); return err },
		"LookupTXT":   func(name string) error { _, err := namespace.LookupTXT(name); return err },
	}
	for method, lookup := range lookups {
		require.ErrorIs(t, lookup("blocked.example.invalid"), errUtils.ErrPolicyDenied, method)
		// A host the rule does not match is admitted, so the native resolver runs and fails as an ordinary
		// lookup error: an empty name is rejected locally without any network traffic.
		err := lookup("")
		require.Error(t, err, method)
		assert.NotErrorIs(t, err, errUtils.ErrPolicyDenied, method)
	}
}

func TestPolicyNetFuncsAdmittedCallsReachNative(t *testing.T) {
	templatePolicy(t, "function", functionAllowAllRule, nil)
	namespace := guardedNetNamespace(t)

	ip, err := namespace.ParseIP("192.0.2.1")
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.1", ip.String())

	_, err = namespace.ParseIP("not-an-ip")
	require.Error(t, err, "native parse errors are returned unchanged")
	assert.NotErrorIs(t, err, errUtils.ErrPolicyDenied)

	prefix, err := namespace.ParseIPPrefix("192.0.2.0/24")
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.0/24", prefix.String())

	ipRange, err := namespace.ParseIPRange("192.0.2.1-192.0.2.9")
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.1-192.0.2.9", ipRange.String())

	// The CIDR helpers are experimental in gomplate and Atmos never enables experimental mode, so an admitted
	// call surfaces the native experimental error rather than a policy denial.
	for method, call := range map[string]func() error{
		"CIDRHost":        func() error { _, err := namespace.CIDRHost(5, "192.0.2.0/24"); return err },
		"CIDRNetmask":     func() error { _, err := namespace.CIDRNetmask("192.0.2.0/24"); return err },
		"CIDRSubnets":     func() error { _, err := namespace.CIDRSubnets(2, "192.0.2.0/24"); return err },
		"CIDRSubnetSizes": func() error { _, err := namespace.CIDRSubnetSizes(2, 2, "192.0.2.0/24"); return err },
	} {
		err := call()
		require.ErrorContains(t, err, "experimental", method)
		assert.NotErrorIs(t, err, errUtils.ErrPolicyDenied, method)
	}

	// Lookups use the admitted path too; an empty name fails inside the native resolver, not in the policy gate.
	for method, lookup := range map[string]func() error{
		"LookupIP":    func() error { _, err := namespace.LookupIP(""); return err },
		"LookupIPs":   func() error { _, err := namespace.LookupIPs(""); return err },
		"LookupCNAME": func() error { _, err := namespace.LookupCNAME(""); return err },
		"LookupSRV":   func() error { _, err := namespace.LookupSRV(""); return err },
		"LookupSRVs":  func() error { _, err := namespace.LookupSRVs(""); return err },
		"LookupTXT":   func() error { _, err := namespace.LookupTXT(""); return err },
	} {
		err := lookup()
		require.Error(t, err, method)
		assert.NotErrorIs(t, err, errUtils.ErrPolicyDenied, method)
	}
}

func TestPolicyEnvFuncs(t *testing.T) {
	const variable = "POLICY_ENV_NAMESPACE_TEST"
	t.Setenv(variable, "from-env")

	t.Run("admitted calls reach the native implementation", func(t *testing.T) {
		templatePolicy(t, "function", functionAllowAllRule, nil)
		namespace := guardedEnvNamespace(t)

		value, err := namespace.Getenv(variable)
		require.NoError(t, err)
		assert.Equal(t, "from-env", value)

		value, err = namespace.Getenv("POLICY_ENV_NAMESPACE_UNSET", "fallback")
		require.NoError(t, err)
		assert.Equal(t, "fallback", value, "defaults apply when the variable is unset")

		value, err = namespace.ExpandEnv("prefix-${" + variable + "}")
		require.NoError(t, err)
		assert.Equal(t, "prefix-from-env", value)
	})

	t.Run("denied calls never read the environment", func(t *testing.T) {
		templatePolicy(t, "function", "input.engine == 'go-template' && input.name.startsWith('env.')", nil)
		namespace := guardedEnvNamespace(t)

		value, err := namespace.Getenv(variable)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.Empty(t, value)

		value, err = namespace.ExpandEnv("${" + variable + "}")
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		assert.Empty(t, value)
	})
}

// Gomplate converts the path argument with conv.ToString, so a []byte names the file its bytes spell.
// Admission checks that same file, not the printed form of the byte slice.
func TestPolicyFileFuncsCheckTheConvertedPath(t *testing.T) {
	root := retryingTempDir(t)
	allowed := filepath.Join(root, "allowed")
	require.NoError(t, os.Mkdir(allowed, 0o700))
	inside := filepath.Join(allowed, "public.txt")
	outside := filepath.Join(root, "secret.txt")
	require.NoError(t, os.WriteFile(inside, []byte("public"), 0o600))
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
	templatePolicy(t, "source", fileConfinementRule, []string{allowed})
	namespace := guardedFileNamespace(t)

	got, err := namespace.Read([]byte(inside))
	require.NoError(t, err)
	assert.Equal(t, "public", got)

	denied, err := namespace.Read([]byte(outside))
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied, "a byte path outside the roots is denied")
	assert.Empty(t, denied)

	_, err = namespace.Exists([]byte(outside))
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
}
