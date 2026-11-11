package pro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	crerrors "github.com/cockroachdb/errors"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/version"
)

const (
	// The event message is cut at this many bytes, so one verbose error cannot push the whole
	// event over the envelope limit and drop it. The text is written to both the message and the
	// top exception value, so it adds at most twice this size (about 16 KiB of the 1 MiB limit).
	maxExceptionMessageBytes = 8 << 10
	// This marker is appended to error text that was cut at maxExceptionMessageBytes.
	truncationMarker = "… [truncated]"
)

// buildExceptionEvent builds the Sentry event for a failure: the cockroachdb/errors report, hints as
// breadcrumbs, component metadata, and runtime identity, with a readable message in place of the
// redacted one.
func buildExceptionEvent(err error, info *schema.ConfigAndStacksInfo, custom map[string]string, runtime map[string]string) *sentry.Event {
	if info == nil {
		info = &schema.ConfigAndStacksInfo{}
	}
	event, details := crerrors.BuildSentryReport(err)
	applyExceptionSummary(event, exceptionSummary(err, info))
	if event.Tags == nil {
		event.Tags = make(map[string]string)
	}
	if event.Contexts == nil {
		event.Contexts = make(map[string]sentry.Context)
	}
	if event.EventID == "" {
		event.EventID = sentry.EventID(strings.ReplaceAll(uuid.NewString(), "-", ""))
	}
	for key, value := range details {
		if context, ok := value.(map[string]any); ok {
			event.Contexts[key] = context
		}
	}
	for _, hint := range crerrors.GetAllHints(err) {
		event.Breadcrumbs = append(event.Breadcrumbs, &sentry.Breadcrumb{Type: "info", Category: "hint", Message: hint, Level: sentry.LevelInfo})
	}
	enrichExceptionMetadata(event, info)
	for key, value := range custom {
		event.Tags[key] = value
	}
	enrichExceptionIdentity(event, info, runtime)
	event.Tags["atmos.exit_code"] = fmt.Sprint(errUtils.GetExitCode(err))
	event.Release = "atmos@" + version.Version
	return event
}

// exceptionSummary is the readable text of the failure: the error chain's own message, prefixed
// with the command that failed when it is known. The cockroachdb/errors library redacts every
// message it cannot prove safe, which turns a bare exit-code failure into "×". Atmos masks the
// whole event before it leaves the process (see maskedExceptionEvent), so the original text is
// sent instead. The text is masked here as well, before it is truncated: a secret split by the
// cut could no longer be matched, and its leading part would be sent.
func exceptionSummary(err error, info *schema.ConfigAndStacksInfo) string {
	summary := err.Error()
	if info.ComponentType != "" && info.SubCommand != "" {
		summary = info.ComponentType + " " + info.SubCommand + ": " + summary
	}
	return truncateExceptionText(ioLayer.MaskString(summary))
}

// applyExceptionSummary replaces the redacted message and the top exception value with summary.
// The error-type breakdown that cockroachdb/errors appends to the message is kept, because it
// names the error types without exposing any message text.
func applyExceptionSummary(event *sentry.Event, summary string) {
	const typesMarker = "\nError types:"
	tail := ""
	if i := strings.Index(event.Message, typesMarker); i >= 0 {
		tail = event.Message[i:]
	}
	event.Message = summary + tail
	if len(event.Exception) > 0 {
		event.Exception[len(event.Exception)-1].Value = summary
	}
}

// truncateExceptionText cuts text to maxExceptionMessageBytes on a rune boundary.
func truncateExceptionText(text string) string {
	if len(text) <= maxExceptionMessageBytes {
		return text
	}
	cut := maxExceptionMessageBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + truncationMarker
}

func enrichExceptionIdentity(event *sentry.Event, info *schema.ConfigAndStacksInfo, runtime map[string]string) {
	execution := make(sentry.Context)
	for key, value := range runtime {
		if value != "" {
			execution[key] = value
			event.Tags["atmos."+key] = value
		}
	}
	component := info.ComponentFromArg
	if component == "" {
		component = info.Component
	}
	stack := info.Stack
	if stack == "" {
		stack = info.StackFromArg
	}
	for key, value := range map[string]string{"component": component, "stack": stack, "component_type": info.ComponentType} {
		if value != "" {
			event.Tags["atmos."+key] = value
		}
	}
	event.Contexts["atmos_execution"] = execution
}

func enrichExceptionMetadata(event *sentry.Event, info *schema.ConfigAndStacksInfo) {
	metadataTags := tags.ToStringSlice(info.ComponentMetadataSection["tags"])
	if typed, ok := info.ComponentMetadataSection["tags"].([]string); ok {
		metadataTags = append([]string(nil), typed...)
	}
	metadataLabels := tags.ToStringMap(info.ComponentMetadataSection["labels"])
	if typed, ok := info.ComponentMetadataSection["labels"].(map[string]string); ok {
		for key, value := range typed {
			metadataLabels[key] = value
		}
	}
	for _, key := range metadataTags {
		event.Tags[key] = "true"
	}
	for key, value := range metadataLabels {
		event.Tags[key] = value
	}
	event.Contexts["atmos_metadata"] = sentry.Context{"tags": metadataTags, "labels": metadataLabels}
}

func maskedExceptionEvent(event *sentry.Event) (*sentry.Event, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, errUtils.ErrFailedToMarshalPayload
	}
	if len(data) > maxExceptionEnvelopeBytes {
		return nil, errUtils.ErrProExceptionEnvelopeTooLarge
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, errUtils.ErrFailedToMarshalPayload
	}
	data, err = json.Marshal(maskExceptionValue(payload))
	if err != nil {
		return nil, errUtils.ErrFailedToMarshalPayload
	}
	var result sentry.Event
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errUtils.ErrFailedToMarshalPayload
	}
	// An identifier is not user content; it must stay equal to the envelope ID.
	result.EventID = event.EventID
	for key, value := range result.Tags {
		if !validExceptionTag(key, value) {
			delete(result.Tags, key)
		}
	}
	return &result, nil
}

func maskExceptionValue(value any) any {
	switch v := value.(type) {
	case string:
		return ioLayer.MaskString(v)
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			result[ioLayer.MaskString(key)] = maskExceptionValue(item)
		}
		return result
	case []any:
		for i, item := range v {
			v[i] = maskExceptionValue(item)
		}
	}
	return value
}

const (
	maxExceptionTagKeyBytes   = 32
	maxExceptionTagValueRunes = 200
)

var exceptionTagKey = regexp.MustCompile(`^[a-zA-Z0-9_.:-]+$`)

func validExceptionTag(key, value string) bool {
	return len(key) > 0 && len(key) <= maxExceptionTagKeyBytes && exceptionTagKey.MatchString(key) &&
		utf8.RuneCountInString(value) <= maxExceptionTagValueRunes && !strings.ContainsAny(value, "\r\n")
}
