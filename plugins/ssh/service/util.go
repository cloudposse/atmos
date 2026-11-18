package service

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxCapture bounds how much of a command's output is kept.
const maxCapture = 1 << 20

// limitedBuffer is a bytes.Buffer that stops growing at maxCapture.
type limitedBuffer struct{ buf bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := maxCapture - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string { return b.buf.String() }

func isAbs(p string) bool { return filepath.IsAbs(p) }

// writeFileAtomic writes data to path through a temporary file in the same directory.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	name := tmp.Name()
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(fmt.Errorf("write %s: %w", name, err))
	}
	if err := tmp.Chmod(mode); err != nil && os.PathSeparator != '\\' {
		return fail(fmt.Errorf("set mode on %s: %w", name, err))
	}
	if err := tmp.Sync(); err != nil {
		return fail(fmt.Errorf("sync %s: %w", name, err))
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// failure turns a non-zero command result into an error that carries the command's own words.
func failure(what string, out Output) error {
	detail := strings.TrimSpace(out.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(out.Stdout)
	}
	return fmt.Errorf("%w: %s exited with %d: %s", ErrCommandFailed, what, out.ExitCode, detail)
}
