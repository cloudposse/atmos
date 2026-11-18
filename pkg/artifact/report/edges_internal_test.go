package report

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
)

func TestComparePositions(t *testing.T) {
	tests := []struct {
		name        string
		left, right []int
		want        int
	}{
		{"both unscheduled", nil, nil, 0},
		{"unscheduled sorts last", nil, []int{0}, 1},
		{"scheduled sorts first", []int{0}, nil, -1},
		{"lower index first", []int{1, 5}, []int{2}, -1},
		{"higher index last", []int{3}, []int{2, 9}, 1},
		{"prefix is smaller", []int{1}, []int{1, 0}, -1},
		{"longer is greater", []int{1, 0, 0}, []int{1}, 2},
		{"equal", []int{1, 2}, []int{1, 2}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, comparePositions(tt.left, tt.right))
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name         string
		action       artifact.Action
		outcome      artifact.Outcome
		verification *artifact.VerificationResult
		err          error
		wantWord     string
		wantKind     string
	}{
		{"canceled", artifact.ActionPull, "", nil, context.Canceled, "Canceled", kindCanceled},
		{"deadline", artifact.ActionPull, "", nil, fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "Timed out", kindFailed},
		{"other error", artifact.ActionPull, "", nil, assert.AnError, "Failed", kindFailed},
		{"failed outcome without error", artifact.ActionPublish, artifact.OutcomeFailed, nil, nil, "Failed", kindFailed},
		{"verified", artifact.ActionVerify, "", &artifact.VerificationResult{Status: artifact.VerificationVerified}, nil, "Verified", kindSuccess},
		{"unchecked", artifact.ActionVerify, "", &artifact.VerificationResult{Status: artifact.VerificationUnchecked}, nil, "Unchecked", kindUnchecked},
		{"verification failed", artifact.ActionVerify, "", &artifact.VerificationResult{Status: artifact.VerificationFailed}, nil, "Verification failed", kindFailed},
		{"error wins over a verified partial result", artifact.ActionVerify, "", &artifact.VerificationResult{Status: artifact.VerificationVerified}, assert.AnError, "Failed", kindFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			word, kind := classify(tt.action, tt.outcome, tt.verification, tt.err)

			assert.Equal(t, tt.wantWord, word)
			assert.Equal(t, tt.wantKind, kind)
		})
	}
}

func TestActionLabelAndKnownActions(t *testing.T) {
	assert.Equal(t, "Operation", actionLabel(""))
	assert.Equal(t, "Publish", actionLabel(artifact.ActionPublish))
	assert.True(t, isKnownAction(string(artifact.ActionVerify)))
	assert.False(t, isKnownAction("teleport"))
}

func TestDefaultTemplatesOpenRejectsUnknownNames(t *testing.T) {
	known := templatesPrefix + string(artifact.ActionPublish) + templateSuffix
	file, err := defaultTemplates{}.Open(known)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	for _, name := range []string{
		string(artifact.ActionPublish) + templateSuffix,  // Missing the directory prefix.
		templatesPrefix + string(artifact.ActionPublish), // Missing the file suffix.
		templatesPrefix + "teleport" + templateSuffix,    // No section for the action.
		templatesPrefix + "../" + frameCommand + "x.md",  // Not a template name.
	} {
		t.Run(name, func(t *testing.T) {
			_, err := defaultTemplates{}.Open(name)

			require.ErrorIs(t, err, fs.ErrNotExist)
		})
	}
}

func TestShortDigestAndSizes(t *testing.T) {
	long := "sha256:" + strings.Repeat("ab", 32)
	assert.Equal(t, "sha256:"+strings.Repeat("ab", shortDigestLength/2)+ellipsis, shortDigest(digest.Digest(long)))
	assert.Equal(t, "sha256:abc", shortDigest(digest.Digest("sha256:abc")), "a short digest is kept whole")
	assert.Equal(t, "no-algorithm", shortDigest(digest.Digest("no-algorithm")), "text without an algorithm is kept whole")

	assert.Empty(t, humanSize(0))
	assert.Empty(t, exactSize(-1))
	assert.Equal(t, "1.0 KiB (1,024 bytes)", exactSize(1024))
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{0, ""},
		{-time.Second, ""},
		{time.Microsecond, "<1 ms"},
		{250 * time.Millisecond, "250 ms"},
		{1500 * time.Millisecond, "1.5 s"},
		{90*time.Second + 400*time.Millisecond, "1m30s"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, formatDuration(tt.duration))
		})
	}
}

func TestShortDigestTextAndSubjects(t *testing.T) {
	assert.Equal(t, "sha256:short", shortDigestText("sha256:short"))
	assert.Equal(t, "sha256:0123456789ab"+ellipsis, shortDigestText("sha256:0123456789abcdef0123"))

	empty := &Signature{}
	applySubject(empty, nil)
	applySubject(empty, &artifact.SignatureSubject{})
	assert.Equal(t, &Signature{}, empty, "a missing subject leaves the signature untouched")

	assert.Nil(t, claimsOf(nil))
	assert.Nil(t, evidenceOf(nil))
}

func TestLabelsDiagnosticsAndInventoryAreCapped(t *testing.T) {
	labels := make(map[string]string)
	for index := range maxLabels + 5 {
		labels[fmt.Sprintf("key-%02d", index)] = "v"
	}
	listed := labelsOf(labels)
	require.Len(t, listed, maxLabels)
	assert.Equal(t, code("key-00"), listed[0].Key, "labels are sorted by key")
	assert.Equal(t, code(fmt.Sprintf("key-%02d", maxLabels-1)), listed[maxLabels-1].Key)
	assert.Nil(t, labelsOf(nil))

	diagnostics := make([]artifact.Diagnostic, maxDiagnostics+3)
	for index := range diagnostics {
		diagnostics[index] = artifact.Diagnostic{Severity: "warning", Message: fmt.Sprintf("message %d", index)}
	}
	shown, more := diagnosticsOf(diagnostics)
	require.Len(t, shown, maxDiagnostics)
	assert.Equal(t, 3, more)
	assert.Equal(t, plain("message 0"), shown[0].Message)
	assert.Equal(t, plain(fmt.Sprintf("message %d", maxDiagnostics-1)), shown[maxDiagnostics-1].Message)

	assert.Nil(t, inventoryOf(nil))
}

func TestTruncateMarkdownClosesWhatTheCutOpened(t *testing.T) {
	body := "intro\n" + codeFence + "\ncode that runs on " + strings.Repeat("x", 400) + "\n" + codeFence + "\n" +
		detailsOpen + "\n" + strings.Repeat("y", 400) + "\n" + detailsClose + "\n" + detailsOpen + "\n" + strings.Repeat("z", 400)

	// Cut inside the first fence, with a collapsible section still open at the cut.
	cutInFence := truncateMarkdown(body, len(truncationNote)+len(codeFence)*2+closeAllowance*(len(detailsClose)+1)+40)
	assert.Equal(t, 0, strings.Count(cutInFence, codeFence)%2, "the fence is balanced")
	assert.True(t, strings.HasSuffix(cutInFence, truncationNote))

	// Cut inside the second collapsible section.
	reserved := len(truncationNote) + len(codeFence) + 1 + closeAllowance*(len(detailsClose)+1)
	cutInDetails := truncateMarkdown(body, reserved+strings.Index(body, strings.Repeat("z", 10))+5)
	assert.Equal(t, strings.Count(cutInDetails, detailsOpen), strings.Count(cutInDetails, detailsClose), "every opened section is closed")
	assert.True(t, strings.HasSuffix(cutInDetails, truncationNote))

	// A limit below the note keeps the output within the limit.
	assert.LessOrEqual(t, len(truncateMarkdown(body, 10)), 10)
}

func TestCutAtRuneNeverSplitsARune(t *testing.T) {
	text := "abé世z" // 1+1+2+3+1 bytes.
	assert.Equal(t, text, cutAtRune(text, len(text)))
	assert.Equal(t, "ab", cutAtRune(text, 3), "a cut inside the two-byte rune backs up to its start")
	assert.Equal(t, "abé", cutAtRune(text, 5), "a cut inside the three-byte rune backs up to its start")
	assert.Equal(t, "abé", cutAtRune(text, 4))
}

func TestVerificationSignatureListsEvidenceAndPolicy(t *testing.T) {
	result := &invoke.Result{Verification: &artifact.VerificationResult{
		Status:            artifact.VerificationVerified,
		PolicyFingerprint: "sha256:0123456789abcdef0123",
		EvidenceDigests:   []digest.Digest{"sha256:aaa", "sha256:bbb"},
	}}

	signature := verificationSignature(result)

	assert.Equal(t, "verified", signature.Status)
	assert.Equal(t, code("sha256:0123456789ab"+ellipsis), signature.Policy)
	assert.Equal(t, []Evidence{{Digest: code("sha256:aaa")}, {Digest: code("sha256:bbb")}}, signature.Evidence)
}
