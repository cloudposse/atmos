package gomplate

import (
	"bytes"
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"text/template"

	"github.com/Shopify/ejson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatasourceHTTPAndCaching(t *testing.T) {
	var gets, heads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		assert.Equal(t, "/data.json", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodHead {
			heads.Add(1)
			return
		}
		gets.Add(1)
		_, _ = w.Write([]byte(`{"n":1000000}`))
	}))
	t.Cleanup(server.Close)
	d, err := NewData([]string{"api=" + server.URL + "/"}, []string{"api=Authorization: Bearer token"})
	require.NoError(t, err)
	first, err := d.Datasource("api", "data.json")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"n": 1000000}, first)
	second, err := d.Datasource("api", "data.json")
	require.NoError(t, err)
	require.Equal(t, first, second)
	raw, err := d.Include("api", "data.json")
	require.NoError(t, err)
	require.Equal(t, `{"n":1000000}`, raw)
	require.True(t, d.DatasourceReachable("api", "data.json"))
	_, err = d.DefineDatasource("later", server.URL+"/unused.json")
	require.NoError(t, err)
	third, err := d.Datasource("api", "data.json")
	require.NoError(t, err)
	require.Equal(t, first, third)
	require.EqualValues(t, 1, gets.Load())
	require.EqualValues(t, 1, heads.Load())
}

func TestDatasourceGETOnlyEndpoint(t *testing.T) {
	var gets, heads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		gets.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"works":true}`))
	}))
	t.Cleanup(server.Close)
	d := &Data{}
	result, err := d.Datasource(server.URL + "/data.json")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"works": true}, result)
	require.EqualValues(t, 1, heads.Load())
	// v4 falls back to GET for metadata and then reads the content with GET.
	require.EqualValues(t, 2, gets.Load())
	require.True(t, d.DatasourceExists(server.URL+"/data.json"))
	require.Equal(t, []string{server.URL + "/data.json"}, d.ListDatasources())
}

func TestDatasourceDefinitionsAndTemplateFunctions(t *testing.T) {
	t.Setenv("ATMOS_GOMPLATE_ADAPTER_DATA", `{"value":"native"}`)
	d := &Data{}
	funcs := CreateFuncs(context.Background(), d)
	tmpl, err := template.New("test").Funcs(funcs).Parse(`{{ defineDatasource "env" "env:///ATMOS_GOMPLATE_ADAPTER_DATA?type=application/json" }}{{ (ds "env").value }}|{{ datasourceExists "env" }}|{{ listDatasources }}|{{ include "env" }}|{{ datasourceReachable "env" }}`)
	require.NoError(t, err)
	var result bytes.Buffer
	require.NoError(t, tmpl.Execute(&result, nil))
	require.Equal(t, `native|true|[env]|{"value":"native"}|true`, result.String())
	_, err = d.DefineDatasource("env", "env:///DOES_NOT_EXIST")
	require.NoError(t, err)
	value, err := d.Datasource("env")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"value": "native"}, value)
	_, err = d.DefineDatasource("other", "env:///ATMOS_GOMPLATE_ADAPTER_DATA?type=application/json")
	require.NoError(t, err)
	require.Equal(t, []string{"env", "other"}, d.ListDatasources())
	value, err = d.Datasource("other")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"value": "native"}, value)
}

func TestDatasourceCanBeDefinedAfterFailedRead(t *testing.T) {
	t.Setenv("ATMOS_GOMPLATE_LATE_DATASOURCE", `{"value":"recovered"}`)
	for _, function := range []string{"include", "datasource"} {
		t.Run(function, func(t *testing.T) {
			d := &Data{}
			var err error
			if function == "include" {
				_, err = d.Include("later")
			} else {
				_, err = d.Datasource("later")
			}
			require.ErrorIs(t, err, errInvalidDatasource)
			_, err = d.DefineDatasource("later", "env:///ATMOS_GOMPLATE_LATE_DATASOURCE?type=application/json")
			require.NoError(t, err)
			value, err := d.Datasource("later")
			require.NoError(t, err)
			require.Equal(t, map[string]any{"value": "recovered"}, value)
			raw, err := d.Include("later")
			require.NoError(t, err)
			require.Equal(t, `{"value":"recovered"}`, raw)
		})
	}
}

func TestDatasourceFailures(t *testing.T) {
	for _, definitions := range [][]string{{"missing-separator"}, {"=env:///X"}, {"bad=http://%"}} {
		_, err := NewData(definitions, nil)
		require.Error(t, err)
	}
	for _, headers := range [][]string{{"invalid"}, {"a=invalid"}, {"a=: value"}} {
		_, err := NewData(nil, headers)
		require.Error(t, err)
	}
	d := &Data{}
	_, err := d.Include("unregistered")
	require.Error(t, err)
	require.False(t, d.DatasourceReachable("unregistered"))
	_, err = d.DefineDatasource("absent", filepath.Join(t.TempDir(), "absent.json"))
	require.NoError(t, err)
	require.False(t, d.DatasourceReachable("absent"))
	d = &Data{Sources: map[string]*Source{"broken": {}}}
	_, err = d.Datasource("broken")
	require.ErrorContains(t, err, "has no URL")
}

func TestDatasourceCancellationAndExtraHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "secret", r.Header.Get("X-Token"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	alias := server.URL + "/data.json"
	d, err := NewData(nil, []string{alias + "=X-Token: secret"})
	require.NoError(t, err)
	result, err := d.Datasource(alias)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ok": true}, result)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d = &Data{Ctx: ctx}
	_, err = d.Datasource(alias)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDatasourceNativeParsersAndEJSON(t *testing.T) {
	jsonValue, err := JSON(`{"n":1000000}`)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"n": 1000000}, jsonValue)
	csv, err := CSV("name,value\nitem,42")
	require.NoError(t, err)
	require.Equal(t, [][]string{{"name", "value"}, {"item", "42"}}, csv)
	toml, err := TOML("value = 'native'")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"value": "native"}, toml)
	public, private, err := ejson.GenerateKeypair()
	require.NoError(t, err)
	t.Setenv("EJSON_KEY", private)
	plain := `{"_public_key":"` + public + `","secret":"preserved"}`
	var encrypted bytes.Buffer
	_, err = ejson.Encrypt(bytes.NewBufferString(plain), &encrypted)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "data.json")
	require.NoError(t, os.WriteFile(path, encrypted.Bytes(), 0o600))
	d := &Data{}
	_, err = d.DefineDatasource("encrypted", path)
	require.NoError(t, err)
	result, err := d.Datasource("encrypted")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"secret": "preserved"}, result)
	decoded, err := JSON(encrypted.String())
	require.NoError(t, err)
	require.Equal(t, map[string]any{"secret": "preserved"}, decoded)
}

func TestNativeFileNamespaceReadsAndWalks(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	require.NoError(t, os.Mkdir("nested", 0o755))
	require.NoError(t, os.WriteFile("a.txt", []byte("root"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join("nested", "b.txt"), []byte("nested"), 0o600))
	native := nativeFileFuncs{}
	for _, path := range []any{"a.txt", []byte(filepath.Join(directory, "a.txt"))} {
		value, err := native.Read(path)
		require.NoError(t, err)
		require.Equal(t, "root", value)
		info, err := native.Stat(path)
		require.NoError(t, err)
		require.Equal(t, "a.txt", info.Name())
		require.False(t, info.IsDir())
		require.True(t, native.Exists(path))
		require.False(t, native.IsDir(path))
	}
	require.True(t, native.IsDir([]byte("nested")))
	names, err := native.ReadDir(directory)
	require.NoError(t, err)
	require.Equal(t, []string{"a.txt", "nested"}, names)
	for _, root := range []string{".", directory} {
		paths, err := native.Walk([]byte(root))
		require.NoError(t, err)
		require.Equal(t, []string{root, filepath.Join(root, "a.txt"), filepath.Join(root, "nested"), filepath.Join(root, "nested", "b.txt")}, paths)
	}
	for _, root := range []string{"a.txt", filepath.Join(directory, "a.txt")} {
		paths, err := native.Walk(root)
		require.NoError(t, err)
		require.Equal(t, []string{root}, paths)
	}
	for _, path := range []string{"absent", filepath.Join(directory, "absent")} {
		require.False(t, native.Exists(path))
		require.False(t, native.IsDir(path))
		_, err := native.Read(path)
		require.ErrorIs(t, err, fs.ErrNotExist)
		_, err = native.Stat(path)
		require.ErrorIs(t, err, fs.ErrNotExist)
		_, err = native.ReadDir(path)
		require.ErrorIs(t, err, fs.ErrNotExist)
		paths, err := native.Walk(path)
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Empty(t, paths)
	}
}

func TestNativeFileNamespaceWritesAndPreservesModes(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	native := nativeFileFuncs{}
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"bytes", []byte{0, 'a', 0xff}, string([]byte{0, 'a', 0xff})},
		{"buffer", bytes.NewBufferString("buffer content"), "buffer content"},
		{"converted", 42, "42"},
	} {
		path := filepath.Join("nested", tc.name)
		output, err := native.Write([]byte(path), tc.value)
		require.NoError(t, err)
		require.Empty(t, output)
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, tc.want, string(content))
	}
	// Compare with OS-created references so the process umask and Windows modes are respected.
	require.NoError(t, os.WriteFile("reference", nil, 0o644))
	reference, err := os.Stat("reference")
	require.NoError(t, err)
	created, err := os.Stat(filepath.Join("nested", "bytes"))
	require.NoError(t, err)
	require.Equal(t, reference.Mode().Perm(), created.Mode().Perm())
	require.NoError(t, os.Mkdir("reference-dir", 0o755))
	referenceDir, err := os.Stat("reference-dir")
	require.NoError(t, err)
	createdDir, err := os.Stat("nested")
	require.NoError(t, err)
	require.Equal(t, referenceDir.Mode().Perm(), createdDir.Mode().Perm())
	require.NoError(t, os.WriteFile("existing", []byte("long existing content"), 0o600))
	before, err := os.Stat("existing")
	require.NoError(t, err)
	_, err = native.Write(filepath.Join(directory, "existing"), "short")
	require.NoError(t, err)
	after, err := os.Stat("existing")
	require.NoError(t, err)
	require.Equal(t, before.Mode(), after.Mode())
	content, err := os.ReadFile("existing")
	require.NoError(t, err)
	require.Equal(t, "short", string(content))
}

func TestNativeFileNamespaceWriteConfinement(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "work")
	require.NoError(t, os.Mkdir(directory, 0o755))
	t.Chdir(directory)
	outside := filepath.Join(parent, "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("untouched"), 0o600))
	native := nativeFileFuncs{}
	for _, path := range []any{outside, []byte(filepath.Join("..", "outside.txt")), "..prefixed"} {
		output, err := native.Write(path, "overwrite")
		require.ErrorContains(t, err, "not contained by working directory")
		require.Empty(t, output)
	}
	content, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "untouched", string(content))
	require.NoFileExists(t, filepath.Join(directory, "..prefixed"))
	// Reading outside the working directory remains permitted; only writes have this native restriction.
	value, err := native.Read(outside)
	require.NoError(t, err)
	require.Equal(t, "untouched", value)
	_, err = native.Write("bad\x00path", "content")
	require.Error(t, err, "invalid OS paths must fail before writing")
	_, err = native.Write(".", "content")
	require.ErrorContains(t, err, "failed to write")
}

func TestNativeFileNamespaceSymlinkTraversal(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	require.NoError(t, os.Mkdir("target", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("target", "file"), []byte("data"), 0o600))
	if err := os.Symlink("target", "link"); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation requires Windows developer mode or privilege: %v", err)
		}
		require.NoError(t, err)
	}
	native := nativeFileFuncs{}
	paths, err := native.Walk("link")
	require.NoError(t, err)
	require.Equal(t, []string{"link", filepath.Join("link", "file")}, paths)
	// Follow a root directory symlink, but never descend into a child symlink.
	require.NoError(t, os.Mkdir("outside", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("outside", "hidden"), []byte("outside"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join("..", "outside"), filepath.Join("target", "child")))
	paths, err = native.Walk("link")
	require.NoError(t, err)
	require.Equal(t, []string{"link", filepath.Join("link", "child"), filepath.Join("link", "file")}, paths)
	require.NoError(t, os.Symlink("missing", "dangling"))
	_, err = native.Write(filepath.Join("dangling", "file"), "content")
	require.ErrorContains(t, err, "failed to make dirs")
	require.NoDirExists(t, "missing")
}

func TestFileNamespaceCurrentVolume(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	require.NoError(t, os.WriteFile("value.txt", []byte("current-volume"), 0o600))
	funcs := CreateFuncs(t.Context(), &Data{})
	if runtime.GOOS == "windows" {
		require.IsType(t, &nativeFileFuncs{}, funcs["file"].(func() any)())
	}
	tmpl, err := template.New("volume").Funcs(funcs).Parse(`{{ file.Read . }}|{{ file.Exists . }}|{{ (file.Stat .).Name }}`)
	require.NoError(t, err)
	for _, path := range []string{"value.txt", filepath.Join(directory, "value.txt")} {
		var output bytes.Buffer
		require.NoError(t, tmpl.Execute(&output, path))
		require.Equal(t, "current-volume|true|value.txt", output.String())
	}
}
