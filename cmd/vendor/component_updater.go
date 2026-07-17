package vendor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/vendoring"
	"github.com/cloudposse/atmos/pkg/vendoring/updater"
)

//nolint:revive // This mirrors the existing update command's independent selector inputs.
func runVendorUpdate(v *viper.Viper, componentType string, tags []string, typeChanged bool, components []string, group string, check bool) (*vendoring.UpdateReport, error) {
	// A nil selection means a direct --group invocation needs discovery. A
	// non-nil selection was already narrowed by the --pull-request discovery
	// pass and must not be rediscovered or widened.
	if group != "" && components == nil {
		discovery, err := runRepoWideUpdate(v, repoWideUpdateParams{typeChanged: typeChanged, componentType: componentType, tags: tags, check: true})
		if err != nil {
			return nil, err
		}
		components = filterGroupComponents(discovery, v.GetStringSlice("vendor.update.groups."+group+".include"), v.GetStringSlice("vendor.update.groups."+group+".exclude"))
		if check {
			return filterReport(discovery, components), nil
		}
		if len(components) == 0 {
			return &vendoring.UpdateReport{}, nil
		}
	}
	if group != "" && len(components) == 0 {
		// Never let an empty group selection fall through to an unscoped update.
		return &vendoring.UpdateReport{}, nil
	}
	if len(components) == 0 {
		return runUpdateWithSpinner(func(onProgress vendorProgressFunc) (*vendoring.UpdateReport, error) {
			return runRepoWideUpdate(v, repoWideUpdateParams{typeChanged: typeChanged, componentType: componentType, tags: tags, check: check, onProgress: onProgress})
		})
	}

	results := make([]vendoring.SourceUpdateResult, 0, len(components))
	for _, component := range components {
		resolved, err := vendoring.ResolveComponentSource(&vendoring.ResolveSourceParams{VendorFile: v.GetString("file"), Component: component, ComponentType: componentType})
		if err != nil {
			return nil, err
		}
		report, err := runUpdateWithSpinner(func(onProgress vendorProgressFunc) (*vendoring.UpdateReport, error) {
			return vendoring.UpdateResolved(resolved, &vendoring.UpdateParams{Tags: tags, DryRun: check, OnProgress: onProgress})
		})
		if err != nil {
			return nil, err
		}
		results = append(results, report.Results...)
	}
	return &vendoring.UpdateReport{Results: results}, nil
}

func normalizeComponentSelectors(components []string) []string {
	if components == nil {
		return nil
	}
	result := make([]string, 0, len(components))
	for _, component := range components {
		component = strings.TrimSpace(component)
		if component != "" && component != "[]" {
			result = append(result, component)
		}
	}
	return result
}

type updateInvocation struct {
	PullRequest bool
	All         bool
	Group       string
	Components  []string
}

func validateUpdateInvocation(v *viper.Viper, cmd *cobra.Command, invocation updateInvocation) error {
	if err := validateUpdateSelectors(v, invocation); err != nil {
		return err
	}
	if err := validateUpdateConfiguration(v); err != nil {
		return err
	}
	if invocation.PullRequest && cmd.Flags().Changed("check") && v.GetBool("check") {
		// A dry run is allowed with --pull-request, but it deliberately does no
		// branch work. The distinction is surfaced in both terminal and summary.
		return nil
	}
	if invocation.PullRequest {
		return validatePullRequestTemplates(v)
	}
	return nil
}

func validateUpdateSelectors(v *viper.Viper, invocation updateInvocation) error {
	if invocation.All && (invocation.Group != "" || len(invocation.Components) > 0) {
		return fmt.Errorf("%w: --all cannot be used with --group or --component", errUtils.ErrComponentUpdaterConfig)
	}
	if invocation.Group != "" && len(invocation.Components) > 0 {
		return fmt.Errorf("%w: --group and --component cannot be used together", errUtils.ErrComponentUpdaterConfig)
	}
	if f := v.GetString("format"); f != "table" && f != "json" {
		return fmt.Errorf("%w: --format must be table or json", errUtils.ErrComponentUpdaterConfig)
	}
	if invocation.Group != "" && !v.IsSet("vendor.update.groups."+invocation.Group) {
		return fmt.Errorf("%w: vendor.update.groups.%s is not configured", errUtils.ErrComponentUpdaterConfig, invocation.Group)
	}
	return nil
}

func validateUpdateConfiguration(v *viper.Viper) error {
	mode := v.GetString("vendor.update.execution.mode")
	if mode != "" && mode != "current" && mode != "worktree" {
		return fmt.Errorf("%w: vendor.update.execution.mode must be current or worktree", errUtils.ErrComponentUpdaterConfig)
	}
	batching := v.GetString("vendor.update.batching.mode")
	if batching != "" && batching != "scope" && batching != "component" {
		return fmt.Errorf("%w: vendor.update.batching.mode must be scope or component", errUtils.ErrComponentUpdaterConfig)
	}
	if batching == "component" && mode != "worktree" {
		return fmt.Errorf("%w: component batching requires vendor.update.execution.mode=worktree", errUtils.ErrComponentUpdaterConfig)
	}
	if batching == "component" {
		return fmt.Errorf("%w: component batching is configured but linked-worktree publishing is not available in this release", errUtils.ErrComponentUpdaterConfig)
	}
	return nil
}

func validatePullRequestTemplates(v *viper.Viper) error {
	for _, text := range []string{v.GetString("vendor.ci.pull_request.title"), v.GetString("vendor.ci.pull_request.body")} {
		if text == "" {
			continue
		}
		if _, err := template.New("component-updater").Funcs(templateFunctions()).Parse(text); err != nil {
			return fmt.Errorf("%w: invalid pull request template: %w", errUtils.ErrComponentUpdaterConfig, err)
		}
	}
	return nil
}

func filterGroupComponents(report *vendoring.UpdateReport, include, exclude []string) []string {
	components := make([]string, 0)
	seen := map[string]bool{}
	for _, result := range report.Results {
		if result.Status != vendoring.StatusUpdated || seen[result.Component] || !matchesPatterns(result.Component, include, true) || matchesPatterns(result.Component, exclude, false) {
			continue
		}
		seen[result.Component] = true
		components = append(components, result.Component)
	}
	sort.Strings(components)
	return components
}

func matchesPatterns(value string, patterns []string, empty bool) bool {
	if len(patterns) == 0 {
		return empty
	}
	for _, pattern := range patterns {
		if ok, _ := path.Match(pattern, value); ok {
			return true
		}
	}
	return false
}

func filterReport(report *vendoring.UpdateReport, components []string) *vendoring.UpdateReport {
	allowed := map[string]bool{}
	for _, component := range components {
		allowed[component] = true
	}
	result := &vendoring.UpdateReport{}
	for _, row := range report.Results {
		if allowed[row.Component] {
			result.Results = append(result.Results, row)
		}
	}
	return result
}

func updatedComponents(report *vendoring.UpdateReport) []string {
	var components []string
	for _, row := range report.Results {
		if row.Status == vendoring.StatusUpdated {
			components = append(components, row.Component)
		}
	}
	return components
}

func updateScope(group string, components []string) string {
	if group != "" {
		return "group-" + group
	}
	if len(components) == 0 {
		return "all"
	}
	if len(components) == 1 {
		return "components-" + components[0]
	}
	copy := append([]string(nil), components...)
	sort.Strings(copy)
	hash := sha256.Sum256([]byte(strings.Join(copy, "\x00")))
	return "components-" + hex.EncodeToString(hash[:])[:componentSelectionHashLength]
}

func prepareComponentUpdateBranch(ctx context.Context, v *viper.Viper, scope string) (string, string, error) {
	base := v.GetString("vendor.ci.pull_request.base_branch")
	if base == "" {
		var err error
		base, err = atmosgit.DefaultBranch(ctx, currentWorkdir, "origin")
		if err != nil {
			return "", "", err
		}
	}
	prefix := strings.Trim(v.GetString("vendor.ci.pull_request.branch_prefix"), "/")
	if prefix == "" {
		prefix = "atmos/component-updater"
	}
	branch := prefix + "/" + scope
	if err := atmosgit.PrepareBranch(ctx, atmosgit.PrepareBranchOptions{Workdir: currentWorkdir, Remote: "origin", Base: base, Branch: branch}); err != nil {
		return "", "", err
	}
	return branch, base, nil
}

type componentUpdatePublication struct {
	scope  string
	branch string
	base   string
	report *vendoring.UpdateReport
}

func publishComponentUpdate(ctx context.Context, v *viper.Viper, publication componentUpdatePublication) (*updater.PullRequest, string, error) {
	commit, err := commitAndPushComponentUpdate(ctx, publication.branch)
	if err != nil || commit == "" {
		return nil, commit, err
	}
	pr, err := reconcileComponentUpdatePullRequest(ctx, v, publication)
	if err != nil {
		return nil, "", err
	}
	return pr, commit, nil
}

func commitAndPushComponentUpdate(ctx context.Context, branch string) (string, error) {
	provider, err := atmosgit.NewProvider("cli")
	if err != nil {
		return "", err
	}
	rc := atmosgit.RepoContext{Workdir: currentWorkdir, Remote: "origin", Branch: branch}
	status, err := provider.Status(ctx, &atmosgit.StatusOptions{RepoContext: rc})
	if err != nil {
		return "", err
	}
	if status.Clean {
		return "", nil
	}
	paths := make([]string, 0, len(status.Entries))
	for _, entry := range status.Entries {
		paths = append(paths, entry.Path)
	}
	commit, err := provider.Commit(ctx, &atmosgit.CommitOptions{RepoContext: rc, Paths: paths, Message: "chore(components): update vendored components", Author: &atmosgit.Author{Name: "atmos[bot]", Email: "atmos-bot@users.noreply.github.com"}})
	if err != nil {
		return "", err
	}
	if !commit.Committed {
		return "", nil
	}
	if err := provider.Push(ctx, &atmosgit.PushOptions{RepoContext: rc, Retries: 1}); err != nil {
		return "", err
	}
	return commit.SHA, nil
}

func reconcileComponentUpdatePullRequest(ctx context.Context, v *viper.Viper, publication componentUpdatePublication) (*updater.PullRequest, error) {
	owner, repository, err := gitHubRepository(ctx, currentWorkdir, "origin")
	if err != nil {
		return nil, err
	}
	title, body, err := renderPRTemplates(v, publication.scope, publication.report)
	if err != nil {
		return nil, err
	}
	publisherName := v.GetString("vendor.ci.pull_request.provider")
	if publisherName == "" {
		publisherName = "github"
	}
	publisher, err := atmosgit.NewPullRequestPublisher(publisherName)
	if err != nil {
		return nil, err
	}
	labels := v.GetStringSlice("vendor.ci.pull_request.labels")
	if len(labels) == 0 {
		labels = []string{"component-update"}
	}
	pr, err := publisher.Reconcile(ctx, &atmosgit.PullRequestOptions{Owner: owner, Repository: repository, Base: publication.base, Head: publication.branch, Title: title, Body: body, Labels: labels, Draft: v.GetBool("vendor.ci.pull_request.draft"), Reviewers: v.GetStringSlice("vendor.ci.pull_request.reviewers"), Assignees: v.GetStringSlice("vendor.ci.pull_request.assignees")})
	if err != nil {
		return nil, err
	}
	return &updater.PullRequest{Number: pr.Number, URL: pr.URL}, nil
}

func templateFunctions() template.FuncMap {
	return template.FuncMap{"markdownTable": func(rows []vendoring.SourceUpdateResult) string {
		var b strings.Builder
		b.WriteString("| Component | Current | Latest |\n| --- | --- | --- |\n")
		for _, row := range rows {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", row.Component, row.CurrentVersion, row.LatestVersion)
		}
		return b.String()
	}}
}

func renderPRTemplates(v *viper.Viper, scope string, report *vendoring.UpdateReport) (string, string, error) {
	data := map[string]any{"scope": map[string]string{"name": scope}, "updates": report.Results}
	render := func(name, fallback string) (string, error) {
		value := v.GetString(name)
		if value == "" {
			value = fallback
		}
		t, err := template.New(name).Funcs(templateFunctions()).Parse(value)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		err = t.Execute(&b, data)
		return b.String(), err
	}
	title, err := render("vendor.ci.pull_request.title", "chore(components): update {{ .scope.name }}")
	if err != nil {
		return "", "", err
	}
	body, err := render("vendor.ci.pull_request.body", "{{ .updates | markdownTable }}")
	return title, body, err
}

func vendorSummaryEnabled(v *viper.Viper) bool {
	return !v.IsSet("vendor.ci.summary.enabled") || v.GetBool("vendor.ci.summary.enabled")
}

func renderVendorUpdateResult(report *vendoring.UpdateReport, check bool, v *viper.Viper, format string) {
	if format == "json" {
		return
	}
	renderUpdateReport(report, check, v.GetBool("outdated"), v.GetBool("archived"))
}

func renderComponentUpdaterJSON(result *updater.Result, format string) error {
	if format != "json" {
		return nil
	}
	return data.WriteJSON(result)
}

const (
	componentSelectionHashLength = 12
)

var (
	currentWorkdir   = "."
	gitHubRepository = atmosgit.GitHubRepository
)
