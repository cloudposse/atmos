package homedir

import (
	"os/user"
	"path/filepath"
	"testing"
)

func BenchmarkDir(b *testing.B) {
	// We do this for any "warmups"
	for i := 0; i < 10; i++ {
		Dir()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Dir()
	}
}

func TestDir(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	dir, err := Dir()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if u.HomeDir != dir {
		t.Fatalf("%#v != %#v", u.HomeDir, dir)
	}

	DisableCache = true
	defer func() { DisableCache = false }()
	t.Setenv("HOME", "")
	dir, err = Dir()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if u.HomeDir != dir {
		t.Fatalf("%#v != %#v", u.HomeDir, dir)
	}
}

func TestReset_ClearsCache(t *testing.T) {
	// First call to populate cache.
	dir1, err := Dir()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// Set a different HOME and reset cache.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	Reset()

	// Dir() should now return the new HOME from env var.
	dir2, err := Dir()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if dir1 == dir2 {
		t.Fatalf("Reset() did not clear cache: both calls returned %q", dir1)
	}

	if dir2 != tmpDir {
		t.Fatalf("After Reset(), expected Dir() to return %q, got %q", tmpDir, dir2)
	}
}

func TestReset_WorksAcrossMultipleTests(t *testing.T) {
	// This test reproduces the issue where Reset() doesn't work properly
	// when tests are run multiple times (go test -count=2).

	for i := 0; i < 3; i++ {
		t.Run("iteration", func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)
			Reset()

			dir, err := Dir()
			if err != nil {
				t.Fatalf("iteration %d: err: %s", i, err)
			}

			if dir != tmpDir {
				t.Fatalf("iteration %d: expected Dir() to return %q, got %q", i, tmpDir, dir)
			}
		})
	}
}

func TestExpand(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	cases := []struct {
		Input  string
		Output string
		Err    bool
	}{
		{
			"/foo",
			"/foo",
			false,
		},

		{
			"~/foo",
			filepath.Join(u.HomeDir, "foo"),
			false,
		},

		{
			"",
			"",
			false,
		},

		{
			"~",
			u.HomeDir,
			false,
		},

		{
			"~foo/foo",
			"",
			true,
		},
	}

	for _, tc := range cases {
		actual, err := Expand(tc.Input)
		if (err != nil) != tc.Err {
			t.Fatalf("Input: %#v\n\nErr: %s", tc.Input, err)
		}

		if actual != tc.Output {
			t.Fatalf("Input: %#v\n\nOutput: %#v", tc.Input, actual)
		}
	}

	DisableCache = true
	defer func() { DisableCache = false }()
	t.Setenv("HOME", "/custom/path/")
	expected := filepath.Join(string(filepath.Separator), "custom", "path", "foo", string(filepath.Separator), "bar")
	actual, err := Expand("~/foo/bar")

	if err != nil {
		t.Errorf("No error is expected, got: %v", err)
	} else if actual != expected {
		t.Errorf("Expected: %v; actual: %v", expected, actual)
	}
}
