package oci

import (
	"archive/tar"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/oci/ocitest"
)

func TestCheckBundlePath(t *testing.T) {
	allowed := []string{"bin/plugin", "bin/plugin.exe", "bin/plugin..exe", "plugin.yaml", "schemas/provider.json", "schemas/nested/dir/broker.json"}
	for _, path := range allowed {
		t.Run("allowed/"+path, func(t *testing.T) {
			require.NoError(t, CheckBundlePath(path))
		})
	}
	forbidden := []string{
		".git/config", "bin/.hidden", ".env", ".terraform/providers/x", ".atmos/credentials", ".aws/credentials", ".ssh/id_rsa", ".netrc",
		"schemas/.credentials.json", "state.tfstate", "terraform.tfstate.backup", "prod.tfvars", "prod.tfvars.json", "server.pem", "tls.key",
		"cert.p12", "cert.pfx", "id_rsa", "id_ed25519.pub", "bin/Credentials.json", "my-SECRET.txt", "terraform.rc", ".terraformrc",
		"/abs/path", "../escape", "a/../b", "a//b", "./a", "a\\b", "", "dir/", "a/b\x00c",
		"C:/escape", "C:escape", "bin/plugin:stream",
	}
	for _, path := range forbidden {
		t.Run("forbidden/"+path, func(t *testing.T) {
			err := CheckBundlePath(path)
			require.ErrorIs(t, err, errUtils.ErrPluginBundlePathForbidden)
			require.Contains(t, err.Error(), strconv.Quote(path), "message names the offending path")
		})
	}
}

func TestCheckBundlePathCarriesAnActionableHint(t *testing.T) {
	err := CheckBundlePath(".git/config")
	require.Error(t, err)
	require.NotEmpty(t, errUtils.AllHints(err), "forbidden paths must explain how to fix the manifest")
}

func tarOf(t *testing.T, headers ...*tar.Header) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, header := range headers {
		content := []byte("x")
		if header.Typeflag == tar.TypeReg {
			header.Size = int64(len(content))
		}
		require.NoError(t, writer.WriteHeader(header))
		if header.Typeflag == tar.TypeReg {
			_, err := writer.Write(content)
			require.NoError(t, err)
		}
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func TestScanBundleRejectsEveryUnsafeEntryClass(t *testing.T) {
	regular := func(name string) *tar.Header { return &tar.Header{Name: name, Mode: 0o644, Typeflag: tar.TypeReg} }
	tests := map[string][]*tar.Header{
		"hidden file":     {regular("plugin.yaml"), regular(".git/config")},
		"hidden schema":   {regular("schemas/.credentials.json")},
		"symlink":         {{Name: "schemas/link.json", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink, Mode: 0o777}},
		"hardlink":        {{Name: "schemas/hard.json", Linkname: "plugin.yaml", Typeflag: tar.TypeLink, Mode: 0o644}},
		"directory":       {{Name: "schemas/", Typeflag: tar.TypeDir, Mode: 0o755}},
		"character dev":   {{Name: "dev", Typeflag: tar.TypeChar, Mode: 0o644}},
		"fifo":            {{Name: "pipe", Typeflag: tar.TypeFifo, Mode: 0o644}},
		"absolute path":   {regular("/etc/passwd")},
		"parent traverse": {regular("../x")},
		"drive absolute":  {regular("C:/escape")},
		"drive relative":  {regular("C:escape")},
		"data stream":     {regular("bin/plugin:stream")},
	}
	for name, headers := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := scanBundle(bytes.NewReader(tarOf(t, headers...)), &bundleRules{})
			require.ErrorIs(t, err, errUtils.ErrPluginBundlePathForbidden)
		})
	}
	t.Run("duplicate entries differing only by case", func(t *testing.T) {
		_, err := scanBundle(bytes.NewReader(tarOf(t, regular("bin/plugin"), regular("bin/Plugin"))), &bundleRules{})
		require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	})
	t.Run("clean entries list size and mode", func(t *testing.T) {
		files, err := scanBundle(bytes.NewReader(tarOf(t, regular("plugin.yaml"), &tar.Header{Name: "bin/plugin", Mode: 0o755, Typeflag: tar.TypeReg})), &bundleRules{})
		require.NoError(t, err)
		require.Equal(t, []BundleFile{{Path: "plugin.yaml", Size: 1, Mode: "0644"}, {Path: "bin/plugin", Size: 1, Mode: "0755"}}, files)
	})
	t.Run("double dots within a filename are local", func(t *testing.T) {
		files, err := scanBundle(bytes.NewReader(tarOf(t, regular("bin/plugin..exe"))), &bundleRules{})
		require.NoError(t, err)
		require.Equal(t, []BundleFile{{Path: "bin/plugin..exe", Size: 1, Mode: "0644"}}, files)
	})
}

func TestScanBundleExactFileSet(t *testing.T) {
	regular := func(name string) *tar.Header { return &tar.Header{Name: name, Mode: 0o644, Typeflag: tar.TypeReg} }
	rules := func() *bundleRules { return &bundleRules{executable: "bin/plugin", exactSet: true} }
	_, err := scanBundle(bytes.NewReader(tarOf(t, regular("plugin.yaml"), regular("bin/plugin"), regular("schemas/a.json"))), rules())
	require.NoError(t, err)
	for name, entries := range map[string][]*tar.Header{
		"extra file":        {regular("plugin.yaml"), regular("bin/plugin"), regular("notes.txt")},
		"extra binary":      {regular("plugin.yaml"), regular("bin/plugin"), regular("bin/other")},
		"non-json schema":   {regular("plugin.yaml"), regular("bin/plugin"), regular("schemas/a.txt")},
		"missing manifest":  {regular("bin/plugin")},
		"missing exec":      {regular("plugin.yaml")},
		"nested manifest":   {regular("plugin.yaml"), regular("bin/plugin"), regular("pkg/plugin.yaml")},
		"schemas outside":   {regular("plugin.yaml"), regular("bin/plugin"), regular("other/schemas/a.json")},
		"bare schema entry": {regular("plugin.yaml"), regular("bin/plugin"), regular("schemas")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := scanBundle(bytes.NewReader(tarOf(t, entries...)), rules())
			require.Error(t, err)
		})
	}
}

func platformOf(t *testing.T, value string) *v1.Platform {
	t.Helper()
	platform, err := parsePlatform(value)
	require.NoError(t, err)
	return platform
}

func TestExecutableFormatMatrix(t *testing.T) {
	supported := map[string][]string{
		"linux":   {"amd64", "arm64", "386", "arm", "ppc64le", "ppc64", "s390x", "riscv64", "loong64"},
		"freebsd": {"amd64", "arm64"},
		"darwin":  {"amd64", "arm64", "386", "arm"},
		"windows": {"amd64", "arm64", "386", "arm"},
	}
	for goos, arches := range supported {
		for _, goarch := range arches {
			t.Run("accepts "+goos+"/"+goarch, func(t *testing.T) {
				binary := ocitest.Binary(goos, goarch)
				require.NoError(t, checkExecutableFormat(bytes.NewReader(binary), platformOf(t, goos+"/"+goarch)))
			})
		}
	}
	rejections := []struct {
		name     string
		binary   []byte
		platform string
	}{
		{"text file", []byte("[core]\n\trepositoryformatversion = 0\n"), "linux/amd64"},
		{"empty file", nil, "linux/amd64"},
		{"git config for darwin", []byte("[core]\n"), "darwin/arm64"},
		{"git config for windows", []byte("[core]\n"), "windows/amd64"},
		{"mach-o on linux", ocitest.Binary("darwin", "amd64"), "linux/amd64"},
		{"pe on linux", ocitest.Binary("windows", "amd64"), "linux/amd64"},
		{"elf on darwin", ocitest.Binary("linux", "arm64"), "darwin/arm64"},
		{"elf on windows", ocitest.Binary("linux", "amd64"), "windows/amd64"},
		{"wrong arch elf", ocitest.Binary("linux", "arm64"), "linux/amd64"},
		{"wrong arch mach-o", ocitest.Binary("darwin", "amd64"), "darwin/arm64"},
		{"wrong arch pe", ocitest.Binary("windows", "arm64"), "windows/amd64"},
		{"wrong byte order", ocitest.Binary("linux", "ppc64"), "linux/ppc64le"},
		{"unsupported os", ocitest.Binary("linux", "amd64"), "plan9/amd64"},
		{"unsupported arch", ocitest.Binary("linux", "amd64"), "linux/mips64"},
	}
	for _, tt := range rejections {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			err := checkExecutableFormat(bytes.NewReader(tt.binary), platformOf(t, tt.platform))
			require.ErrorIs(t, err, errUtils.ErrPluginBundleExecutableInvalid)
			require.NotEmpty(t, errUtils.AllHints(err))
		})
	}
}

func TestExecutableFormatRejectsNonExecutableObjects(t *testing.T) {
	// A shared object is not an executable program for Mach-O or PE; a relocatable ELF is never executable.
	relocatable := ocitest.Binary("linux", "amd64")
	binary.LittleEndian.PutUint16(relocatable[16:], 1)
	require.ErrorIs(t, checkExecutableFormat(bytes.NewReader(relocatable), platformOf(t, "linux/amd64")), errUtils.ErrPluginBundleExecutableInvalid)
	library := ocitest.Binary("windows", "amd64")
	binary.LittleEndian.PutUint16(library[len(library)-2:], 0x2022)
	require.ErrorIs(t, checkExecutableFormat(bytes.NewReader(library), platformOf(t, "windows/amd64")), errUtils.ErrPluginBundleExecutableInvalid)
	dylib := ocitest.Binary("darwin", "arm64")
	binary.LittleEndian.PutUint32(dylib[12:], 6)
	require.ErrorIs(t, checkExecutableFormat(bytes.NewReader(dylib), platformOf(t, "darwin/arm64")), errUtils.ErrPluginBundleExecutableInvalid)
}

func universalBinary(slices ...[]byte) []byte {
	cpus := map[string]uint32{"arm64": 0x0100000c, "amd64": 0x01000007}
	var buffer bytes.Buffer
	write := func(values ...uint32) {
		for _, value := range values {
			_ = binary.Write(&buffer, binary.BigEndian, value)
		}
	}
	write(0xcafebabe, uint32(len(slices)))
	offset := uint32(4096)
	offsets := make([]uint32, len(slices))
	for index, slice := range slices {
		arch := "arm64"
		if binary.LittleEndian.Uint32(slice[4:]) == cpus["amd64"] {
			arch = "amd64"
		}
		offsets[index] = offset
		write(cpus[arch], 0, offset, uint32(len(slice)), 12)
		offset += 4096
	}
	for index, slice := range slices {
		for uint32(buffer.Len()) < offsets[index] {
			buffer.WriteByte(0)
		}
		buffer.Write(slice)
	}
	return buffer.Bytes()
}

func TestExecutableFormatUniversalBinaryNeedsMatchingSlice(t *testing.T) {
	universal := universalBinary(ocitest.Binary("darwin", "arm64"), ocitest.Binary("darwin", "amd64"))
	require.NoError(t, checkExecutableFormat(bytes.NewReader(universal), platformOf(t, "darwin/arm64")))
	require.NoError(t, checkExecutableFormat(bytes.NewReader(universal), platformOf(t, "darwin/amd64")))
	arm64Only := universalBinary(ocitest.Binary("darwin", "arm64"))
	require.ErrorIs(t, checkExecutableFormat(bytes.NewReader(arm64Only), platformOf(t, "darwin/amd64")), errUtils.ErrPluginBundleExecutableInvalid)
	require.ErrorIs(t, checkExecutableFormat(bytes.NewReader(universal), platformOf(t, "linux/amd64")), errUtils.ErrPluginBundleExecutableInvalid)
}

func TestExecutableFormatAcceptsTheHostTestBinary(t *testing.T) {
	if _, supported := map[string]bool{"linux": true, "darwin": true, "windows": true}[runtime.GOOS]; !supported {
		t.Skipf("no native format check for %s", runtime.GOOS)
	}
	path, err := os.Executable()
	require.NoError(t, err)
	file, err := os.Open(filepath.Clean(path))
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })
	require.NoError(t, checkExecutableFormat(file, &v1.Platform{OS: runtime.GOOS, Architecture: runtime.GOARCH}))
	other := "amd64"
	if runtime.GOARCH == "amd64" {
		other = "arm64"
	}
	require.ErrorIs(t, checkExecutableFormat(file, &v1.Platform{OS: runtime.GOOS, Architecture: other}), errUtils.ErrPluginBundleExecutableInvalid)
}
