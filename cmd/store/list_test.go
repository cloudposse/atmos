package store

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	pstore "github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/ui"
)

// decodeJSONRows decodes stdout as the `--format json` renderer's output shape (an array of
// header-name -> string-value objects, e.g. {"Key": "image_tag", "Value": "v1.2.3"}) rather than
// substring-searching it, so a malformed or partially-masked row would fail the assertion instead
// of slipping through.
func decodeJSONRows(t *testing.T, stdout *bytes.Buffer) []map[string]string {
	t.Helper()

	var rows []map[string]string
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &rows))
	return rows
}

// captureStreams is like captureStdout but also wires up the UI (stderr) channel, for tests that
// need to assert an empty-result ui.Info message as well as stdout.
func captureStreams(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()

	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: stderr}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	t.Cleanup(func() { data.Reset() })
	return stdout, stderr
}

func TestDescriptorsToRows(t *testing.T) {
	rows := descriptorsToRows([]pstore.Descriptor{
		{Name: "app-metadata", Kind: pstore.KindAWSSSM, Secret: false, Deletable: true, HasStatus: true, Local: false, Listable: true},
	})
	require.Len(t, rows, 1)
	assert.Equal(t, "app-metadata", rows[0]["name"])
	assert.Equal(t, pstore.KindAWSSSM, rows[0]["kind"])
	assert.Equal(t, true, rows[0]["deletable"])
	assert.Equal(t, true, rows[0]["hasStatus"])
	assert.Equal(t, true, rows[0]["listable"])
}

func TestRunStoreList(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	svc.descs = []pstore.Descriptor{
		{Name: "app-metadata", Kind: pstore.KindAWSSSM},
		{Name: "vault", Kind: pstore.KindHashicorpVault, Secret: true},
	}
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "--format", "json")
	require.NoError(t, err)
}

func TestRunStoreList_Empty(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list")
	require.NoError(t, err)
}

func TestRunStoreList_ServiceError(t *testing.T) {
	setupIO(t)
	installService(t, nil, assert.AnError)

	err := runStoreSubcommand(t, "list")
	require.ErrorIs(t, err, assert.AnError)
}

// TestRunStoreList_DoesNotUseAuthenticatedLoad proves `store list` loads its service via
// loadServiceForListFn, not loadServiceFn -- list only reads store configuration and capability
// metadata, so it must not attempt authentication (which can trigger an unwanted interactive
// identity prompt when a config declares multiple identities with no default).
func TestRunStoreList_DoesNotUseAuthenticatedLoad(t *testing.T) {
	setupIO(t)

	origAuth := loadServiceFn
	loadServiceFn = func(_ storeScope) (storeService, error) {
		t.Fatal("store list must not call loadServiceFn (the authenticated loader)")
		return nil, nil
	}
	t.Cleanup(func() { loadServiceFn = origAuth })

	svc := newFakeStoreService()
	svc.descs = []pstore.Descriptor{{Name: "app-metadata", Kind: pstore.KindAWSSSM}}
	origList := loadServiceForListFn
	loadServiceForListFn = func(_ storeScope) (storeService, error) { return svc, nil }
	t.Cleanup(func() { loadServiceForListFn = origList })

	err := runStoreSubcommand(t, "list")
	require.NoError(t, err)
}

func TestRunStoreListKeyValues(t *testing.T) {
	stdout := captureStdout(t)
	svc := newFakeStoreService()
	svc.keyValues = []pstore.KeyValue{
		{Key: "image_tag", Value: "v1.2.3"},
		{Key: "region", Value: "us-east-1"},
	}
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "app-metadata", "--stack", "dev", "--component", "vpc", "--format", "json")
	require.NoError(t, err)

	require.Len(t, svc.listKeyValuesCalls, 1)
	assert.Equal(t, "app-metadata", svc.listKeyValuesCalls[0].name)
	assert.Equal(t, "dev", svc.listKeyValuesCalls[0].stack)
	assert.Equal(t, "vpc", svc.listKeyValuesCalls[0].component)

	rows := decodeJSONRows(t, stdout)
	require.Len(t, rows, 2)
	assert.Equal(t, map[string]string{"Stack": "dev", "Component": "vpc", "Key": "image_tag", "Value": "v1.2.3"}, rows[0])
	assert.Equal(t, map[string]string{"Stack": "dev", "Component": "vpc", "Key": "region", "Value": "us-east-1"}, rows[1])
}

// TestRunStoreListKeyValues_SecretStoreRegistersMaskedValues proves list registers each
// secret-store value with the masker via registerSecretValueFn before rendering.
func TestRunStoreListKeyValues_SecretStoreRegistersMaskedValues(t *testing.T) {
	setupIO(t)
	registered := overrideRegisterSecretValue(t)
	svc := newFakeStoreService()
	svc.secret = true
	svc.keyValues = []pstore.KeyValue{{Key: "password", Value: "hunter2"}}
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "app-secrets", "--format", "json")
	require.NoError(t, err)
	assert.Equal(t, []any{"hunter2"}, *registered)
}

// TestRunStoreListKeyValues_SecretStoreMasksOutput proves a secret store's values are actually
// masked in the rendered stdout output, not just registered with the masker -- it exercises the
// real io.RegisterSecretValue -> masker -> stdout pipeline end to end (registerSecretValueFn is
// NOT stubbed here, unlike the registration-only test above).
//
// This can't use captureStdout's isolated WithStreams context: registerSecretValueFn always
// registers against pkg/io's own global masker (see io.RegisterSecret), which is only reachable
// through the real global I/O context -- and that context's streams dynamically resolve
// os.Stdout/os.Stderr at write time (see pkg/io/streams.go's newStreams doc comment) rather than
// accepting injected buffers. So this redirects real os.Stdout via os.Pipe, matching the pattern
// already used for the same reason in cmd/cmd_utils_test.go.
func TestRunStoreListKeyValues_SecretStoreMasksOutput(t *testing.T) {
	iolib.Reset()
	t.Cleanup(iolib.Reset)

	oldStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	require.NoError(t, iolib.Initialize())
	ioCtx := iolib.GetContext()
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)

	svc := newFakeStoreService()
	svc.secret = true
	svc.keyValues = []pstore.KeyValue{{Key: "password", Value: "hunter2"}}
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "app-secrets", "--format", "json")
	require.NoError(t, err)

	require.NoError(t, w.Close())
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, copyErr := io.Copy(&buf, r)
	require.NoError(t, copyErr)

	assert.NotContains(t, buf.String(), "hunter2")
	rows := decodeJSONRows(t, &buf)
	require.Len(t, rows, 1)
	assert.Equal(t, "password", rows[0]["Key"])
	assert.Equal(t, iolib.MaskReplacement, rows[0]["Value"])
}

func TestRunStoreListKeyValues_NonSecretStoreDoesNotRegisterValues(t *testing.T) {
	setupIO(t)
	registered := overrideRegisterSecretValue(t)
	svc := newFakeStoreService()
	svc.keyValues = []pstore.KeyValue{{Key: "image_tag", Value: "v1.2.3"}}
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "app-metadata", "--format", "json")
	require.NoError(t, err)
	assert.Empty(t, *registered)
}

func TestRunStoreListKeyValues_Empty(t *testing.T) {
	stdout, stderr := captureStreams(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "app-metadata")
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Contains(t, ansi.Strip(stderr.String()), `No keys found in store "app-metadata".`)
}

func TestRunStoreListKeyValues_NotSupported(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	svc.listKeyValuesErr = assert.AnError
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "app-metadata")
	require.ErrorIs(t, err, assert.AnError)
}

// TestRunStoreListKeyValues_UsesAuthenticatedLoad proves `store list STORE` loads its service via
// loadServiceFn (the authenticated loader), not loadServiceForListFn -- unlike the bare
// `store list`, listing a store's contents needs real backend access.
func TestRunStoreListKeyValues_UsesAuthenticatedLoad(t *testing.T) {
	setupIO(t)

	origList := loadServiceForListFn
	loadServiceForListFn = func(_ storeScope) (storeService, error) {
		t.Fatal("store list STORE must not call loadServiceForListFn (the credential-free loader)")
		return nil, nil
	}
	t.Cleanup(func() { loadServiceForListFn = origList })

	svc := newFakeStoreService()
	origAuth := loadServiceFn
	loadServiceFn = func(_ storeScope) (storeService, error) { return svc, nil }
	t.Cleanup(func() { loadServiceFn = origAuth })

	err := runStoreSubcommand(t, "list", "app-metadata")
	require.NoError(t, err)
}
