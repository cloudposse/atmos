package permission

import (
	"context"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Option configures NewFromConfig.
type Option func(*options)

type options struct {
	prompter Prompter
	before   func()
	after    func()
}

// WithPrompter replaces the default CLI prompter. The permission cache is not
// consulted by a custom prompter unless it does so itself.
func WithPrompter(prompter Prompter) Option {
	return func(o *options) {
		o.prompter = prompter
	}
}

// WithPromptHooks registers callbacks that bracket every real prompt. The before
// callback runs immediately before a prompt can appear and the after callback runs when it returns (always,
// via defer). They are not invoked when the checker decides without prompting
// (yolo, allow, blocked, allowed list) or when a cached decision answers.
// Either callback may be nil.
func WithPromptHooks(before, after func()) Option {
	return func(o *options) {
		o.before = before
		o.after = after
	}
}

// NewFromConfig builds a permission Checker from the AI tool settings: it resolves
// the mode (see ResolveMode), loads the persistent decision cache (warning and
// continuing without it on failure), and wires a CLI prompter.
func NewFromConfig(atmosConfig *schema.AtmosConfiguration, opts ...Option) (*Checker, error) {
	defer perf.Track(atmosConfig, "permission.NewFromConfig")()

	if atmosConfig == nil {
		atmosConfig = &schema.AtmosConfiguration{}
	}

	var o options
	for _, opt := range opts {
		opt(&o)
	}

	settings := &atmosConfig.AI.Tools
	mode, yolo, err := ResolveMode(settings)
	if err != nil {
		return nil, err
	}

	prompter := o.prompter
	if prompter == nil {
		prompter = newDefaultPrompter(atmosConfig)
	}
	if o.before != nil || o.after != nil {
		prompter = &hookedPrompter{inner: prompter, before: o.before, after: o.after}
	}

	config := &Config{
		Mode:       mode,
		Allowed:    settings.Allowed,
		Restricted: settings.Restricted,
		Blocked:    settings.Blocked,
		YOLOMode:   yolo,
	}

	return NewChecker(config, prompter), nil
}

// newDefaultPrompter returns a CLI prompter backed by the persistent cache when available.
func newDefaultPrompter(atmosConfig *schema.AtmosConfiguration) Prompter {
	cache, err := NewPermissionCache(atmosConfig.BasePath)
	if err != nil {
		log.Warnf("Failed to initialize permission cache: %v", err)
		// Continue without cache - will prompt every time.
		return NewCLIPrompter()
	}
	return NewCLIPrompterWithCache(cache)
}

// cachedDecisionChecker is implemented by prompters that can answer from a stored
// decision without showing a prompt.
type cachedDecisionChecker interface {
	HasCachedDecision(tool Tool) bool
}

// hookedPrompter decorates a Prompter with before/after callbacks.
type hookedPrompter struct {
	inner  Prompter
	before func()
	after  func()
}

// Prompt invokes the hooks around the wrapped prompter.
// The hooks are skipped when the wrapped prompter would answer from its cache.
func (h *hookedPrompter) Prompt(ctx context.Context, tool Tool, params map[string]interface{}) (bool, error) {
	if checker, ok := h.inner.(cachedDecisionChecker); ok && checker.HasCachedDecision(tool) {
		return h.inner.Prompt(ctx, tool, params)
	}
	if h.before != nil {
		h.before()
	}
	if h.after != nil {
		defer h.after()
	}
	return h.inner.Prompt(ctx, tool, params)
}
