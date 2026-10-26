package github

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ReportSARIF implements provider.SARIFReporter by uploading the raw SARIF to
// GitHub Code Scanning (the Security tab). Requires GitHub Advanced Security on
// private repos and a token with the `security_events` write scope. The report
// Category is stamped into each run's automationDetails.id so multiple uploads
// for the same commit are tracked as distinct analyses.
func (p *Provider) ReportSARIF(ctx context.Context, report provider.SARIFReport) error {
	defer perf.Track(nil, "github.Provider.ReportSARIF")()

	if err := p.ensureClient(); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCISARIFUploadFailed, err)
	}

	cictx, err := p.Context()
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrCISARIFUploadFailed, err)
	}
	if cictx.RepoOwner == "" || cictx.RepoName == "" || cictx.SHA == "" {
		return fmt.Errorf("%w: missing repository owner/name or commit SHA", errUtils.ErrCISARIFUploadFailed)
	}

	body := withCategory(report.Body, report.Category)
	encoded, err := gzipBase64(body)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrCISARIFUploadFailed, err)
	}

	analysis := &github.SarifAnalysis{
		CommitSHA: github.String(cictx.SHA),
		Ref:       github.String(cictx.Ref),
		Sarif:     github.String(encoded),
	}

	id, _, err := p.client.GitHub().CodeScanning.UploadSarif(ctx, cictx.RepoOwner, cictx.RepoName, analysis)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrCISARIFUploadFailed, err)
	}
	log.Debug("Uploaded SARIF to GitHub Code Scanning", "category", report.Category, "id", id.GetID())
	return nil
}

// gzipBase64 gzip-compresses then base64-encodes the SARIF, as the Code
// Scanning upload endpoint requires.
func gzipBase64(data []byte) (string, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// withCategory stamps category into each SARIF run's automationDetails.id —
// the field GitHub uses to keep separate analyses for the same commit from
// overwriting each other. Code Scanning has no separate "category" upload
// parameter; it is carried inside the SARIF. GitHub interprets id as
// category/run-id, so a plain category is written with a trailing slash.
// Best-effort: if category is empty or the document can't be parsed, the
// original bytes are returned unchanged so the upload still proceeds (GitHub
// will assign a default identity).
func withCategory(sarif []byte, category string) []byte {
	if category == "" {
		return sarif
	}
	var doc map[string]any
	if err := json.Unmarshal(sarif, &doc); err != nil {
		log.Debug("SARIF category injection skipped: document not parseable", "error", err)
		return sarif
	}
	runs, ok := doc["runs"].([]any)
	if !ok {
		return sarif
	}
	for _, r := range runs {
		run, ok := r.(map[string]any)
		if !ok {
			continue
		}
		details, ok := run["automationDetails"].(map[string]any)
		if !ok {
			details = map[string]any{}
		}
		details["id"] = categoryID(category)
		run["automationDetails"] = details
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return sarif
	}
	return out
}

func categoryID(category string) string {
	if category == "" || strings.Contains(category, "/") {
		return category
	}
	return category + "/"
}
