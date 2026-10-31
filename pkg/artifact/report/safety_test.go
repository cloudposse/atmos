package report_test

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/report"
	atmosio "github.com/cloudposse/atmos/pkg/io"
)

// hostile is a name that tries to break out of every Markdown context.
const hostile = "evil|name`tick``double<script>alert(1)</script>*bold*_it_[l](http://x)\nnext line # head\r\n- item"

func TestMarkdownEscaping(t *testing.T) {
	outcome := published("a", "x/y:1")
	outcome.Result.Repository = hostile
	outcome.Result.Location = "/tmp/" + hostile
	outcome.Result.Requested = hostile
	outcome.Result.Diagnostics = []artifact.Diagnostic{{Severity: artifact.SeverityWarning, Code: "code|`", Message: hostile}}
	outcome.Result.Record.Metadata.Description = hostile
	outcome.Result.Record.Metadata.Labels = map[string]string{hostile: hostile}
	outcome.Result.Record.Inventory = []artifact.Entry{{Path: hostile, Type: artifact.EntryFile, Size: 1}}
	markdown := flushOne(t, enabledConfig(), entry(hostile, "a", 1, outcome, errors.New(hostile)))

	// Raw HTML survives only inside code spans and fenced blocks, where Markdown renders it as text.
	visible := regexp.MustCompile("(?s)```.*?```").ReplaceAllString(markdown, "")
	assert.NotContains(t, visible, "<script>", "raw HTML must not survive in running text")
	assert.NotContains(t, visible, "</script>")
	rows := 0
	for _, line := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(line, "| 1 |") {
			rows++
			// Every unescaped pipe is a column separator: the overview row has exactly nine columns.
			unescaped := strings.Count(strings.ReplaceAll(line, `\|`, ""), "|")
			assert.Equal(t, 10, unescaped, "a table row must not be split by user text: %s", line)
		}
	}
	require.Equal(t, 1, rows)
	assert.NotContains(t, markdown, "\r")
	// A newline in user text never starts a new Markdown block outside the fenced error block.
	outside := regexp.MustCompile("(?s)```text\n.*?\n```").ReplaceAllString(markdown, "")
	assert.NotContains(t, outside, "\n- item")
	assert.NotContains(t, outside, "\nnext line # head")
	assert.Contains(t, markdown, "&lt;script&gt;", "the step name is escaped as text")
}

func TestCodeSpanCannotBeClosedByTheValue(t *testing.T) {
	outcome := published("a", "x/y:1")
	outcome.Result.Location = "/tmp/a``b`c"
	markdown := flushOne(t, enabledConfig(), entry("", "a", 1, outcome, nil))
	// The value contains runs of one and two backticks, so the span uses three.
	assert.Contains(t, markdown, "```/tmp/a``b`c```")
}

func TestFencedErrorCannotBeClosedByTheMessage(t *testing.T) {
	markdown := flushOne(t, enabledConfig(), entry("", "a", 1, published("a", "x/y:1"), errors.New("bad\n```\n# injected heading")))
	assert.Contains(t, markdown, "````text\nbad\n```\n# injected heading\n````")
}

func TestSecretsAreMasked(t *testing.T) {
	const secret = "mask-me-report-value"
	atmosio.RegisterSecret(secret)
	outcome := published("a", "x/y:1")
	outcome.Result.Diagnostics = []artifact.Diagnostic{{Severity: artifact.SeverityInfo, Message: "token " + secret}}
	outcome.Result.Record.Metadata.Labels = map[string]string{"owner": secret}
	markdown := flushOne(t, enabledConfig(), entry("deploy-"+secret, "a", 1, outcome, fmt.Errorf("auth failed with %s", secret)))
	assert.NotContains(t, markdown, secret)
	assert.Contains(t, markdown, "auth failed with")
}

func TestSignedUrlsAndCredentialsAreRemoved(t *testing.T) {
	outcome := published("a", "x/y:1")
	outcome.Result.Location = "https://user:pass@bucket.s3.amazonaws.com/key?X-Amz-Signature=deadbeef&X-Amz-Credential=AKIA#frag"
	outcome.Result.Diagnostics = []artifact.Diagnostic{{Message: "retry https://host/path?token=abc123 later"}}
	markdown := flushOne(t, enabledConfig(), entry("", "a", 1, outcome, errors.New("GET https://signed.example/obj?X-Amz-Signature=cafe failed")))
	for _, leaked := range []string{"X-Amz-Signature", "deadbeef", "AKIA", "user:pass", "token=abc123", "cafe", "#frag"} {
		assert.NotContains(t, markdown, leaked)
	}
	assert.Contains(t, markdown, "https://bucket.s3.amazonaws.com/key")
	// A bare URL in running text is not left clickable.
	assert.NotContains(t, markdown, "retry https://")
}

func TestTruncationIsVisibleAndKeepsTheOverview(t *testing.T) {
	var entries []*report.Entry
	for index := 0; index < 60; index++ {
		outcome := published(fmt.Sprintf("op-%d", index), fmt.Sprintf("x/pkg-%02d:1", index))
		entries = append(entries, entry("publish", fmt.Sprintf("op-%d", index), 1, outcome, nil, index))
	}
	fake := &fakeReporter{}
	session := newSession(enabledConfig(), fake, report.WithMaxBytes(12_000))
	for _, item := range entries {
		session.Record(item)
	}
	require.NoError(t, session.Flush())
	require.Len(t, fake.summaries, 1)
	markdown := fake.summaries[0]

	assert.LessOrEqual(t, len(markdown), 12_000)
	assert.Contains(t, markdown, "left out to keep this summary within the size limit", "truncation is visible")
	assert.Contains(t, markdown, "60 operations: 60 succeeded.")
	for index := 0; index < 60; index++ {
		assert.Contains(t, markdown, fmt.Sprintf("`x/pkg-%02d:1`", index), "the overview lists every operation")
	}
	assert.True(t, utf8.ValidString(markdown))
}

func TestHardTruncationClosesOpenBlocks(t *testing.T) {
	outcome := published("a", "x/y:1")
	outcome.Result.Diagnostics = []artifact.Diagnostic{{Severity: artifact.SeverityInfo, Message: strings.Repeat("é", 400)}}
	fake := &fakeReporter{}
	session := newSession(enabledConfig(), fake, report.WithMaxBytes(450))
	session.Record(entry("publish", "a", 1, outcome, errors.New(strings.Repeat("long failure ", 400))))
	require.NoError(t, session.Flush())
	markdown := fake.summaries[0]
	assert.LessOrEqual(t, len(markdown), 450)
	assert.True(t, utf8.ValidString(markdown))
	assert.Contains(t, markdown, "cut to fit the size limit")
	assert.Zero(t, strings.Count(markdown, "```")%2, "code fences stay balanced")
	assert.Equal(t, strings.Count(markdown, "<details>"), strings.Count(markdown, "</details>"))
}

func TestCompactionDropsDetailsBeforeSections(t *testing.T) {
	outcome := published("a", "x/y:1")
	files := make([]artifact.Entry, 500)
	for index := range files {
		files[index] = artifact.Entry{Path: fmt.Sprintf("dir/file-%03d.tf", index), Type: artifact.EntryFile, Size: 10}
	}
	outcome.Result.Record.Inventory = files
	full := flushOne(t, enabledConfig(), entry("publish", "a", 1, outcome, nil))
	fake := &fakeReporter{}
	session := newSession(enabledConfig(), fake, report.WithMaxBytes(len(full)-200))
	session.Record(entry("publish", "a", 1, outcome, nil))
	require.NoError(t, session.Flush())
	compact := fake.summaries[0]
	assert.Contains(t, compact, "Inventory, metadata, and long diagnostic lists were left out")
	assert.NotContains(t, compact, "dir/file-000.tf")
	assert.Contains(t, compact, "### ✅ 1. Publish `x/y:1`", "the section itself stays")
}

func TestListAndSearchCollectionsRender(t *testing.T) {
	collection := &invoke.Outcome{Collection: &invoke.Collection{Artifacts: []invoke.Listed{{Ref: "a/b:1"}}}}
	markdown := flushOne(t, enabledConfig(), &report.Entry{Action: artifact.ActionSearch, Attempt: 1, Outcome: collection})
	assert.Contains(t, markdown, "Searched 1 artifact.")
}

func TestEntriesWithoutResultsStillRender(t *testing.T) {
	markdown := flushOne(t, enabledConfig(), &report.Entry{Action: artifact.ActionPull, Attempt: 1, Err: errors.New("boom")})
	assert.Contains(t, markdown, "❌ Failed")
	assert.Contains(t, markdown, "boom")
}
