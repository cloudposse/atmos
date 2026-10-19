package toolchain

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestBatchRendererSharedProgressAndResults(t *testing.T) {
	tool := toolInfo{owner: "owner", repo: "terraform", version: "1.0"}
	other := toolInfo{owner: "owner", repo: "tofu", version: "2.0"}
	r := newBatchRenderer(2)
	captureUITestOutput(t, func() { r.start(tool); r.start(other) })
	output := captureUITestOutput(t, func() {
		r.updateProgress(tool, downloadProgress{downloaded: 1024, total: 2048})
		r.updateProgress(other, downloadProgress{downloaded: 50, total: -1})
		r.updateProgress(toolInfo{repo: "missing"}, downloadProgress{downloaded: 999, total: 999})
		r.tick()
	})
	text := ansi.Strip(output)
	assert.Contains(t, text, "Downloading owner/terraform@1.0")
	assert.Contains(t, text, "1.0 KB/2.0 KB")
	assert.Contains(t, text, "Downloading owner/tofu@2.0")
	assert.Contains(t, text, "50 B")
	assert.Contains(t, text, "0/2 complete, 2 running")
	assert.NotContains(t, text, "missing")
	output = captureUITestOutput(t, func() { r.updateProgress(tool, downloadProgress{downloaded: 2048, total: 2048}) })
	assert.Contains(t, ansi.Strip(output), "Verifying owner/terraform@1.0")
	output = captureUITestOutput(t, func() { r.complete(other, resultSkipped, nil) })
	text = ansi.Strip(output)
	assert.Contains(t, text, "Skipped")
	assert.Contains(t, text, "owner/tofu@2.0")
	assert.Contains(t, text, "already installed")
	assert.Contains(t, text, "Verifying owner/terraform@1.0")
	assert.NotContains(t, text, "Downloading owner/tofu")
	assert.Contains(t, text, "1/2 complete, 1 running")
	output = captureUITestOutput(t, func() { r.complete(tool, resultFailed, errors.New("checksum mismatch")); r.clear() })
	text = ansi.Strip(output)
	assert.Contains(t, text, "Install failed")
	assert.Contains(t, text, "checksum mismatch")
	assert.NotContains(t, text, "running")
	assert.Empty(t, captureUITestOutput(t, r.clear))
}

func TestBatchRendererDuplicateToolsKeepIndependentRows(t *testing.T) {
	tool := toolInfo{owner: "owner", repo: "tool", version: "1"}
	r := &batchRenderer{total: 3}
	captureUITestOutput(t, func() { r.start(tool); r.start(tool) })
	output := captureUITestOutput(t, func() { r.complete(tool, resultInstalled, nil) })
	assert.Contains(t, ansi.Strip(output), "1/3 complete, 1 running")
	assert.Contains(t, ansi.Strip(output), "Downloading owner/tool@1")
	captureUITestOutput(t, func() { r.start(tool) })
	output = captureUITestOutput(t, func() { r.tick() })
	assert.Equal(t, 2, strings.Count(ansi.Strip(output), "Downloading owner/tool@1"))
	assert.Contains(t, ansi.Strip(output), "1/3 complete, 2 running")
}

func TestBatchRendererCompletionWithoutStartRetainsResult(t *testing.T) {
	r := &batchRenderer{total: 1}
	output := captureUITestOutput(t, func() { r.complete(toolInfo{owner: "owner", repo: "tool", version: "1"}, resultInstalled, nil) })
	assert.Contains(t, ansi.Strip(output), "Installed")
	assert.Contains(t, ansi.Strip(output), "owner/tool@1")
}

func TestLiveBatchRendererDuplicateLabelsAndLaterStarts(t *testing.T) {
	r := newLiveBatchRenderer(3)
	captureUITestOutput(t, func() { r.start("same"); r.start("same") })
	output := captureUITestOutput(t, func() { r.complete("same", "first result", batchLineInfo); r.start("third") })
	text := ansi.Strip(output)
	assert.Contains(t, text, "first result")
	assert.Contains(t, text, "same")
	assert.Contains(t, text, "third")
	assert.Contains(t, text, "1/3 complete, 2 running")
	output = captureUITestOutput(t, func() { r.complete("third", "third result", batchLineInfo) })
	text = ansi.Strip(output)
	assert.Contains(t, text, "third result")
	assert.Contains(t, text, "same")
	assert.Contains(t, text, "2/3 complete, 1 running")
	output = captureUITestOutput(t, func() { r.complete("same", "second result", batchLineSuccess) })
	assert.Contains(t, ansi.Strip(output), "second result")
	assert.NotContains(t, ansi.Strip(output), "running")
}

func TestLiveBatchRendererCompletionWithoutStartRetainsResult(t *testing.T) {
	r := newLiveBatchRenderer(1)
	output := captureUITestOutput(t, func() { r.complete("missing", "finished without start", batchLineSuccess) })
	assert.Contains(t, ansi.Strip(output), "finished without start")
}
