package list

import (
	"bytes"
	"io"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/edition"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// editionsTestStreams is a minimal io.Streams implementation backed by in-memory buffers,
// so tests can capture data.Write output without touching the real stdout/stderr.
type editionsTestStreams struct {
	output *bytes.Buffer
	error  *bytes.Buffer
}

func (s editionsTestStreams) Input() io.Reader     { return bytes.NewReader(nil) }
func (s editionsTestStreams) Output() io.Writer    { return s.output }
func (s editionsTestStreams) Error() io.Writer     { return s.error }
func (s editionsTestStreams) RawOutput() io.Writer { return s.output }
func (s editionsTestStreams) RawError() io.Writer  { return s.error }

// captureEditionsOutput initializes the data-writer context with in-memory buffers for the
// duration of the test and returns the stdout buffer.
func captureEditionsOutput(t *testing.T) *bytes.Buffer {
	t.Helper()

	var stdout, stderr bytes.Buffer
	ioCtx, err := iolib.NewContext(iolib.WithStreams(editionsTestStreams{output: &stdout, error: &stderr}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)

	return &stdout
}

func TestParseOptionalAnchor(t *testing.T) {
	anchor, err := parseOptionalAnchor("")
	require.NoError(t, err)
	assert.Nil(t, anchor, "empty string means unbounded")

	anchor, err = parseOptionalAnchor("2026-01")
	require.NoError(t, err)
	require.NotNil(t, anchor)
	assert.Equal(t, "2026-01", anchor.Raw)

	_, err = parseOptionalAnchor("not-a-date")
	require.ErrorIs(t, err, edition.ErrInvalidEdition)
}

func TestEditionsToDataNewestFirst(t *testing.T) {
	entries := []edition.Entry{
		{Date: "2025-01-01", Key: "a", Kind: edition.KindValue, Old: true, New: false, Description: "first", Ref: "r1"},
		{Date: "2026-01-01", Key: "b", Kind: edition.KindValue, Old: "x", New: "y", Description: "second", Ref: "r2"},
	}

	data := editionsToData(entries)
	require.Len(t, data, 2)
	// Newest first.
	assert.Equal(t, "2026-01-01", data[0]["date"])
	assert.Equal(t, "b", data[0]["key"])
	assert.Equal(t, "x", data[0]["old"])
	assert.Equal(t, "y", data[0]["new"])
	assert.Equal(t, "2025-01-01", data[1]["date"])
	assert.Equal(t, "a", data[1]["key"])
	assert.Equal(t, "true", data[1]["old"])
	assert.Equal(t, "false", data[1]["new"])
}

func TestExecuteListEditionsWithOptionsInvalidAnchors(t *testing.T) {
	err := executeListEditionsWithOptions(&EditionsOptions{From: "13-2026"})
	require.ErrorIs(t, err, edition.ErrInvalidEdition)

	err = executeListEditionsWithOptions(&EditionsOptions{To: "2026-99"})
	require.ErrorIs(t, err, edition.ErrInvalidEdition)
}

func TestExecuteListEditionsWithOptionsFormats(t *testing.T) {
	initTestIO(t)
	for _, outputFormat := range []string{"", "json", "yaml", "csv", "tsv"} {
		t.Run(outputFormat, func(t *testing.T) {
			require.NoError(t, executeListEditionsWithOptions(&EditionsOptions{Format: outputFormat, From: "2025", To: "2026"}))
		})
	}
}

func TestExecuteListEditionsWithOptionsEmptyRange(t *testing.T) {
	initTestIO(t)
	require.NoError(t, executeListEditionsWithOptions(&EditionsOptions{Format: "json", From: "2099"}))
}

// TestExecuteListEditionsWithOptionsTTYFooter covers the table/TTY branch: with a TTY
// attached and no explicit --format (or --format=table), the command must render the
// table via RenderToString and append the range-summarizing footer built by
// buildEditionsFooter, writing the combined result to the data channel — instead of the
// non-TTY r.Render(rows) path exercised by TestExecuteListEditionsWithOptionsFormats.
func TestExecuteListEditionsWithOptionsTTYFooter(t *testing.T) {
	stdout := captureEditionsOutput(t)

	origForceTTY := viper.GetBool("force-tty")
	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", origForceTTY) })

	require.NoError(t, executeListEditionsWithOptions(&EditionsOptions{From: "2025", To: "2026"}))

	output := stdout.String()
	assert.Contains(t, output, "between editions 2025 and 2026", "TTY output must include the range footer")
}

func TestEditionsCommandFlags(t *testing.T) {
	for _, name := range []string{"format", "from", "to"} {
		assert.NotNil(t, editionsCmd.Flags().Lookup(name), "%s should be registered", name)
	}
	assert.NoError(t, editionsCmd.Args(editionsCmd, nil))
	assert.Error(t, editionsCmd.Args(editionsCmd, []string{"unexpected"}))
}

func TestExecuteListEditionsBindsCommandFlags(t *testing.T) {
	initTestIO(t)
	previous := map[string]string{}
	for _, name := range []string{"format", "from", "to"} {
		value, err := editionsCmd.Flags().GetString(name)
		require.NoError(t, err)
		previous[name] = value
		previousViper := viper.GetString(name)
		t.Cleanup(func() { viper.Set(name, previousViper) })
	}
	t.Cleanup(func() {
		for name, value := range previous {
			require.NoError(t, editionsCmd.Flags().Set(name, value))
		}
	})
	require.NoError(t, editionsCmd.Flags().Set("format", "json"))
	require.NoError(t, editionsCmd.Flags().Set("from", "2025"))
	require.NoError(t, editionsCmd.Flags().Set("to", "2026"))

	require.NoError(t, executeListEditions(editionsCmd, nil))
}

func TestEditionColumns(t *testing.T) {
	columns := editionColumns()
	require.Len(t, columns, 5)
	assert.Equal(t, "Date", columns[0].Name)
	assert.Equal(t, "Description", columns[4].Name)
}

func TestBuildEditionsFooter(t *testing.T) {
	tests := []struct {
		name string
		opts EditionsOptions
		want string
	}{
		{name: "no range", opts: EditionsOptions{}, want: "journaled"},
		{name: "range", opts: EditionsOptions{From: "2025", To: "2026"}, want: "between editions 2025 and 2026"},
		{name: "from only", opts: EditionsOptions{From: "2025"}, want: "since edition 2025"},
		{name: "to only", opts: EditionsOptions{To: "2026"}, want: "up to edition 2026"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, buildEditionsFooter(2, &tt.opts), tt.want)
		})
	}
	assert.Contains(t, buildEditionsFooter(1, &EditionsOptions{}), "1 default change journaled")
}
