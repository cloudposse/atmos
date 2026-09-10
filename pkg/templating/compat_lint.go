package templating

import (
	"net/url"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/cloudposse/atmos/pkg/perf"
	atmostemplate "github.com/cloudposse/atmos/pkg/template"
)

// datasourceCallNames are the gomplate functions that read a named datasource
// by alias, where the alias is a literal first argument.
var datasourceCallNames = map[string]struct{}{
	"ds":         {},
	"datasource": {},
	"include":    {},
}

// atmosDatasourceMethod is the `atmos.GomplateDatasource` method chain Atmos
// layers on top of gomplate's datasource functions (see rendererOnlyMethods
// in funcs.go); it reads a datasource the same way `ds`/`datasource` do.
const atmosDatasourceMethod = "atmos.GomplateDatasource"

// lintDeprecatedUsage warns about template constructs whose behavior changed
// under gomplate v5 without the function name itself being removed, so
// nothing else in this package catches them. It never fails a render: every
// finding here is a warning through reporter, same as the namespace and bare
// alias shims.
func lintDeprecatedUsage(parsed *template.Template, plan *renderPlan, reporter DeprecationReporter) {
	defer perf.Track(nil, "templating.lintDeprecatedUsage")()

	lintDatasourceSchemes(plan, reporter)

	atmostemplate.WalkNodes(parsed, func(node parse.Node) {
		switch n := node.(type) {
		case *parse.ChainNode:
			lintDotValueOnSecretsManagerDatasource(n, plan, reporter)
		case *parse.PipeNode:
			lintDatasourceSubPathURLPipe(n, plan, reporter)
		}
	})
}

// lintDatasourceSchemes warns about configured datasource URLs whose scheme
// gomplate v5 no longer supports, independent of whether the template
// actually reads them.
func lintDatasourceSchemes(plan *renderPlan, reporter DeprecationReporter) {
	for alias, ds := range plan.req.Datasources {
		if schemeOf(ds.URL) != "boltdb" {
			continue
		}
		reporter.Deprecated(plan.req.Name, "boltdb:// datasource ("+alias+")", "a supported datasource scheme",
			"the boltdb:// datasource scheme was removed in gomplate v5 and Atmos cannot emulate it. "+
				"Replace the \""+alias+"\" datasource with a supported scheme (for example file://, s3://, or a store-backed datasource).")
	}
}

// lintDotValueOnSecretsManagerDatasource warns about `(ds "alias" ...).Value`
// when alias is a configured aws+smp:// (AWS SSM Parameter Store) datasource:
// gomplate v5's aws+smp datasource returns the parameter value directly
// instead of a struct with a Value field, so `.Value` now looks up a field
// that doesn't exist.
func lintDotValueOnSecretsManagerDatasource(chain *parse.ChainNode, plan *renderPlan, reporter DeprecationReporter) {
	if len(chain.Field) == 0 || chain.Field[0] != "Value" {
		return
	}
	pipe, ok := chain.Node.(*parse.PipeNode)
	if !ok || len(pipe.Cmds) != 1 {
		return
	}
	cmd := pipe.Cmds[0]
	callName, ok := datasourceCallName(cmd)
	if !ok || callName == "include" {
		return
	}
	alias, ok := literalDatasourceAlias(cmd)
	if !ok {
		return
	}
	ds, ok := plan.req.Datasources[alias]
	if !ok || schemeOf(ds.URL) != "aws+smp" {
		return
	}
	reporter.Deprecated(plan.req.Name, callName+" \""+alias+"\" .Value", "the parameter value directly",
		"aws+smp:// datasources return the parameter value directly in gomplate v5 instead of a struct with a Value field. "+
			"Drop `.Value`; if you need the parsed JSON value of a String parameter, add `?type=application/json` to the datasource URL.")
}

// lintDatasourceSubPathURLPipe checks every command in pipe for a datasource
// sub-path call. A command at index > 0 within the pipe additionally
// receives the previous command's output as an implicit final argument (for
// example `{{ "app.yaml" | ds "dir" }}` passes "app.yaml" as ds's last
// argument at execution time), which is invisible to cmd.Args, so the
// command's position in the pipe tells lintDatasourceSubPathURL to count one
// extra argument.
func lintDatasourceSubPathURLPipe(pipe *parse.PipeNode, plan *renderPlan, reporter DeprecationReporter) {
	if pipe == nil {
		return
	}
	for i, cmd := range pipe.Cmds {
		extraArgs := 0
		if i > 0 {
			extraArgs = 1
		}
		lintDatasourceSubPathURL(cmd, extraArgs, plan, reporter)
	}
}

// lintDatasourceSubPathURL warns about a `ds`/`datasource`/`include`/
// `atmos.GomplateDatasource` call with two or more arguments (an alias plus a
// sub-path) whose alias URL doesn't end in "/": gomplate v5 resolves the
// sub-path argument as a relative URL against the alias URL, so a directory
// datasource must end with "/" to be treated as one. The extraArgs parameter
// accounts for a sub-path supplied through a pipeline rather than lexically
// in cmd.Args; see lintDatasourceSubPathURLPipe.
func lintDatasourceSubPathURL(cmd *parse.CommandNode, extraArgs int, plan *renderPlan, reporter DeprecationReporter) {
	callName, ok := datasourceCallName(cmd)
	if !ok || len(cmd.Args)+extraArgs < 3 {
		return
	}
	alias, ok := literalDatasourceAlias(cmd)
	if !ok {
		return
	}
	ds, ok := plan.req.Datasources[alias]
	if !ok || isDatasourceDirectoryURL(ds.URL) {
		return
	}
	reporter.Deprecated(plan.req.Name, callName+" \""+alias+"\" <sub-path>", "a trailing slash on the datasource URL",
		"sub-path arguments are resolved as relative URLs in gomplate v5; end the datasource URL with `/` so it is treated as a directory (currently: \""+ds.URL+"\").")
}

// isDatasourceDirectoryURL reports whether rawURL identifies a directory
// datasource. It parses the URL and checks the path component, so a query
// string (e.g. "file:///tmp/data/?type=json") doesn't hide a trailing slash
// in the path, and a query string ending in "/" (e.g.
// "file:///tmp/data?redirect=/") doesn't falsely look like one. If rawURL
// can't be parsed, it falls back to checking the raw string.
func isDatasourceDirectoryURL(rawURL string) bool {
	parsed, err := parseDatasourceURL(rawURL)
	if err != nil {
		return strings.HasSuffix(rawURL, "/")
	}
	return strings.HasSuffix(parsed.Path, "/")
}

// datasourceCallName reports the datasource function name at the head of
// cmd, if any: a bare identifier in datasourceCallNames, or the
// atmos.GomplateDatasource method chain.
func datasourceCallName(cmd *parse.CommandNode) (string, bool) {
	if cmd == nil || len(cmd.Args) == 0 {
		return "", false
	}
	switch head := cmd.Args[0].(type) {
	case *parse.IdentifierNode:
		if _, ok := datasourceCallNames[head.Ident]; ok {
			return head.Ident, true
		}
	case *parse.ChainNode:
		ident, ok := head.Node.(*parse.IdentifierNode)
		if ok && len(head.Field) > 0 && ident.Ident+"."+head.Field[0] == atmosDatasourceMethod {
			return atmosDatasourceMethod, true
		}
	}
	return "", false
}

// literalDatasourceAlias returns cmd's second argument (the alias) when it's
// a string literal. A dynamic alias (a variable, a field, a nested call)
// can't be resolved statically, so callers skip the finding rather than guess.
func literalDatasourceAlias(cmd *parse.CommandNode) (string, bool) {
	if len(cmd.Args) < 2 {
		return "", false
	}
	str, ok := cmd.Args[1].(*parse.StringNode)
	if !ok {
		return "", false
	}
	return str.Text, true
}

// schemeOf returns rawURL's scheme, or "" if it can't be parsed.
func schemeOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Scheme
}
