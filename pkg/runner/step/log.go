package step

import (
	"context"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// LogHandler logs a message using the atmos logger.
type LogHandler struct {
	BaseHandler
}

func init() {
	Register(&LogHandler{
		BaseHandler: NewBaseHandler("log", CategoryOutput, false),
	})
}

// Validate checks that the step has required fields.
func (h *LogHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.LogHandler.Validate")()

	return h.ValidateRequired(step, "content", step.Content)
}

// Execute logs the message at the specified level.
func (h *LogHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.LogHandler.Execute")()

	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Get log level from step config (default to info).
	level := getLogLevel(step.Level)

	// Build structured fields from step.Fields map.
	keyvals := h.buildKeyvals(step, vars)

	// Log at the appropriate level with structured fields.
	switch level {
	case log.TraceLevel:
		log.Trace(content, keyvals...)
	case log.DebugLevel:
		log.Debug(content, keyvals...)
	case log.WarnLevel:
		log.Warn(content, keyvals...)
	case log.ErrorLevel:
		log.Error(content, keyvals...)
	default:
		log.Info(content, keyvals...)
	}

	return NewStepResult(content), nil
}

// buildKeyvals converts step.Fields to a slice of key-value pairs for structured logging.
func (h *LogHandler) buildKeyvals(step *schema.WorkflowStep, vars *Variables) []interface{} {
	if len(step.Fields) == 0 {
		return nil
	}

	// Cap at reasonable maximum to avoid overflow on 32-bit systems.
	const maxCapacity = 1 << 20 // 1M capacity is more than reasonable.
	fieldsLen := len(step.Fields)
	// If doubling fieldsLen would exceed maxCapacity (or overflow), clamp to maxCapacity.
	var capacity int
	if fieldsLen >= maxCapacity/2 {
		capacity = maxCapacity
	} else {
		capacity = fieldsLen * 2
	}
	keyvals := make([]interface{}, 0, capacity)
	for key, value := range step.Fields {
		// Resolve template variables in field values.
		resolvedValue, err := vars.Resolve(value)
		if err != nil {
			// On error, use the original value.
			resolvedValue = value
		}
		keyvals = append(keyvals, key, resolvedValue)
	}
	return keyvals
}

// getLogLevel parses a log level string.
func getLogLevel(level string) log.Level {
	switch strings.ToLower(level) {
	case "trace":
		return log.TraceLevel
	case "debug":
		return log.DebugLevel
	case "warn", "warning":
		return log.WarnLevel
	case "error":
		return log.ErrorLevel
	case "info", "":
		return log.InfoLevel
	default:
		return log.InfoLevel
	}
}
