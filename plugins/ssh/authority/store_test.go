package authority_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
)

func sampleReceipt(name, fp string) authority.Receipt {
	return authority.Receipt{
		Authority: "github", Subject: "octocat", KeyName: name, Fingerprint: fp,
		PublicKey:  "ssh-ed25519 AAAA",
		Uses:       []authority.Use{authority.UseSigning, authority.UseAuthentication},
		References: map[authority.Use]string{authority.UseSigning: "11", authority.UseAuthentication: "22"},
		Created:    time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func newStore(t *testing.T) *authority.Store {
	t.Helper()
	s, err := authority.NewStore(filepath.Join(t.TempDir(), "receipts"))
	require.NoError(t, err)
	return s
}

func TestStoreRoundTrip(t *testing.T) {
	s := newStore(t)
	want := sampleReceipt("ops", "SHA256:abc")
	want.Certificate = []byte{1, 2, 3}

	require.NoError(t, s.Save(want))
	got, err := s.Load("github", "ops")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	want.Subject = "someone"
	require.NoError(t, s.Save(want))
	got, err = s.Load("github", "ops")
	require.NoError(t, err)
	assert.Equal(t, "someone", got.Subject)
}

func TestStorePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix modes do not apply")
	}
	s := newStore(t)
	require.NoError(t, s.Save(sampleReceipt("ops", "SHA256:abc")))

	dir, err := os.Stat(s.Dir())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dir.Mode().Perm())
	entries, err := os.ReadDir(s.Dir())
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary file is left behind")
	info, err := entries[0].Info()
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestStoreLoadMissing(t *testing.T) {
	s := newStore(t)
	_, err := s.Load("github", "ops")
	assert.ErrorIs(t, err, authority.ErrReceiptNotFound)
}

func TestStoreCorruptFile(t *testing.T) {
	tests := map[string]string{
		"not json":        "{{{",
		"wrong version":   `{"version":99,"receipt":{"authority":"github","key_name":"ops","fingerprint":"x"}}`,
		"missing fields":  `{"version":1,"receipt":{"authority":"github"}}`,
		"empty":           ``,
		"unknown version": `{"receipt":{"authority":"github","key_name":"ops","fingerprint":"x"}}`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			require.NoError(t, s.Save(sampleReceipt("ops", "SHA256:abc")))
			files, err := filepath.Glob(filepath.Join(s.Dir(), "*.json"))
			require.NoError(t, err)
			require.Len(t, files, 1)
			require.NoError(t, os.WriteFile(files[0], []byte(content), 0o600))

			_, err = s.Load("github", "ops")
			assert.ErrorIs(t, err, authority.ErrReceiptCorrupt)
			_, err = s.List()
			assert.ErrorIs(t, err, authority.ErrReceiptCorrupt)
		})
	}
}

func TestStoreListSkipsBadFilesButReturnsTheRest(t *testing.T) {
	s := newStore(t)
	require.NoError(t, s.Save(sampleReceipt("zeta", "SHA256:z")))
	require.NoError(t, s.Save(sampleReceipt("alpha", "SHA256:a")))
	other := sampleReceipt("alpha", "SHA256:a")
	other.Authority = "aws"
	require.NoError(t, s.Save(other))
	require.NoError(t, os.WriteFile(filepath.Join(s.Dir(), "junk.json"), []byte("nope"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(s.Dir(), ".tmp-123"), []byte("partial"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(s.Dir(), "notes.txt"), []byte("hi"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(s.Dir(), "sub.json"), 0o700))

	got, err := s.List()
	require.ErrorIs(t, err, authority.ErrReceiptCorrupt)
	require.Len(t, got, 3)
	assert.Equal(t, "aws", got[0].Authority)
	assert.Equal(t, "alpha", got[1].KeyName)
	assert.Equal(t, "zeta", got[2].KeyName)
}

func TestStoreListWithoutDirectory(t *testing.T) {
	s := newStore(t)
	got, err := s.List()
	assert.NoError(t, err)
	assert.Empty(t, got)
}

func TestStoreDelete(t *testing.T) {
	s := newStore(t)
	require.NoError(t, s.Save(sampleReceipt("ops", "SHA256:abc")))
	require.NoError(t, s.Delete("github", "ops"))
	_, err := s.Load("github", "ops")
	assert.ErrorIs(t, err, authority.ErrReceiptNotFound)
	assert.NoError(t, s.Delete("github", "ops"), "deleting twice is fine")
}

func TestStoreKeepsOddNamesInsideTheDirectory(t *testing.T) {
	s := newStore(t)
	names := []string{"../../etc/passwd", `a\b`, "con:1", "ops key", "ops.with.dots", "ünï", strings.Repeat("x", 40)}
	for _, name := range names {
		require.NoError(t, s.Save(sampleReceipt(name, "SHA256:"+name)))
	}
	entries, err := os.ReadDir(s.Dir())
	require.NoError(t, err)
	assert.Len(t, entries, len(names))
	for _, name := range names {
		got, err := s.Load("github", name)
		require.NoError(t, err, name)
		assert.Equal(t, name, got.KeyName)
	}
	parent, err := os.ReadDir(filepath.Dir(s.Dir()))
	require.NoError(t, err)
	assert.Len(t, parent, 1, "nothing was written outside the receipt directory")
}

func TestStoreRefusesToOverwriteAnotherKeysFile(t *testing.T) {
	s := newStore(t)
	require.NoError(t, s.Save(sampleReceipt("ops", "SHA256:abc")))
	files, err := filepath.Glob(filepath.Join(s.Dir(), "*.json"))
	require.NoError(t, err)
	// Simulate a case-insensitive file system, where "OPS" would resolve to the same file.
	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	collide := filepath.Join(s.Dir(), "github.OPS.json")
	require.NoError(t, os.WriteFile(collide, data, 0o600))

	_, err = s.Load("github", "OPS")
	assert.ErrorIs(t, err, authority.ErrReceiptCollision)
	assert.ErrorIs(t, s.Save(sampleReceipt("OPS", "SHA256:other")), authority.ErrReceiptCollision)
}

func TestStoreRejectsEmptyNames(t *testing.T) {
	_, err := authority.NewStore("")
	assert.ErrorIs(t, err, authority.ErrInvalidRequest)
	s := newStore(t)
	assert.ErrorIs(t, s.Save(authority.Receipt{Authority: "github"}), authority.ErrInvalidRequest)
	_, err = s.Load("", "ops")
	assert.ErrorIs(t, err, authority.ErrInvalidRequest)
	assert.ErrorIs(t, s.Delete("github", ""), authority.ErrInvalidRequest)
}

func TestReceiptHelpers(t *testing.T) {
	r := authority.Receipt{
		Uses:       []authority.Use{authority.UseSigning},
		References: map[authority.Use]string{authority.UseAuthentication: "1"},
	}
	assert.Equal(t, []authority.Use{authority.UseAuthentication, authority.UseSigning}, r.EnrolledUses())

	now := time.Now()
	assert.False(t, r.Expired(now), "no expiry never expires")
	r.ExpiresAt = now.Add(time.Minute)
	assert.False(t, r.Expired(now))
	assert.True(t, r.Expired(now.Add(time.Minute)))

	assert.Equal(t, []authority.Use{authority.UseAuthentication, authority.UseSigning},
		authority.NormalizeUses([]authority.Use{authority.UseSigning, "", authority.UseAuthentication, authority.UseSigning}))
}

func TestErrorText(t *testing.T) {
	err := &authority.Error{
		Authority: "github", Operation: "add key", Use: authority.UseSigning, Status: 404,
		Permission: "write:ssh_signing_key", RetryAfter: time.Minute, Detail: "Not Found",
		Err: authority.ErrMissingPermission,
	}
	text := err.Error()
	for _, part := range []string{"github", "add key signing", "write:ssh_signing_key", "404", "1m0s", "Not Found"} {
		assert.Contains(t, text, part)
	}
	assert.ErrorIs(t, err, authority.ErrMissingPermission)
}
