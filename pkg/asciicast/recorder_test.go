package asciicast

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolvePathUsesXDGCacheBase(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", temp)

	started := time.Date(2026, 6, 30, 14, 22, 33, 0, time.UTC)
	path, err := ResolvePath(&Options{Command: []string{"terraform", "plan", "vpc"}}, started)
	if err != nil {
		t.Fatal(err)
	}

	wantPrefix := filepath.Join(temp, "atmos", "casts", "2026", "06", "30")
	if !strings.HasPrefix(path, wantPrefix) {
		t.Fatalf("path %q does not start with %q", path, wantPrefix)
	}
	if !strings.Contains(filepath.Base(path), "142233-terraform-plan-vpc-") {
		t.Fatalf("path %q does not include command slug", path)
	}
}

func TestStartPropagatesResolvePathError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ATMOS_XDG_CACHE_HOME", blocker)

	// No explicit Path/BasePath, so Start must call ResolvePath, which fails
	// resolving the XDG cache directory, and Start must propagate that error
	// directly (the `if err != nil { return nil, err }` branch right after
	// ResolvePath, before any directory or file is ever created).
	_, err := Start(&Options{})
	if err == nil {
		t.Fatal("expected Start to propagate ResolvePath error")
	}
}

func TestCommandSlugSkipsOnlyExactAtmosBinary(t *testing.T) {
	if got := CommandSlug([]string{"atmos.exe", "workflow", "run"}); got != "workflow-run" {
		t.Fatalf("slug = %q, want workflow-run", got)
	}
	if got := CommandSlug([]string{"atmosphere", "workflow"}); got != "atmosphere-workflow" {
		t.Fatalf("slug = %q, want atmosphere-workflow", got)
	}
}

func TestRecorderWidth(t *testing.T) {
	if got := (*Recorder)(nil).Width(); got != 0 {
		t.Fatalf("nil recorder width = %d, want 0", got)
	}

	rec := &Recorder{width: 132}
	if got := rec.Width(); got != 132 {
		t.Fatalf("width = %d, want 132", got)
	}
}

func TestStartFailsWhenExplicitPathExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.cast")
	if err := os.WriteFile(path, []byte("exists"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Start(&Options{Path: path, Explicit: true})
	if err == nil {
		t.Fatal("expected explicit path collision error")
	}
	if !errors.Is(err, ErrCastOutputExists) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartRemovesCastFileWhenHeaderWriteFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.cast")
	errHeader := errors.New("header write failed")
	oldWriteRecorderHeader := writeRecorderHeader
	writeRecorderHeader = func(_ *Recorder, _ any) error {
		return errHeader
	}
	t.Cleanup(func() {
		writeRecorderHeader = oldWriteRecorderHeader
	})

	_, err := Start(&Options{Path: path, Explicit: true})
	if !errors.Is(err, errHeader) {
		t.Fatalf("expected header error, got %v", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cast file was not removed after failed start: %v", statErr)
	}
}

func TestCloseCommitsTempFileIntoPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	rec, err := Start(&Options{Path: path, Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("final cast exists before Close: %v", statErr)
	}
	rec.Record("o", "hello")
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "hello") {
		t.Fatal("committed cast is missing recorded output")
	}
	assertNoTempFiles(t, dir)
}

func TestDiscardLeavesExistingCastUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	if err := os.WriteFile(path, []byte("previous good cast"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := Start(&Options{Path: path, Explicit: true, Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	rec.Record("o", "broken output")
	if err := rec.Discard(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "previous good cast" {
		t.Fatalf("discard modified the committed cast: %q", content)
	}
	// Close after Discard must stay a no-op and never touch the final path.
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "previous good cast" {
		t.Fatalf("close after discard modified the committed cast: %q", content)
	}
	assertNoTempFiles(t, dir)
}

func TestOverwriteReplacesExistingCastOnCommit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	if err := os.WriteFile(path, []byte("stale cast"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := Start(&Options{Path: path, Explicit: true, Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	rec.Record("o", "fresh output")
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "fresh output") {
		t.Fatalf("commit did not replace the stale cast: %q", content)
	}
	assertNoTempFiles(t, dir)
}

// assertNoTempFiles fails when a recording leaves temp files behind.
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("temp cast file left behind: %s", entry.Name())
		}
	}
}

func TestRecorderOmitsInputUnlessEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.cast")
	rec, err := Start(&Options{Path: path, Explicit: true, Now: func() time.Time {
		return time.Unix(10, 0)
	}})
	if err != nil {
		t.Fatal(err)
	}
	rec.Record("i", "secret input")
	rec.Record("o", "visible output")
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "secret input") {
		t.Fatal("input was recorded even though input recording is disabled")
	}
	if !strings.Contains(string(content), "visible output") {
		t.Fatal("output event was not recorded")
	}
}

func TestStartWritesHeaderDefaultsAndSafeEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "demo.cast")
	started := time.Unix(123, 0)

	rec, err := Start(&Options{
		Path:     path,
		Explicit: true,
		Title:    "Workflow Run",
		Command:  []string{"atmos", "workflow", "run"},
		Env: map[string]string{
			"SHELL":        "/bin/zsh",
			"TERM":         "xterm-256color",
			"COLORTERM":    "truecolor",
			"SECRET_TOKEN": "redacted",
		},
		Now: func() time.Time { return started },
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Path() != path {
		t.Fatalf("path = %q, want %q", rec.Path(), path)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	lines := readCastLines(t, path)
	var header Header
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatal(err)
	}
	if header.Version != 3 || header.Term == nil || header.Term.Cols != DefaultWidth || header.Term.Rows != DefaultHeight {
		t.Fatalf("unexpected header: %#v", header)
	}
	if header.Term.Type != "xterm-256color" {
		t.Fatalf("terminal type = %q", header.Term.Type)
	}
	if header.Timestamp != started.Unix() {
		t.Fatalf("timestamp = %d, want %d", header.Timestamp, started.Unix())
	}
	if header.Title != "Workflow Run" {
		t.Fatalf("title = %q", header.Title)
	}
	if header.Command != "atmos workflow run" {
		t.Fatalf("command = %q", header.Command)
	}
	if header.Env["SHELL"] != "/bin/zsh" || header.Env["COLORTERM"] != "truecolor" {
		t.Fatalf("safe env missing expected keys: %#v", header.Env)
	}
	if _, ok := header.Env["TERM"]; ok {
		t.Fatalf("TERM should be stored in term.type for v3, env: %#v", header.Env)
	}
	if _, ok := header.Env["SECRET_TOKEN"]; ok {
		t.Fatalf("unsafe env was recorded: %#v", header.Env)
	}
}

func TestRecorderRecordsInputWhenEnabledAndNormalizesStreams(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.cast")
	rec, err := Start(&Options{Path: path, Explicit: true, RecordIn: true})
	if err != nil {
		t.Fatal(err)
	}

	rec.Record("i", "typed")
	rec.Record("debug", "visible")
	if err := rec.Resize(100, 24); err != nil {
		t.Fatal(err)
	}
	if err := rec.Event("e", "stderr"); err != nil {
		t.Fatal(err)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("second close should be ignored: %v", err)
	}
	if err := rec.Event("o", "ignored"); err != nil {
		t.Fatalf("event after close should be ignored: %v", err)
	}
	if err := rec.Resize(1, 1); err != nil {
		t.Fatalf("resize after close should be ignored: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"i","typed"`, `"o","visible"`, `"r","100x24"`, `"e","stderr"`} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("missing %s in:\n%s", want, content)
		}
	}
	if strings.Contains(string(content), "ignored") {
		t.Fatalf("closed recorder wrote event:\n%s", content)
	}
}

func TestRecorderOutputRateSplitsTerminalLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.cast")
	rec, err := Start(&Options{Path: path, Explicit: true, OutputRate: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Event("o", "one\ntwo\nthree"); err != nil {
		t.Fatal(err)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	lines := readCastLines(t, path)
	if len(lines) != 4 {
		t.Fatalf("line count = %d, want header plus 3 events:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for _, want := range []string{`"one\n"`, `"two\n"`, `"three"`} {
		if !strings.Contains(strings.Join(lines, "\n"), want) {
			t.Fatalf("missing split chunk %s in %v", want, lines)
		}
	}
}

func TestRecorderNilAndEmptyOperationsAreNoops(t *testing.T) {
	var rec *Recorder
	if rec.Path() != "" {
		t.Fatal("nil recorder path should be empty")
	}
	rec.Record("o", "ignored")
	if err := rec.Close(); err != nil {
		t.Fatalf("nil close error: %v", err)
	}

	if got := splitTerminalLines("plain"); len(got) != 1 || got[0] != "plain" {
		t.Fatalf("split without newline = %#v", got)
	}
	if got := splitTerminalLines("a\nb\n"); len(got) != 2 || got[0] != "a\n" || got[1] != "b\n" {
		t.Fatalf("split with trailing newline = %#v", got)
	}
	if maxDuration(time.Second, time.Millisecond) != time.Second {
		t.Fatal("maxDuration did not return larger value")
	}
	if maxDuration(time.Millisecond, time.Second) != time.Second {
		t.Fatal("maxDuration did not return larger value when b > a")
	}
	if env := safeEnv(map[string]string{"SECRET": "x"}); env != nil {
		t.Fatalf("safeEnv = %#v, want nil", env)
	}
}

func TestResolvePathExplicitBaseAndCommandSlug(t *testing.T) {
	started := time.Date(2026, 7, 1, 1, 2, 3, 0, time.UTC)
	explicit := filepath.Join(t.TempDir(), "..", "demo.cast")
	path, err := ResolvePath(&Options{Path: explicit}, started)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Clean(explicit) {
		t.Fatalf("explicit path = %q, want clean path", path)
	}

	base := t.TempDir()
	path, err = ResolvePath(&Options{BasePath: base, Command: []string{"/usr/local/bin/atmos", "--flag", "terraform", "plan", "vpc", "extra"}}, started)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, filepath.Join(base, "2026", "07", "01")) {
		t.Fatalf("path = %q", path)
	}
	if !strings.Contains(filepath.Base(path), "010203-terraform-plan-vpc-extra-") {
		t.Fatalf("path does not include slug: %q", path)
	}
	if slug := CommandSlug([]string{"atmos", "-s", "dev", "terraform!!!", strings.Repeat("x", 100)}); len(slug) > slugMaxLen {
		t.Fatalf("slug too long: %q", slug)
	}
	if slug := CommandSlug([]string{"atmos", "--help"}); slug != "" {
		t.Fatalf("slug = %q, want empty", slug)
	}

	path, err = ResolvePath(&Options{BasePath: base, Title: "Quick Start: List Instances"}, started)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(path), "010203-quick-start-list-instances-") {
		t.Fatalf("title fallback slug missing: %q", path)
	}
}

func TestReadEventsAndPlayV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.cast")
	content := strings.Join([]string{
		`{"version":2,"width":80,"height":24,"timestamp":1}`,
		`[0,"o","hello"]`,
		`[0,"i","ignored"]`,
		`[0,"e"," error"]`,
		`[0,"o"]`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	header, events, err := ReadEvents(path)
	if err != nil {
		t.Fatal(err)
	}
	if header.Width != 80 || len(events) != 3 {
		t.Fatalf("header=%#v events=%#v", header, events)
	}
	var out strings.Builder
	if err := Play(path, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello error" {
		t.Fatalf("played output = %q", out.String())
	}
}

func TestReadEventsV3AccumulatesRelativeTimesAndSkipsComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.cast")
	content := strings.Join([]string{
		`{"version":3,"term":{"cols":80,"rows":24,"type":"xterm-256color"},"timestamp":1,"title":"Demo"}`,
		`# comment`,
		`[0.5,"o","hello"]`,
		`[0.25,"x","0"]`,
		`[0.75,"e"," error"]`,
		`[0.25,"i","ignored by play"]`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	header, events, err := ReadEvents(path)
	if err != nil {
		t.Fatal(err)
	}
	if header.Version != 3 || header.Term == nil || header.Term.Cols != 80 || header.Term.Rows != 24 || header.Title != "Demo" {
		t.Fatalf("unexpected header: %#v", header)
	}
	if len(events) != 3 {
		t.Fatalf("events=%#v", events)
	}
	if events[0].Time != 0.5 || events[1].Time != 1.5 || events[2].Time != 1.75 {
		t.Fatalf("events were not accumulated: %#v", events)
	}
	var out strings.Builder
	if err := Play(path, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello error" {
		t.Fatalf("played output = %q", out.String())
	}
}

func TestReadEventsErrors(t *testing.T) {
	temp := t.TempDir()
	empty := filepath.Join(temp, "empty.cast")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := ReadEvents(empty)
	if !errors.Is(err, ErrEmptyCastFile) {
		t.Fatalf("expected empty cast error, got %v", err)
	}

	badHeader := filepath.Join(temp, "bad-header.cast")
	if err := os.WriteFile(badHeader, []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = ReadEvents(badHeader)
	if err == nil || !strings.Contains(err.Error(), "decode cast header") {
		t.Fatalf("expected header decode error, got %v", err)
	}

	badEvent := filepath.Join(temp, "bad-event.cast")
	if err := os.WriteFile(badEvent, []byte("{}\n[\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = ReadEvents(badEvent)
	if err == nil || !strings.Contains(err.Error(), "decode cast event") {
		t.Fatalf("expected event decode error, got %v", err)
	}
}

func readCastLines(t *testing.T, path string) []string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}

func TestStartDefaultsNilOptions(t *testing.T) {
	rec, err := Start(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rec.Discard() }()

	if rec.width != DefaultWidth || rec.height != DefaultHeight {
		t.Fatalf("unexpected defaults: width=%d height=%d", rec.width, rec.height)
	}
	if rec.command != "" {
		t.Fatalf("command = %q, want empty", rec.command)
	}
}

func TestStartFailsWhenMkdirAllTargetIsAFile(t *testing.T) {
	dir := t.TempDir()
	// Make the parent directory component a plain file so MkdirAll fails
	// cross-platform (works on both Unix and Windows, no chmod needed).
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "nested", "demo.cast")

	_, err := Start(&Options{Path: path, Explicit: true})
	if err == nil {
		t.Fatal("expected mkdir failure")
	}
	if !strings.Contains(err.Error(), "create cast directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateCastTempFileErrorsWhenParentDirMissing(t *testing.T) {
	// CreateTemp fails because the parent directory doesn't exist.
	path := filepath.Join(t.TempDir(), "missing-parent", "demo.cast")
	_, err := createCastTempFile(path, true)
	if err == nil {
		t.Fatal("expected create temp file error")
	}
	if !strings.Contains(err.Error(), "create cast file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCloseReturnsNilForNilRecorder(t *testing.T) {
	var rec *Recorder
	if err := rec.Close(); err != nil {
		t.Fatalf("nil recorder close error: %v", err)
	}
}

func TestDiscardReturnsNilForNilRecorder(t *testing.T) {
	var rec *Recorder
	if err := rec.Discard(); err != nil {
		t.Fatalf("nil recorder discard error: %v", err)
	}
}

func TestCloseRemovesTempFileWhenCloseFileErrors(t *testing.T) {
	dir := t.TempDir()
	tempFile, err := os.CreateTemp(dir, ".demo.cast.tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	tempPath := tempFile.Name()

	rec := &Recorder{
		file: tempFile,
		// A flush error from the broken writer surfaces as closeFile's err
		// while tempPath stays non-empty, so Close must remove the temp
		// file and return the error directly (bypassing commit).
		writer:   bufio.NewWriterSize(&errWriter{failAfter: 0}, 1),
		path:     filepath.Join(dir, "demo.cast"),
		tempPath: tempPath,
	}
	// Buffer something so Flush has content to flush and fail on.
	_, _ = rec.writer.WriteString("x")

	err = rec.Close()
	if !errors.Is(err, errSimulatedWrite) {
		t.Fatalf("expected simulated write error, got %v", err)
	}
	if _, statErr := os.Stat(tempPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("temp file was not removed after failed close: %v", statErr)
	}
}

func TestDiscardIsNoopAfterAlreadyClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	rec, err := Start(&Options{Path: path, Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Discard(); err != nil {
		t.Fatal(err)
	}
	// Second Discard: closeFile returns ("", nil) since already closed, so
	// Discard must short-circuit via tempPath == "" and return nil directly.
	if err := rec.Discard(); err != nil {
		t.Fatalf("second discard should be a no-op: %v", err)
	}
}

func TestCloseReturnsErrDirectlyWhenTempPathEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	rec, err := Start(&Options{Path: path, Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	// Second close: closeFile returns ("", nil) since already closed, so
	// Close must bypass commit and return nil directly (tempPath == "").
	if err := rec.Close(); err != nil {
		t.Fatalf("second close should bypass commit and return nil: %v", err)
	}
}

func TestDiscardReturnsRemoveErrWhenCloseErrIsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	rec, err := Start(&Options{Path: path, Explicit: true})
	if err != nil {
		t.Fatal(err)
	}
	tempPath := rec.tempPath
	// Remove the temp file out from under the recorder so Discard's own
	// os.Remove call fails with a non-nil removeErr while closeFile's err is nil.
	if err := os.Remove(tempPath); err != nil {
		t.Fatal(err)
	}
	err = rec.Discard()
	if err == nil {
		t.Fatal("expected remove error from discard")
	}
}

func TestCommitFailsWhenRemovingNonEmptyDestinationDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	// r.path is a non-empty directory, so os.Remove(r.path) fails on both
	// Unix and Windows without needing chmod tricks.
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tempFile, err := os.CreateTemp(dir, ".demo.cast.tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	tempPath := tempFile.Name()
	_ = tempFile.Close()

	rec := &Recorder{path: path}
	err = rec.commit(tempPath)
	if err == nil {
		t.Fatal("expected commit error when destination is a non-empty directory")
	}
	if !strings.Contains(err.Error(), "commit cast file") {
		t.Fatalf("unexpected error: %v", err)
	}
	// commit must clean up the temp file on failure.
	if _, statErr := os.Stat(tempPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("temp file was not removed after failed commit: %v", statErr)
	}
}

func TestCommitSucceedsWhenDestinationMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	tempFile, err := os.CreateTemp(dir, ".demo.cast.tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	tempPath := tempFile.Name()
	if _, err := tempFile.WriteString("payload"); err != nil {
		t.Fatal(err)
	}
	_ = tempFile.Close()

	rec := &Recorder{path: path}
	if err := rec.commit(tempPath); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "payload" {
		t.Fatalf("committed content = %q", content)
	}
}

func TestCommitRemovesExistingDestinationThenRenamesSuccessfully(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	// An empty directory at r.path makes the first os.Rename fail (renaming
	// a file onto a directory is rejected on both Unix and Windows), but
	// os.Remove succeeds because the directory is empty, letting the
	// second os.Rename succeed and replace it with the temp file's content.
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	tempFile, err := os.CreateTemp(dir, ".demo.cast.tmp-*")
	if err != nil {
		t.Fatal(err)
	}
	tempPath := tempFile.Name()
	if _, err := tempFile.WriteString("recommitted payload"); err != nil {
		t.Fatal(err)
	}
	_ = tempFile.Close()

	rec := &Recorder{path: path}
	if err := rec.commit(tempPath); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "recommitted payload" {
		t.Fatalf("committed content = %q", content)
	}
}

func TestCommitFailsOnSecondRenameAfterRemovingDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	// An empty directory at r.path makes the first rename fail but the
	// subsequent os.Remove succeed (removing an empty dir works on both
	// Unix and Windows). A tempPath that never existed makes the *second*
	// os.Rename fail identically, exercising commit's final error branch.
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	tempPath := filepath.Join(dir, "never-created-temp")

	rec := &Recorder{path: path}
	err := rec.commit(tempPath)
	if err == nil {
		t.Fatal("expected commit error on second rename failure")
	}
	if !strings.Contains(err.Error(), "commit cast file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteJSONPropagatesMarshalWriteAndByteErrors(t *testing.T) {
	payload := []any{1}
	marshaled, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	// A buffer smaller than the marshaled payload forces bufio's "large
	// write, empty buffer" path, which writes directly to the underlying
	// writer and surfaces its error immediately from r.writer.Write(b).
	rec := &Recorder{writer: bufio.NewWriterSize(&errWriter{failAfter: 0}, 1)}
	if err := rec.writeJSON(payload); !errors.Is(err, errSimulatedWrite) {
		t.Fatalf("expected write error, got %v", err)
	}

	// A buffer sized exactly to the marshaled payload lets the first Write
	// buffer without flushing (bufio only flushes when len(p) exceeds
	// Available()), so the underlying writer's first call happens on the
	// subsequent WriteByte('\n') flush instead.
	rec = &Recorder{writer: bufio.NewWriterSize(&errWriter{failAfter: 0}, len(marshaled))}
	if err := rec.writeJSON(payload); !errors.Is(err, errSimulatedWrite) {
		t.Fatalf("expected byte write error, got %v", err)
	}

	rec = &Recorder{writer: bufio.NewWriter(&bytes.Buffer{})}
	if err := rec.writeJSON(make(chan int)); err == nil {
		t.Fatal("expected marshal error for unsupported type")
	}
}

func TestWriteRelativeEventPropagatesWriteJSONError(t *testing.T) {
	rec := &Recorder{writer: bufio.NewWriterSize(&errWriter{failAfter: 0}, 1)}
	err := rec.writeRelativeEvent(time.Second, "o", "hi")
	if !errors.Is(err, errSimulatedWrite) {
		t.Fatalf("expected broken writer error, got %v", err)
	}
}

func TestWriteEventLockedPropagatesChunkWriteError(t *testing.T) {
	rec := &Recorder{
		writer:     bufio.NewWriterSize(&errWriter{failAfter: 0}, 1),
		started:    time.Now(),
		outputRate: time.Second,
	}
	if err := rec.writeEventLocked("o", "line one\nline two"); !errors.Is(err, errSimulatedWrite) {
		t.Fatalf("expected broken writer error, got %v", err)
	}
}

func TestWriteRelativeEventClampsNegativeDeltaToZero(t *testing.T) {
	rec := &Recorder{
		writer:        bufio.NewWriter(&bytes.Buffer{}),
		lastEventTime: 5 * time.Second,
	}
	// eventTime (1s) is before lastEventTime (5s), producing a negative delta
	// that must clamp to zero rather than go negative.
	if err := rec.writeRelativeEvent(time.Second, "o", "late"); err != nil {
		t.Fatal(err)
	}
	if rec.lastEventTime != time.Second {
		t.Fatalf("lastEventTime = %s, want 1s", rec.lastEventTime)
	}
}

func TestResolvePathDefaultsNilOptions(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", temp)

	path, err := ResolvePath(nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("expected non-empty resolved path for nil options")
	}
}

func TestResolvePathPropagatesXDGCacheDirError(t *testing.T) {
	temp := t.TempDir()
	// Point ATMOS_XDG_CACHE_HOME at a plain file so the underlying
	// os.MkdirAll inside xdg.GetXDGCacheDir fails, forcing ResolvePath's
	// error branch when no explicit BasePath is supplied.
	blocker := filepath.Join(temp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ATMOS_XDG_CACHE_HOME", blocker)

	_, err := ResolvePath(&Options{}, time.Now())
	if err == nil {
		t.Fatal("expected xdg cache dir resolution error")
	}
}

func TestSlugResolutionFallsBackToNameThenDefault(t *testing.T) {
	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	base := t.TempDir()

	// No Command, no Title, but Name is set: falls back to CommandSlug([]string{opts.Name}).
	path, err := ResolvePath(&Options{BasePath: base, Name: "my-recording"}, started)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(path), "-my-recording-") {
		t.Fatalf("path %q missing name-derived slug", path)
	}

	// Nothing at all resolves: falls back to defaultCastCmd ("atmos").
	path, err = ResolvePath(&Options{BasePath: base}, started)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(path), "-"+defaultCastCmd+"-") {
		t.Fatalf("path %q missing default command slug", path)
	}
}

func TestRandomIDProducesDistinctHexIDs(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		id := RandomID(defaultIDLen)
		if len(id) != defaultIDLen {
			t.Fatalf("id length = %d, want %d", len(id), defaultIDLen)
		}
		for _, r := range id {
			if !strings.ContainsRune("0123456789abcdef", r) {
				t.Fatalf("id %q contains non-hex rune %q", id, r)
			}
		}
		seen[id] = true
	}
	if len(seen) < 2 {
		t.Fatalf("RandomID produced no variation across calls: %v", seen)
	}
}

func TestSafeEnvFiltersPresentAbsentAndEmptyKeys(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want map[string]string
	}{
		{
			name: "present keys pass through",
			env:  map[string]string{"SHELL": "/bin/zsh", "TERM": "xterm", "COLORTERM": "truecolor"},
			want: map[string]string{"SHELL": "/bin/zsh", "TERM": "xterm", "COLORTERM": "truecolor"},
		},
		{
			name: "absent keys are omitted",
			env:  map[string]string{"OTHER": "value"},
			want: nil,
		},
		{
			name: "empty-string values are omitted",
			env:  map[string]string{"SHELL": "", "TERM": "", "COLORTERM": ""},
			want: nil,
		},
		{
			name: "nil env returns nil",
			env:  nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeEnv(tt.env)
			if len(got) != len(tt.want) {
				t.Fatalf("safeEnv(%#v) = %#v, want %#v", tt.env, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("safeEnv(%#v)[%q] = %q, want %q", tt.env, k, got[k], v)
				}
			}
		})
	}
}

func TestSafeEnvV3FiltersPresentAbsentAndEmptyKeys(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want map[string]string
	}{
		{
			name: "present keys pass through",
			env:  map[string]string{"SHELL": "/bin/bash", "COLORTERM": "truecolor"},
			want: map[string]string{"SHELL": "/bin/bash", "COLORTERM": "truecolor"},
		},
		{
			name: "absent keys are omitted",
			env:  map[string]string{"TERM": "xterm"},
			want: nil,
		},
		{
			name: "empty-string values are omitted",
			env:  map[string]string{"SHELL": "", "COLORTERM": ""},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeEnvV3(tt.env)
			if len(got) != len(tt.want) {
				t.Fatalf("safeEnvV3(%#v) = %#v, want %#v", tt.env, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("safeEnvV3(%#v)[%q] = %q, want %q", tt.env, k, got[k], v)
				}
			}
		})
	}
}
