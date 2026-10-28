package versionnotice

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/duration"
)

func annotationTitle(current string, behind uint64, buildDate string, now time.Time) string {
	title := fmt.Sprintf("Atmos %s is %d releases behind", strings.TrimPrefix(current, "v"), behind)
	built, err := time.Parse(time.RFC3339, buildDate)
	if err != nil || built.IsZero() || !built.Before(now) {
		return title
	}
	return title + " (about " + duration.FormatApproximate(now.Sub(built)) + " old)"
}
