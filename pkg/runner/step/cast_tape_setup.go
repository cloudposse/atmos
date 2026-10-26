package step

import (
	"maps"
	"os"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// This file recognizes the shell idioms a VHS author types purely to
// establish state (cd, export, unset, clear). Atmos has native env: and
// working_directory: step config for exactly this, so a recognized idiom is
// lifted into tracked state instead of being replayed as a typed command.
//
// Recognition uses a real shell parser (mvdan.cc/sh) rather than string
// prefixes: only text that parses to exactly one simple command, with no
// operators, pipes, redirects, subshells or command substitution, is lifted.
// Anything else (`cd sub && pwd`, `cd "$(git rev-parse --show-toplevel)"`) runs
// as ordinary typed text, where the real shell applies it faithfully.

const (
	tapeSetupCd    = "cd"
	tapeSetupClear = "clear"
	tapeSetupEnv   = "export"
	tapeSetupUnset = "unset"
)

// tapeSetupCommand is a recognized setup idiom.
type tapeSetupCommand struct {
	Kind string // "export", "unset", "cd", "clear", or "" if text isn't a recognized setup command.
	Env  map[string]string
	Dir  string
}

// parseTapeSetupCommand returns the setup idiom the text consists of. The
// second result is false when the text is anything other than exactly one
// simple cd/export/unset/clear command whose words expand without running a
// command. Words are expanded against the process environment overlaid with
// the tape's tracked env, so `export PATH=$PATH:/x` and `cd ~` resolve the
// way the real shell would.
func parseTapeSetupCommand(text string, state *tapeTrackedState) (tapeSetupCommand, bool) {
	stmt, ok := parseSingleSimpleStatement(text)
	if !ok {
		return tapeSetupCommand{}, false
	}
	ex := newTapeExpander(state)
	switch cmd := stmt.Cmd.(type) {
	case *syntax.DeclClause:
		if cmd.Variant == nil || cmd.Variant.Value != tapeSetupEnv {
			return tapeSetupCommand{}, false
		}
		return liftTapeExport(cmd, ex)
	case *syntax.CallExpr:
		if len(cmd.Assigns) > 0 || len(cmd.Args) == 0 {
			return tapeSetupCommand{}, false
		}
		return liftTapeCall(cmd, ex)
	default:
		return tapeSetupCommand{}, false
	}
}

// parseSingleSimpleStatement parses text and returns its only statement when
// that statement is a plain foreground command with no negation, redirects or
// coprocess.
func parseSingleSimpleStatement(text string) (*syntax.Stmt, bool) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(text), "")
	if err != nil || len(file.Stmts) != 1 {
		return nil, false
	}
	stmt := file.Stmts[0]
	if stmt.Background || stmt.Coprocess || stmt.Negated || len(stmt.Redirs) > 0 {
		return nil, false
	}
	return stmt, true
}

func liftTapeCall(call *syntax.CallExpr, ex *tapeExpander) (tapeSetupCommand, bool) {
	switch call.Args[0].Lit() {
	case tapeSetupClear:
		if len(call.Args) != 1 {
			return tapeSetupCommand{}, false
		}
		return tapeSetupCommand{Kind: tapeSetupClear}, true
	case tapeSetupCd:
		return liftTapeCd(call, ex)
	case tapeSetupUnset:
		return liftTapeUnset(call, ex)
	default:
		return tapeSetupCommand{}, false
	}
}

// liftTapeCd lifts `cd`, `cd <dir>` and `cd ~`. `cd -`, options such as -P,
// and more than one operand are left to the real shell.
func liftTapeCd(call *syntax.CallExpr, ex *tapeExpander) (tapeSetupCommand, bool) {
	fields, err := ex.fields(call.Args[1:])
	if err != nil || len(fields) > 1 {
		return tapeSetupCommand{}, false
	}
	if len(fields) == 0 {
		home := ex.get("HOME")
		if home == "" {
			return tapeSetupCommand{}, false
		}
		return tapeSetupCommand{Kind: tapeSetupCd, Dir: home}, true
	}
	target := fields[0]
	if target == "" || strings.HasPrefix(target, "-") {
		return tapeSetupCommand{}, false
	}
	return tapeSetupCommand{Kind: tapeSetupCd, Dir: target}, true
}

// liftTapeUnset lifts `unset NAME...` (and the variable-selecting -v option).
func liftTapeUnset(call *syntax.CallExpr, ex *tapeExpander) (tapeSetupCommand, bool) {
	fields, err := ex.fields(call.Args[1:])
	if err != nil || len(fields) == 0 {
		return tapeSetupCommand{}, false
	}
	env := make(map[string]string, len(fields))
	for _, name := range fields {
		if name == "-v" {
			continue
		}
		if name == "" || strings.HasPrefix(name, "-") {
			return tapeSetupCommand{}, false
		}
		env[name] = ""
	}
	if len(env) == 0 {
		return tapeSetupCommand{}, false
	}
	return tapeSetupCommand{Kind: tapeSetupUnset, Env: env}, true
}

// liftTapeExport lifts `export NAME=value...` and `export NAME`. Assignments
// see the ones before them, as in a real shell (`export A=1 B=$A`).
func liftTapeExport(decl *syntax.DeclClause, ex *tapeExpander) (tapeSetupCommand, bool) {
	if len(decl.Args) == 0 {
		return tapeSetupCommand{}, false
	}
	env := make(map[string]string, len(decl.Args))
	for _, assign := range decl.Args {
		if assign.Name == nil || assign.Index != nil || assign.Array != nil {
			return tapeSetupCommand{}, false
		}
		if assign.Naked {
			// `export NAME` exports a variable that already exists; the tracked env already has it.
			continue
		}
		value := ""
		if assign.Value != nil {
			expanded, err := ex.literal(assign)
			if err != nil {
				return tapeSetupCommand{}, false
			}
			value = expanded
		}
		if assign.Append {
			value = ex.get(assign.Name.Value) + value
		}
		env[assign.Name.Value] = value
		ex.set(assign.Name.Value, value)
	}
	return tapeSetupCommand{Kind: tapeSetupEnv, Env: env}, true
}

// tapeExpander expands shell words against a snapshot of the environment the
// tape's commands would see. It never runs commands: a word that needs command
// substitution fails to expand, which makes the caller decline to lift it.
type tapeExpander struct {
	env map[string]string
}

func newTapeExpander(state *tapeTrackedState) *tapeExpander {
	env := make(map[string]string)
	for _, pair := range os.Environ() {
		if name, value, ok := strings.Cut(pair, "="); ok {
			env[name] = value
		}
	}
	maps.Copy(env, state.env)
	return &tapeExpander{env: env}
}

func (e *tapeExpander) config() *expand.Config {
	pairs := make([]string, 0, len(e.env))
	for name, value := range e.env {
		pairs = append(pairs, name+"="+value)
	}
	return &expand.Config{Env: expand.ListEnviron(pairs...)}
}

func (e *tapeExpander) get(name string) string { return e.env[name] }

func (e *tapeExpander) set(name, value string) { e.env[name] = value }

func (e *tapeExpander) fields(words []*syntax.Word) ([]string, error) {
	return expand.Fields(e.config(), words...)
}

func (e *tapeExpander) literal(assign *syntax.Assign) (string, error) {
	return expand.Literal(e.config(), assign.Value)
}

// tapeTextChangesShellState reports whether text contains a cd, unset or
// export at a command position anywhere in it, including inside lists and
// subshells. Text that does not parse reports false.
func tapeTextChangesShellState(text string) bool {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(text), "")
	if err != nil {
		return false
	}
	found := false
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.DeclClause:
			if n.Variant != nil && n.Variant.Value == tapeSetupEnv {
				found = true
			}
		case *syntax.CallExpr:
			if len(n.Args) > 0 {
				if name := n.Args[0].Lit(); name == tapeSetupCd || name == tapeSetupUnset {
					found = true
				}
			}
		}
		return !found
	})
	return found
}
