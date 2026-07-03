package asciicast

import (
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

func TestCommandSlugSkipsOnlyExactAtmosBinary(t *testing.T) {
	if got := CommandSlug([]string{"atmos.exe", "workflow", "run"}); got != "workflow-run" {
		t.Fatalf("slug = %q, want workflow-run", got)
	}
	if got := CommandSlug([]string{"atmosphere", "workflow"}); got != "atmosphere-workflow" {
		t.Fatalf("slug = %q, want atmosphere-workflow", got)
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
