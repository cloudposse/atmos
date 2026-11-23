package edition

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// releaseBoundaryFile records the latest published release and the journal entries it shipped.
const releaseBoundaryFile = "testdata/latest-release.yaml"

// regenerateReleaseEnv rewrites releaseBoundaryFile as "<version>@<YYYY-MM-DD>" from the current journal.
const regenerateReleaseEnv = "ATMOS_REGENERATE_EDITION_RELEASE"

const releaseBoundaryHeader = `# The latest published Atmos release and the edition journal entries it shipped.
#
# A journal entry that is not listed here has not been published yet, so it must be
# dated after this release: a project pinned to the release date (or any earlier date)
# while running this release must keep that release's behavior after upgrading.
# Listed entries are frozen: re-dating or removing one changes what existing pins resolve to.
#
# Update this file whenever a release is published (see "Release boundary" in
# docs/prd/editions.md). From the commit the release was built from, run:
#
#   ATMOS_REGENERATE_EDITION_RELEASE=v1.235.0@2026-11-25 go test ./pkg/edition -run TestJournalReleaseBoundary
`

type releaseBoundary struct {
	Version string          `yaml:"version"`
	Date    string          `yaml:"date"`
	Entries []releasedEntry `yaml:"entries"`
}

type releasedEntry struct {
	Date string `yaml:"date"`
	Key  string `yaml:"key"`
}

func (e releasedEntry) String() string { return e.Date + " " + e.Key }

func renderReleaseBoundary(t *testing.T, boundary *releaseBoundary) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString(releaseBoundaryHeader)
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	require.NoError(t, encoder.Encode(boundary))
	require.NoError(t, encoder.Close())
	return buf.Bytes()
}

func loadReleaseBoundary(t *testing.T) (*releaseBoundary, []byte) {
	t.Helper()
	data, err := os.ReadFile(releaseBoundaryFile)
	require.NoError(t, err)
	var boundary releaseBoundary
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	require.NoError(t, decoder.Decode(&boundary))
	return &boundary, data
}

// regenerateReleaseBoundary records every current journal entry as shipped in the given release.
func regenerateReleaseBoundary(t *testing.T, spec string) {
	t.Helper()
	version, date, ok := strings.Cut(spec, "@")
	require.True(t, ok && version != "" && date != "", "%s must be <version>@<YYYY-MM-DD>", regenerateReleaseEnv)
	boundary := &releaseBoundary{Version: version, Date: date}
	for _, entry := range Journal() {
		boundary.Entries = append(boundary.Entries, releasedEntry{Date: entry.Date, Key: entry.Key})
	}
	require.NoError(t, os.WriteFile(releaseBoundaryFile, renderReleaseBoundary(t, boundary), 0o600))
}

// releaseBoundaryViolations reports journal entries that would change behavior for a project
// pinned while running the latest published release.
func releaseBoundaryViolations(entries []Entry, boundary *releaseBoundary) []string {
	releaseDate, err := time.ParseInLocation(dateLayout, boundary.Date, time.UTC)
	if err != nil || boundary.Version == "" || len(boundary.Entries) == 0 {
		return []string{fmt.Sprintf("invalid release boundary %q@%q with %d entries", boundary.Version, boundary.Date, len(boundary.Entries))}
	}
	released := make(map[releasedEntry]bool, len(boundary.Entries))
	for _, entry := range boundary.Entries {
		released[entry] = true
	}
	var violations []string
	present := make(map[releasedEntry]bool, len(entries))
	for i := range entries {
		id := releasedEntry{Date: entries[i].Date, Key: entries[i].Key}
		present[id] = true
		if released[id] || entryDate(&entries[i]).After(releaseDate) {
			continue
		}
		violations = append(violations, fmt.Sprintf(
			"unreleased journal entry %s is dated on or before %s (%s); date it after the latest published release",
			id, boundary.Version, boundary.Date))
	}
	for _, entry := range boundary.Entries {
		if !present[entry] {
			violations = append(violations, fmt.Sprintf(
				"journal entry %s shipped in %s and must keep its date", entry, boundary.Version))
		}
	}
	return violations
}

// TestJournalReleaseBoundary prevents back-dated journal entries: an entry that a published
// release did not ship must be dated after that release, otherwise a project pinned while running
// it silently gets the new default or behavior after upgrading.
func TestJournalReleaseBoundary(t *testing.T) {
	if spec := os.Getenv(regenerateReleaseEnv); spec != "" {
		regenerateReleaseBoundary(t, spec)
	}
	boundary, data := loadReleaseBoundary(t)
	assert.Equal(t, string(renderReleaseBoundary(t, boundary)), string(data),
		"%s must be in canonical form; regenerate it with %s", releaseBoundaryFile, regenerateReleaseEnv)
	assert.Empty(t, releaseBoundaryViolations(Journal(), boundary))
}

// TestLatestReleasePinKeepsReleaseBehavior checks the user-facing effect: pinning to the latest
// release date rolls back every change that release did not ship.
func TestLatestReleasePinKeepsReleaseBehavior(t *testing.T) {
	boundary, _ := loadReleaseBoundary(t)
	released := make(map[releasedEntry]bool, len(boundary.Entries))
	for _, entry := range boundary.Entries {
		released[entry] = true
	}
	pin, err := DescribePin(boundary.Date, "config")
	require.NoError(t, err)
	rolledBack := make(map[string]bool, len(pin.Overrides))
	for _, change := range pin.Overrides {
		rolledBack[change.Key] = true
	}
	for _, entry := range Journal() {
		if released[releasedEntry{Date: entry.Date, Key: entry.Key}] {
			continue
		}
		assert.True(t, rolledBack[entry.Key], "pin %s must roll back unreleased %s", boundary.Date, entry.Key)
		if entry.Kind == KindBehavior {
			changed, err := BehaviorChanged(boundary.Date, entry.Key)
			require.NoError(t, err)
			assert.False(t, changed, "pin %s must keep the %s behavior of %s", boundary.Date, entry.Key, boundary.Version)
		}
	}
}

func TestReleaseBoundaryViolations(t *testing.T) {
	released := releasedEntry{Date: "2026-11-01", Key: "shipped"}
	boundary := &releaseBoundary{Version: "v1.0.0", Date: "2026-11-10", Entries: []releasedEntry{released}}
	shipped := Entry{Date: released.Date, Key: released.Key}

	tests := []struct {
		name     string
		entries  []Entry
		boundary *releaseBoundary
		want     []string
	}{
		{"released entries and later entries pass", []Entry{shipped, {Date: "2026-11-11", Key: "next"}}, boundary, nil},
		{"unreleased entry on the release date", []Entry{shipped, {Date: "2026-11-10", Key: "late"}}, boundary, []string{"2026-11-10 late"}},
		{"unreleased entry before the release date", []Entry{shipped, {Date: "2026-10-29", Key: "late"}}, boundary, []string{"2026-10-29 late"}},
		{"released entry re-dated", []Entry{{Date: "2026-11-12", Key: "shipped"}}, boundary, []string{"2026-11-01 shipped shipped in v1.0.0"}},
		{"released entry removed", nil, boundary, []string{"2026-11-01 shipped shipped in v1.0.0"}},
		{"invalid boundary date", []Entry{shipped}, &releaseBoundary{Version: "v1.0.0", Date: "2026-13-01", Entries: boundary.Entries}, []string{"invalid release boundary"}},
		{"empty boundary version", []Entry{shipped}, &releaseBoundary{Date: "2026-11-10", Entries: boundary.Entries}, []string{"invalid release boundary"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := releaseBoundaryViolations(tt.entries, tt.boundary)
			require.Len(t, got, len(tt.want), "%v", got)
			for i, want := range tt.want {
				assert.Contains(t, got[i], want)
			}
		})
	}
}
