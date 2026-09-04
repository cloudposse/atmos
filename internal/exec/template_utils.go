package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"text/template"
	"text/template/parse"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/samber/lo"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/templating"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	logKeyTemplate = "template"

	// Datasource timeout applied when neither `atmos.yaml` nor the stack
	// manifest's `settings.templates.settings` sets `gomplate.timeout`.
	defaultDatasourceTimeoutSeconds = 5
)

// missingKeyOption maps the "ignore missing template values" switch to the
// text/template "missingkey" option. With it off, a template that indexes a map
// with a key that is not present fails, e.g.:
//
//	template: catalog/terraform/eks_cluster_tmpl_hierarchical.yaml:17:12: executing "catalog/terraform/eks_cluster_tmpl_hierarchical.yaml" at <.flavor>: map has no entry for key "flavor"
func missingKeyOption(ignoreMissingTemplateValues bool) string {
	if ignoreMissingTemplateValues {
		return templating.MissingKeyDefault
	}
	return templating.MissingKeyError
}

// ProcessTmpl parses and executes Go templates with the Gomplate, Sprig and
// Atmos function sets.
func ProcessTmpl(
	atmosConfig *schema.AtmosConfiguration,
	tmplName string,
	tmplValue string,
	tmplData any,
	ignoreMissingTemplateValues bool,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.ProcessTmpl")()

	ctx := context.TODO()

	cfg := atmosConfig
	if cfg == nil {
		cfg = &schema.AtmosConfiguration{}
	}

	engine := templating.New()

	return engine.Render(ctx, &templating.Request{
		Name:       tmplName,
		Text:       tmplValue,
		Data:       tmplData,
		Funcs:      FuncMap(cfg, &schema.ConfigAndStacksInfo{}, ctx, engine),
		MissingKey: missingKeyOption(ignoreMissingTemplateValues),
	})
}

// convertRawEnvToStringMap converts a raw env value (any) to map[string]string.
// Handles map[string]any (with non-string values skipped) and map[string]string.
// Returns nil if the input is nil, an unsupported type, or produces an empty map.
func convertRawEnvToStringMap(envRaw any) map[string]string {
	if envRaw == nil {
		return nil
	}

	result := make(map[string]string)

	switch env := envRaw.(type) {
	case map[string]any:
		for k, v := range env {
			if s, ok := v.(string); ok {
				result[k] = s
			}
		}
	case map[string]string:
		for k, v := range env {
			result[k] = v
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// convertRawDelimitersToStringSlice converts a raw delimiters value (any) decoded from rendered
// YAML to []string, rejecting the whole value instead of silently dropping a bad element: e.g.
// `delimiters: ["<<", 1, ">>"]` must not become the valid-looking 2-item pair ["<<", ">>"], and
// `delimiters: [1, 2]` must not become an empty slice that resolveTemplateDelimiters' own
// empty-means-default fallback would then silently accept. A present-but-genuinely-empty
// sequence (`delimiters: []`) still returns a non-nil empty slice with no error (distinct from
// an absent key, which callers check separately via map key presence) so that fallback continues
// to apply to it specifically.
func convertRawDelimitersToStringSlice(delimitersRaw any) ([]string, error) {
	var raw []any

	switch delimiters := delimitersRaw.(type) {
	case []any:
		raw = delimiters
	case []string:
		for _, s := range delimiters {
			raw = append(raw, s)
		}
	default:
		return nil, fmt.Errorf("%w: 'delimiters' must be a list of strings, got %T", errUtils.ErrInvalidTemplateSettings, delimitersRaw)
	}

	result := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("%w: 'delimiters' elements must be non-empty strings, got %#v", errUtils.ErrInvalidTemplateSettings, v)
		}
		result = append(result, s)
	}

	return result, nil
}

// extractEnvFromRawMap extracts the env map from a raw settings map[string]any.
// This is needed because mapstructure:"-" on TemplatesSettings.Env causes
// mapstructure.Decode to silently drop the env field.
// The path navigated is: templates -> settings -> env.
func extractEnvFromRawMap(settingsMap map[string]any) map[string]string {
	templates, ok := settingsMap["templates"].(map[string]any)
	if !ok {
		return nil
	}

	settings, ok := templates["settings"].(map[string]any)
	if !ok {
		return nil
	}

	envRaw, ok := settings["env"]
	if !ok {
		return nil
	}

	return convertRawEnvToStringMap(envRaw)
}

// savedEnvVar stores the original state of an environment variable for restore.
type savedEnvVar struct {
	value   string
	existed bool
}

// setEnvVarsWithRestore sets environment variables and returns a cleanup function
// that restores the original values. This prevents env pollution across components.
func setEnvVarsWithRestore(envVars map[string]string) (func(), error) {
	saved := make(map[string]savedEnvVar, len(envVars))

	for k := range envVars {
		if original, existed := os.LookupEnv(k); existed {
			saved[k] = savedEnvVar{value: original, existed: true}
		} else {
			saved[k] = savedEnvVar{existed: false}
		}
	}

	cleanup := func() {
		for k, original := range saved {
			if original.existed {
				os.Setenv(k, original.value)
			} else {
				os.Unsetenv(k)
			}
		}
	}

	for k, v := range envVars {
		if err := os.Setenv(k, v); err != nil {
			// Return cleanup for vars already set before the failure.
			return cleanup, err
		}
	}

	return cleanup, nil
}

// ProcessTmplWithDatasources parses and executes Go templates with datasources.
func ProcessTmplWithDatasources(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	settingsSection schema.Settings,
	tmplName string,
	tmplValue string,
	tmplData any,
	ignoreMissingTemplateValues bool,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.ProcessTmplWithDatasources")()

	if !atmosConfig.Templates.Settings.Enabled {
		log.Debug("ProcessTmplWithDatasources: not processing templates since templating is disabled in 'atmos.yaml'", logKeyTemplate, tmplName)
		return tmplValue, nil
	}

	log.Trace("ProcessTmplWithDatasources", logKeyTemplate, tmplName)

	// Preserve env vars before mapstructure drops them.
	// mapstructure:"-" on TemplatesSettings.Env causes Decode to silently drop the field.
	// We extract env from both sources directly before the encode/decode/merge pipeline.
	cliEnv := atmosConfig.Templates.Settings.Env
	stackEnv := settingsSection.Templates.Settings.Env

	// Merge the template settings from `atmos.yaml` CLI config and from the stack manifests
	var cliConfigTemplateSettingsMap map[string]any
	var stackManifestTemplateSettingsMap map[string]any
	var templateSettings schema.TemplatesSettings

	err := mapstructure.Decode(atmosConfig.Templates.Settings, &cliConfigTemplateSettingsMap)
	if err != nil {
		return "", err
	}

	err = mapstructure.Decode(settingsSection.Templates.Settings, &stackManifestTemplateSettingsMap)
	if err != nil {
		return "", err
	}

	templateSettingsMerged, err := merge.Merge(atmosConfig, []map[string]any{cliConfigTemplateSettingsMap, stackManifestTemplateSettingsMap})
	if err != nil {
		return "", err
	}

	err = mapstructure.Decode(templateSettingsMerged, &templateSettings)
	if err != nil {
		return "", err
	}

	// Restore env vars that mapstructure dropped.
	// Stack manifest env overrides CLI config env (same precedence as the merge above).
	mergedEnv := make(map[string]string)
	for k, v := range cliEnv {
		mergedEnv[k] = v
	}
	for k, v := range stackEnv {
		mergedEnv[k] = v
	}
	if len(mergedEnv) > 0 {
		templateSettings.Env = mergedEnv
	}

	// Number of processing evaluations/passes
	evaluations, _ := lo.Coalesce(atmosConfig.Templates.Settings.Evaluations, 1)
	result := tmplValue

	// effectiveDelimiters carries the resolved delimiter pair across evaluation passes. It
	// starts from the pre-merge struct fields directly (nil stack-level Delimiters means "not
	// set, defer to CLI config"; non-nil, including explicitly empty, means "stack decides" --
	// see the Delimiters comment below for why templateSettings.Delimiters itself isn't used
	// here), then is only overwritten when a pass's own rendered output explicitly declares a
	// "settings.templates.settings.delimiters" key -- so a template that introduces new
	// delimiters on its first pass has them honored on the next pass, without a pass that
	// renders no such section silently reverting to the original config.
	effectiveDelimiters := settingsSection.Templates.Settings.Delimiters
	if effectiveDelimiters == nil {
		effectiveDelimiters = atmosConfig.Templates.Settings.Delimiters
	}

	// Set environment variables for template processing before the loop.
	// Restore originals when the function returns.
	if len(templateSettings.Env) > 0 {
		cleanup, envErr := setEnvVarsWithRestore(templateSettings.Env)
		if envErr != nil {
			return "", envErr
		}
		defer cleanup()
	}

	// Track extra env keys introduced by the evaluation loop that weren't in the
	// initial set, so we can clean them up when the function returns.
	extraLoopKeys := make(map[string]struct{})
	defer func() {
		for k := range extraLoopKeys {
			os.Unsetenv(k)
		}
	}()

	for i := 0; i < evaluations; i++ {
		log.Trace("ProcessTmplWithDatasources", logKeyTemplate, tmplName, "evaluation", i+1)

		rendered, err := renderTemplatePass(&templatePass{
			atmosConfig:                 atmosConfig,
			configAndStacksInfo:         configAndStacksInfo,
			templateSettings:            &templateSettings,
			name:                        tmplName,
			text:                        result,
			data:                        tmplData,
			delimiters:                  effectiveDelimiters,
			ignoreMissingTemplateValues: ignoreMissingTemplateValues,
		})
		if err != nil {
			return "", err
		}

		result = rendered
		resultMap, err := u.UnmarshalYAML[schema.AtmosSectionMapType](result)
		if err != nil {
			return "", err
		}

		if resultMapSettings, ok := resultMap["settings"].(map[string]any); ok {
			if resultMapSettingsTemplates, ok := resultMapSettings["templates"].(map[string]any); ok {
				if resultMapSettingsTemplatesSettings, ok := resultMapSettingsTemplates["settings"].(map[string]any); ok {
					// Extract env before mapstructure drops it.
					resultEnv := convertRawEnvToStringMap(resultMapSettingsTemplatesSettings["env"])

					// Validate delimiters before mapstructure.Decode below: a malformed
					// `settings.templates.settings.delimiters` value (e.g. a non-string
					// element) would otherwise surface as mapstructure's raw internal decode
					// error instead of the same ErrInvalidTemplateSettings message
					// resolveTemplateDelimiters gives for the same class of problem.
					var newDelimiters []string
					rawDelimiters, hasDelimiters := resultMapSettingsTemplatesSettings["delimiters"]
					if hasDelimiters {
						var convErr error
						newDelimiters, convErr = convertRawDelimitersToStringSlice(rawDelimiters)
						if convErr != nil {
							return "", convErr
						}
					}

					err = mapstructure.Decode(resultMapSettingsTemplatesSettings, &templateSettings)
					if err != nil {
						return "", err
					}

					// Restore env after mapstructure dropped it.
					// Also update OS env vars for the next evaluation pass.
					if len(resultEnv) > 0 {
						templateSettings.Env = resultEnv
						for k, v := range resultEnv {
							os.Setenv(k, v)
							// Track keys not in the initial set for deferred cleanup.
							if _, inInitial := mergedEnv[k]; !inInitial {
								extraLoopKeys[k] = struct{}{}
							}
						}
					}

					// Only update effectiveDelimiters when this pass's own rendered output
					// explicitly declared a delimiters key -- a pass whose output has no
					// "settings.templates.settings.delimiters" section at all must leave the
					// prior pass's resolved delimiters in place for the next pass, not silently
					// reset them to nil/empty.
					if hasDelimiters {
						effectiveDelimiters = newDelimiters
					}
				}
			}
		}
	}

	log.Trace("ProcessTmplWithDatasources: processed", logKeyTemplate, tmplName)

	return result, nil
}

// templatePass describes one evaluation pass of a stack manifest template.
type templatePass struct {
	atmosConfig                 *schema.AtmosConfiguration
	configAndStacksInfo         *schema.ConfigAndStacksInfo
	templateSettings            *schema.TemplatesSettings
	name                        string
	text                        string
	data                        any
	delimiters                  []string
	ignoreMissingTemplateValues bool
}

// renderTemplatePass renders one evaluation pass of a stack manifest template
// with the Gomplate (functions and datasources), Sprig and Atmos function sets,
// honoring the `templates.settings` toggles from `atmos.yaml` and the merged
// datasource definitions and timeout from the stack manifest.
func renderTemplatePass(pass *templatePass) (string, error) {
	gomplateEnabled := pass.atmosConfig.Templates.Settings.Gomplate.Enabled
	sprigEnabled := pass.atmosConfig.Templates.Settings.Sprig.Enabled

	ctx := context.TODO()
	cancel := func() {}
	var datasources map[string]templating.Datasource

	if gomplateEnabled {
		timeoutSeconds, _ := lo.Coalesce(pass.templateSettings.Gomplate.Timeout, defaultDatasourceTimeoutSeconds)
		ctx, cancel = context.WithTimeout(ctx, time.Second*time.Duration(timeoutSeconds))
		datasources = datasourcesFromSettings(pass.templateSettings.Gomplate.Datasources)
	}
	defer cancel()

	leftDelimiter, rightDelimiter, err := resolveTemplateDelimiters(pass.delimiters)
	if err != nil {
		return "", err
	}

	engine := templating.New(
		templating.WithGomplate(gomplateEnabled),
		templating.WithSprig(sprigEnabled),
	)

	return engine.Render(ctx, &templating.Request{
		Name:        pass.name,
		Text:        pass.text,
		Data:        pass.data,
		Funcs:       FuncMap(pass.atmosConfig, pass.configAndStacksInfo, context.TODO(), engine),
		LeftDelim:   leftDelimiter,
		RightDelim:  rightDelimiter,
		MissingKey:  missingKeyOption(pass.ignoreMissingTemplateValues),
		Datasources: datasources,
	})
}

// datasourcesFromSettings converts the `gomplate.datasources` configuration
// into datasource definitions for the template engine.
func datasourcesFromSettings(settings map[string]schema.TemplatesSettingsGomplateDatasource) map[string]templating.Datasource {
	if len(settings) == 0 {
		return nil
	}
	datasources := make(map[string]templating.Datasource, len(settings))
	for alias, definition := range settings {
		datasources[alias] = templating.Datasource{
			URL:     definition.Url,
			Headers: definition.Headers,
		}
	}
	return datasources
}

// resolveTemplateDelimiters returns the effective left/right Go template
// delimiters for the given configured pair, falling back to the default
// "{{"/"}}" when none are configured. Shared by every code path that needs
// to honor 'templates.settings.delimiters' from 'atmos.yaml'.
func resolveTemplateDelimiters(delimiters []string) (string, string, error) {
	leftDelimiter := "{{"
	rightDelimiter := "}}"

	if len(delimiters) == 0 {
		return leftDelimiter, rightDelimiter, nil
	}

	delimiterError := fmt.Errorf("%w: invalid 'templates.settings.delimiters' config in 'atmos.yaml': %v\n"+
		"'delimiters' must be an array with two string items: left and right delimiter\n"+
		"the left and right delimiters must not be an empty string", errUtils.ErrInvalidTemplateSettings, delimiters)

	if len(delimiters) != 2 || delimiters[0] == "" || delimiters[1] == "" {
		return "", "", delimiterError
	}

	return delimiters[0], delimiters[1], nil
}

// IsGolangTemplate checks if the provided string is a Go template, honoring
// the effective delimiters from atmosConfig.Templates.Settings.Delimiters
// (falling back to the default "{{"/"}}" when atmosConfig is nil or none are
// configured) — the same delimiters ProcessTmplWithDatasources actually
// executes it with. Without this, a project configured with custom
// delimiters (e.g. "[[ ]]") would have its templated import strings
// misclassified as plain text here, and treated as a genuinely missing
// import instead of a template awaiting later resolution.
func IsGolangTemplate(atmosConfig *schema.AtmosConfiguration, str string) (bool, error) {
	defer perf.Track(atmosConfig, "exec.IsGolangTemplate")()

	var configuredDelimiters []string
	if atmosConfig != nil {
		configuredDelimiters = atmosConfig.Templates.Settings.Delimiters
	}

	leftDelimiter, rightDelimiter, err := resolveTemplateDelimiters(configuredDelimiters)
	if err != nil {
		return false, err
	}

	t, err := template.New(str).Delims(leftDelimiter, rightDelimiter).Parse(str)
	if err != nil {
		return false, err
	}

	isGoTemplate := false

	// Iterate over all nodes in the template and check if any of them is of type `NodeAction` (field evaluation)
	for _, node := range t.Root.Nodes {
		if node.Type() == parse.NodeAction {
			isGoTemplate = true
			break
		}
	}

	return isGoTemplate, nil
}

// Create temporary directory.
func createTempDirectory() (string, error) {
	// Create a temporary directory for the temporary files.
	tempDir, err := os.MkdirTemp("", "atmos-templates-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	// Ensure directory permissions are restricted.
	if err := os.Chmod(tempDir, defaultDirPermissions); err != nil {
		return "", fmt.Errorf("failed to set temp directory permissions: %w", err)
	}
	return tempDir, nil
}

// Write merged JSON data to a temporary file and return its final file URL.
func writeMergedDataToFile(tempDir string, mergedData map[string]interface{}) (*url.URL, error) {
	// Write the merged JSON data to a file.
	rawJSON, err := json.Marshal(mergedData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged data to JSON: %w", err)
	}

	// Create a temporary file inside the temp directory.
	tmpfile, err := os.CreateTemp(tempDir, "gomplate-data-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp data file for gomplate: %w", err)
	}
	tmpName := tmpfile.Name()
	if _, err := tmpfile.Write(rawJSON); err != nil {
		tmpfile.Close()
		return nil, fmt.Errorf("failed to write JSON to temp file: %w", err)
	}
	if err := tmpfile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp data file: %w", err)
	}

	fileURL := toFileScheme(tmpName)

	finalFileUrl, err := fixWindowsFileScheme(fileURL)
	if err != nil {
		return nil, err
	}
	return finalFileUrl, nil
}

// Write the 'outer' top-level file and return its final file URL.
func writeOuterTopLevelFile(tempDir string, fileURL string, mergedData map[string]interface{}) (*url.URL, error) {
	// Keep merged data at the template root for templates that use `.name`,
	// while preserving the historical Env.README_YAML helper.
	topLevel := make(map[string]interface{}, len(mergedData))
	for k, v := range mergedData {
		topLevel[k] = v
	}

	envData := map[string]interface{}{}
	if existingEnv, ok := topLevel["Env"].(map[string]interface{}); ok {
		for k, v := range existingEnv {
			envData[k] = v
		}
	}
	envData["README_YAML"] = fileURL
	topLevel["Env"] = envData

	outerJSON, err := json.Marshal(topLevel)
	if err != nil {
		return nil, err
	}

	tmpfile2, err := os.CreateTemp(tempDir, "gomplate-top-level-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp data file for top-level: %w", err)
	}
	tmpName2 := tmpfile2.Name()
	if _, err = tmpfile2.Write(outerJSON); err != nil {
		tmpfile2.Close()
		return nil, fmt.Errorf("failed to write top-level JSON: %w", err)
	}
	if err = tmpfile2.Close(); err != nil {
		return nil, fmt.Errorf("failed to close top-level JSON: %w", err)
	}

	topLevelFileURL := toFileScheme(tmpName2)

	finalTopLevelFileURL, err := fixWindowsFileScheme(topLevelFileURL)
	if err != nil {
		return nil, err
	}
	return finalTopLevelFileURL, nil
}

// ProcessTmplWithDatasourcesGomplate parses and executes Go templates with datasources using Gomplate.
func ProcessTmplWithDatasourcesGomplate(
	atmosConfig *schema.AtmosConfiguration,
	tmplName string,
	tmplValue string,
	mergedData map[string]interface{},
	ignoreMissingTemplateValues bool,
) (string, error) {
	defer perf.Track(atmosConfig, "exec.ProcessTmplWithDatasourcesGomplate")()

	tempDir, err := createTempDirectory()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	finalFileUrl, err := writeMergedDataToFile(tempDir, mergedData)
	if err != nil {
		return "", err
	}

	finalTopLevelFileURL, err := writeOuterTopLevelFile(tempDir, finalFileUrl.String(), mergedData)
	if err != nil {
		return "", err
	}

	// Render with gomplate's own context handling: the merged data is exposed
	// on `.` (with the historical `.Env.README_YAML` helper) and as the
	// `config` datasource. Only Gomplate functions are available here.
	engine := templating.New(templating.WithSprig(false))

	return engine.Render(context.Background(), &templating.Request{
		Name:       tmplName,
		Text:       tmplValue,
		MissingKey: missingKeyOption(ignoreMissingTemplateValues),
		ContextSources: map[string]templating.Datasource{
			".":      {URL: finalTopLevelFileURL.String()},
			"config": {URL: finalFileUrl.String()},
		},
	})
}
