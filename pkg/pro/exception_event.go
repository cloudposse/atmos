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

func buildExceptionEvent(err error, info *schema.ConfigAndStacksInfo, custom map[string]string, runtime map[string]string) *sentry.Event {
	event, details := crerrors.BuildSentryReport(err)
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
