package internal

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"mvdan.cc/sh/v3/shell"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const maxAliasExpansionDepth = 32

const dash = "-"

var (
	errAliasTargetEmpty        = errors.New("alias target is empty")
	errAliasCycleDetected      = errors.New("command alias cycle detected")
	errAliasExpansionExhausted = errors.New("command alias expansion exceeded max depth")
)

type aliasRule struct {
	key    []string
	value  []string
	rawKey string
}

// AliasExpander rewrites argv prefixes using configured command aliases.
type AliasExpander struct {
	rules []aliasRule
}

var configuredCommandAliases = struct {
	mu       sync.RWMutex
	expander *AliasExpander
}{}

// ConfigureCommandAliases stores the command aliases used by ExpandCommandAliases.
func ConfigureCommandAliases(aliases schema.CommandAliases) error {
	defer perf.Track(nil, "internal.ConfigureCommandAliases")()

	expander, err := NewAliasExpander(aliases)
	if err != nil {
		return err
	}

	configuredCommandAliases.mu.Lock()
	defer configuredCommandAliases.mu.Unlock()
	configuredCommandAliases.expander = expander
	return nil
}

// ExpandCommandAliases expands args using the configured command aliases.
func ExpandCommandAliases(args []string) ([]string, error) {
	defer perf.Track(nil, "internal.ExpandCommandAliases")()

	configuredCommandAliases.mu.RLock()
	expander := configuredCommandAliases.expander
	configuredCommandAliases.mu.RUnlock()

	if expander == nil {
		return args, nil
	}
	return expander.Expand(args)
}

// NewAliasExpander parses command aliases into longest-prefix match rules.
func NewAliasExpander(aliases schema.CommandAliases) (*AliasExpander, error) {
	defer perf.Track(nil, "internal.NewAliasExpander")()

	if len(aliases) == 0 {
		return &AliasExpander{}, nil
	}

	rules := make([]aliasRule, 0, len(aliases))
	for rawKey, rawValue := range aliases {
		key, err := parseAliasFields(rawKey)
		if err != nil {
			return nil, fmt.Errorf("invalid alias %q: %w", rawKey, err)
		}
		if len(key) == 0 {
			continue
		}

		value, err := parseAliasFields(rawValue)
		if err != nil {
			return nil, fmt.Errorf("invalid alias %q target %q: %w", rawKey, rawValue, err)
		}
		if len(value) == 0 {
			return nil, fmt.Errorf("invalid alias %q: %w", rawKey, errAliasTargetEmpty)
		}

		rules = append(rules, aliasRule{
			key:    key,
			value:  value,
			rawKey: strings.Join(key, " "),
		})
	}

	sort.SliceStable(rules, func(i, j int) bool {
		if len(rules[i].key) == len(rules[j].key) {
			return rules[i].rawKey < rules[j].rawKey
		}
		return len(rules[i].key) > len(rules[j].key)
	})

	return &AliasExpander{rules: rules}, nil
}

// Expand rewrites args by applying matching aliases. Shortcut aliases can chain.
// Same-prefix aliases are treated as defaults and applied once, with inserted
// flags placed before the user's remaining args so explicit CLI flags still win.
func (e *AliasExpander) Expand(args []string) ([]string, error) {
	defer perf.Track(nil, "internal.AliasExpander.Expand")()

	if e == nil || len(e.rules) == 0 || len(args) == 0 {
		return args, nil
	}

	current := append([]string(nil), args...)
	seen := make(map[string]struct{})

	for depth := 0; depth < maxAliasExpansionDepth; depth++ {
		rule := e.findLongestMatch(current)
		if rule == nil {
			return current, nil
		}

		signature := strings.Join(current, "\x00")
		if _, ok := seen[signature]; ok {
			return nil, fmt.Errorf("%w while expanding %q", errAliasCycleDetected, strings.Join(args, " "))
		}
		seen[signature] = struct{}{}

		next := applyAliasRule(*rule, current)
		if stringSlicesEqual(next, current) {
			return current, nil
		}

		if hasTokenPrefix(rule.value, rule.key) {
			return next, nil
		}
		current = next
	}

	return nil, fmt.Errorf("%w: %d steps while expanding %q", errAliasExpansionExhausted, maxAliasExpansionDepth, strings.Join(args, " "))
}

func (e *AliasExpander) findLongestMatch(args []string) *aliasRule {
	for i := range e.rules {
		rule := &e.rules[i]
		if hasTokenPrefix(args, rule.key) {
			return rule
		}
	}
	return nil
}

func applyAliasRule(rule aliasRule, args []string) []string {
	rest := args[len(rule.key):]
	if hasTokenPrefix(rule.value, rule.key) {
		inserted := suppressDefaultTokensFromEnv(rule.value[len(rule.key):])
		out := make([]string, 0, len(rule.key)+len(inserted)+len(rest))
		out = append(out, rule.key...)
		out = append(out, inserted...)
		out = append(out, rest...)
		return out
	}

	out := make([]string, 0, len(rule.value)+len(rest))
	out = append(out, rule.value...)
	out = append(out, rest...)
	return out
}

func parseAliasFields(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	return shell.Fields(value, nil)
}

func hasTokenPrefix(args, prefix []string) bool {
	if len(prefix) == 0 || len(args) < len(prefix) {
		return false
	}
	for i := range prefix {
		if args[i] != prefix[i] {
			return false
		}
	}
	return true
}

func suppressDefaultTokensFromEnv(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		name := flagNameFromArg(tokens[i])
		if name != "" && flagEnvIsSet(name) {
			if defaultTokenHasSeparateValue(tokens, i) {
				i++
			}
			continue
		}
		out = append(out, tokens[i])
	}
	return out
}

func defaultTokenHasSeparateValue(tokens []string, i int) bool {
	if strings.Contains(tokens[i], "=") {
		return false
	}
	return i+1 < len(tokens) && flagNameFromArg(tokens[i+1]) == ""
}

func flagNameFromArg(arg string) string {
	if !strings.HasPrefix(arg, dash) || arg == dash || arg == "--" {
		return ""
	}
	name := strings.TrimLeft(arg, dash)
	if idx := strings.IndexByte(name, '='); idx >= 0 {
		name = name[:idx]
	}
	return name
}

func flagEnvIsSet(name string) bool {
	if _, ok := os.LookupEnv(conventionalEnvVar(name)); ok {
		return true
	}
	if flag := flags.GlobalFlagsRegistry().Get(name); flag != nil {
		for _, env := range flag.GetEnvVars() {
			if _, ok := os.LookupEnv(env); ok {
				return true
			}
		}
	}
	return false
}

func conventionalEnvVar(name string) string {
	return "ATMOS_" + strings.ToUpper(strings.ReplaceAll(name, dash, "_"))
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
