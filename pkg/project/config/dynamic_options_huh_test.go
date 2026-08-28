package config

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// This file proves -- via REAL huh execution (huh.Group/Field Update/Init
// cycles, not a direct call into dynamicOptionsFunc's closure) -- that a
// later field's dynamic Options actually re-resolves once an earlier
// field's bound answer changes, for every source field type huh's
// OptionsFunc bindings can now be scoped to (fieldFormContext.fieldPointers,
// generalized from the old select/input-blind multiselectValues). This
// matters because scaffold prompts fields one at a time, each its own
// huh.Group/page: a later field is only ever displayed or validated after
// the fields before it are already answered, so what's proven here is that
// it resolves correctly by that point -- not that the user watches it
// change on screen while another field is simultaneously visible.
//
// Calling dynamicOptionsFunc(...)() directly -- as
// TestDynamicOptionsFunc_ReactsToMultiselectMutation in
// field_options_test.go does -- only proves the closure itself re-resolves
// against whatever answers snapshot it's given. It never exercises huh's own
// Eval.shouldUpdate() bindings-hash gating (see
// github.com/charmbracelet/huh@v1.0.0/eval.go and field_select.go's
// `case updateFieldMsg:` branch), which is exactly the mechanism that was
// broken: OptionsFunc's bindings argument used to always be
// ctx.multiselectValues, so a select- or input-sourced field's own pointer
// was never present in another field's bindings map, and huh's hash-based
// change detection had nothing to compare against once that pointer's
// pointee changed. These tests drive that real mechanism with pumpGroup
// below.

// pumpGroup drives a tea.WindowSizeMsg{} -- the same message huh's own
// bubbletea.Program sends on startup and every subsequent field-rebuild
// trigger -- and any tea.Cmd it and its descendants produce, including
// nested tea.BatchMsg, through group's real Update loop until no field
// yields a further command (bounded, to guard against a runaway
// spinner.Tick chain). This is the same per-frame event pump a running
// tea.Program performs, and is what actually exercises huh's
// Eval.shouldUpdate() bindings-hash gating inside each field's own Update
// method.
func pumpGroup(t *testing.T, group *huh.Group) {
	t.Helper()

	pending := []tea.Msg{tea.WindowSizeMsg{}}
	for i := 0; i < 20 && len(pending) > 0; i++ {
		var next []tea.Msg
		for _, m := range pending {
			_, cmd := group.Update(m)
			next = append(next, drainCmd(cmd)...)
		}
		pending = next
	}
}

// drainCmd executes cmd (if non-nil), expanding a returned tea.BatchMsg into
// its constituent commands' own messages, and discarding a perpetual
// spinner.TickMsg -- huh's options-loading spinner, which never resolves on
// its own and isn't needed once the options-resolution command has already
// run in the same batch.
func drainCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drainCmd(c)...)
		}
		return out
	}
	if _, ok := msg.(spinner.TickMsg); ok {
		return nil
	}
	return []tea.Msg{msg}
}

// newSharedFieldFormContext builds the same fieldFormContext buildConfigForm
// threads across every field in a template, for tests that construct two
// fields directly via createFieldInContext instead of going through the full
// buildConfigForm/ScaffoldConfig path.
func newSharedFieldFormContext(render FieldOptionsRenderer) *fieldFormContext {
	return &fieldFormContext{
		valueGetters:  make(map[string]func() interface{}),
		fieldPointers: make(map[string]any),
		render:        render,
		delimiters:    defaultDelimiters(nil),
	}
}

// TestDynamicOptions_SelectSourcedFieldResolvesInRealHuhForm proves the fix
// for a select-sourced dynamic Options field: a later field whose Options is
// a Go-template expression referencing an earlier *select* field's answer
// now re-resolves once that earlier field's bound value changes, driven
// through huh's real Update cycle rather than a bare closure call.
func TestDynamicOptions_SelectSourcedFieldResolvesInRealHuhForm(t *testing.T) {
	render := func(_ string, answers map[string]interface{}, _ []string) ([]string, error) {
		chosen, _ := answers["chosen_env"].(string)
		if chosen == "" {
			return nil, nil
		}
		return []string{chosen}, nil
	}
	ctx := newSharedFieldFormContext(render)

	envField := &FieldDefinition{Name: "chosen_env", Type: "select", Options: []string{"dev", "staging", "prod"}}
	envHuhField, envGetter := createFieldInContext("chosen_env", envField, map[string]interface{}{"chosen_env": "dev"}, ctx)
	ctx.valueGetters["chosen_env"] = envGetter

	hintField := &FieldDefinition{Name: "region_hint", Type: "select", Options: `{{ list answers.chosen_env }}`}
	hintHuhField, hintGetter := createFieldInContext("region_hint", hintField, map[string]interface{}{}, ctx)
	ctx.valueGetters["region_hint"] = hintGetter

	group := huh.NewGroup(envHuhField, hintHuhField)

	// First real update cycle: resolves against the initial "dev" answer.
	pumpGroup(t, group)
	before := ansi.Strip(hintHuhField.View())
	require.Contains(t, before, "dev", "expected the initial dynamic option to reflect chosen_env's starting value")

	// Mutate the earlier select's bound value directly -- the same pointer
	// huh's own Select.updateValue() writes through on selection.
	envPtr, ok := ctx.fieldPointers["chosen_env"].(*string)
	require.True(t, ok, "expected chosen_env's field pointer to be registered as *string")
	*envPtr = "prod"

	// Second real update cycle: must re-resolve now that the bound value
	// changed, since that's precisely what optionsBindings/fieldPointers
	// wires into huh's OptionsFunc bindings argument.
	pumpGroup(t, group)
	after := ansi.Strip(hintHuhField.View())
	require.Contains(t, after, "prod", "expected the dynamic option to reflect the mutated select-sourced answer")
	require.False(t, strings.Contains(after, "dev"), "expected the stale option to be gone once the select-sourced answer changed")
}

// TestDynamicOptions_InputSourcedFieldResolvesInRealHuhForm proves the fix
// for an input-sourced dynamic Options field, mirroring the real-world
// csv_regions -> default_region fixture shipped in
// dynamic-options/scaffold.yaml (an `input` field feeding a `select` via a
// splitList Go-template expression).
func TestDynamicOptions_InputSourcedFieldResolvesInRealHuhForm(t *testing.T) {
	render := func(_ string, answers map[string]interface{}, _ []string) ([]string, error) {
		csv, _ := answers["csv_regions"].(string)
		if csv == "" {
			return nil, nil
		}
		return strings.Split(csv, ","), nil
	}
	ctx := newSharedFieldFormContext(render)

	csvField := &FieldDefinition{Name: "csv_regions", Type: "input", Default: "eu-west-1,us-east-1"}
	csvHuhField, csvGetter := createFieldInContext("csv_regions", csvField, map[string]interface{}{}, ctx)
	ctx.valueGetters["csv_regions"] = csvGetter

	regionField := &FieldDefinition{Name: "default_region", Type: "select", Options: `{{ splitList "," answers.csv_regions }}`}
	regionHuhField, regionGetter := createFieldInContext("default_region", regionField, map[string]interface{}{}, ctx)
	ctx.valueGetters["default_region"] = regionGetter

	group := huh.NewGroup(csvHuhField, regionHuhField)

	// First real update cycle: resolves against the initial default CSV value.
	pumpGroup(t, group)
	before := ansi.Strip(regionHuhField.View())
	require.Contains(t, before, "eu-west-1")
	require.Contains(t, before, "us-east-1")

	// Mutate the earlier input's bound value. A plain pointer write isn't
	// enough for an Input field: huh's own Input.Update always syncs
	// accessor <- textinput.Value() (never the reverse) on every real update
	// cycle, so a bare `*ptr = ...` would just be clobbered back to the
	// unfocused textinput's original buffer on the very next pump. Calling
	// the field's own exported Value() again -- the same call
	// createFieldInContext made to bind it originally -- re-syncs
	// textinput's internal buffer from the now-mutated pointer, exactly like
	// a real keystroke would, without needing to simulate one.
	csvPtr, ok := ctx.fieldPointers["csv_regions"].(*string)
	require.True(t, ok, "expected csv_regions's field pointer to be registered as *string")
	*csvPtr = "ap-southeast-2"
	csvInput, ok := csvHuhField.(*huh.Input)
	require.True(t, ok, "expected csv_regions to be a *huh.Input")
	csvInput.Value(csvPtr)

	// Second real update cycle: must re-resolve now that the bound input
	// value changed.
	pumpGroup(t, group)
	after := ansi.Strip(regionHuhField.View())
	require.Contains(t, after, "ap-southeast-2", "expected the dynamic option to reflect the mutated input-sourced answer")
	require.False(t, strings.Contains(after, "eu-west-1"), "expected the stale options to be gone once the input-sourced answer changed")
	require.False(t, strings.Contains(after, "us-east-1"), "expected the stale options to be gone once the input-sourced answer changed")
}

// TestDynamicOptions_MultiselectSourcedFieldStillResolvesInRealHuhForm is
// the no-regression counterpart: a multiselect-sourced dynamic Options field
// (the case that already worked pre-fix, via the old multiselectValues map)
// must still resolve correctly now that fieldPointers has replaced it.
func TestDynamicOptions_MultiselectSourcedFieldStillResolvesInRealHuhForm(t *testing.T) {
	ctx := newSharedFieldFormContext(nil) // dot-path source needs no renderer.

	envsField := &FieldDefinition{Name: "envs", Type: "multiselect", Options: []string{"dev", "staging", "prod"}}
	envsHuhField, envsGetter := createFieldInContext("envs", envsField, map[string]interface{}{"envs": []string{"dev"}}, ctx)
	ctx.valueGetters["envs"] = envsGetter

	defaultEnvField := &FieldDefinition{Name: "default_env", Type: "select", Options: "answers.envs"}
	defaultEnvHuhField, defaultEnvGetter := createFieldInContext("default_env", defaultEnvField, map[string]interface{}{}, ctx)
	ctx.valueGetters["default_env"] = defaultEnvGetter

	group := huh.NewGroup(envsHuhField, defaultEnvHuhField)

	// First real update cycle: resolves against the initial ["dev"] answer.
	pumpGroup(t, group)
	before := ansi.Strip(defaultEnvHuhField.View())
	require.Contains(t, before, "dev")
	require.False(t, strings.Contains(before, "staging"))

	// Mutate the earlier multiselect's bound value directly, exactly as
	// TestDynamicOptionsFunc_ReactsToMultiselectMutation does at the
	// closure level -- here proven through huh's own Update cycle instead.
	envsPtr, ok := ctx.fieldPointers["envs"].(*[]string)
	require.True(t, ok, "expected envs's field pointer to be registered as *[]string")
	*envsPtr = []string{"staging", "prod"}

	// Second real update cycle: must re-resolve now that the bound
	// multiselect value changed.
	pumpGroup(t, group)
	after := ansi.Strip(defaultEnvHuhField.View())
	require.Contains(t, after, "staging")
	require.Contains(t, after, "prod")
	require.False(t, strings.Contains(after, "dev"), "expected the stale option to be gone once the multiselect-sourced answer changed")
}

// TestDynamicOptions_LabelValueSourceFieldRendersRecoveredLabels proves the
// label/value feature end to end through real huh rendering: envs declares
// {label,value} options, default_env sources from it via a dot-path, and
// the rendered View() must show the *labels* ("Development"/"Staging") for
// only the values actually selected in envs -- not the raw "dev"/"staging"
// values, and not the filtered-out "Production" label -- while
// default_env's own bound Go value (what would flow into answers/templates)
// stays a plain value string throughout, never a label.
func TestDynamicOptions_LabelValueSourceFieldRendersRecoveredLabels(t *testing.T) {
	ctx := newSharedFieldFormContext(nil) // dot-path source needs no renderer.

	envsField := &FieldDefinition{Name: "envs", Type: "multiselect", Options: []any{
		map[string]interface{}{"label": "Development", "value": "dev"},
		map[string]interface{}{"label": "Staging", "value": "staging"},
		map[string]interface{}{"label": "Production", "value": "prod"},
	}}
	envsHuhField, envsGetter := createFieldInContext("envs", envsField, map[string]interface{}{"envs": []string{"dev", "staging"}}, ctx)
	ctx.valueGetters["envs"] = envsGetter
	ctx.fieldsByName = map[string]*FieldDefinition{"envs": envsField}

	defaultEnvField := &FieldDefinition{Name: "default_env", Type: "select", Options: "answers.envs", Default: "dev"}
	defaultEnvHuhField, defaultEnvGetter := createFieldInContext("default_env", defaultEnvField, map[string]interface{}{}, ctx)
	ctx.valueGetters["default_env"] = defaultEnvGetter

	group := huh.NewGroup(envsHuhField, defaultEnvHuhField)
	pumpGroup(t, group)

	rendered := ansi.Strip(defaultEnvHuhField.View())
	require.Contains(t, rendered, "Development", "expected the recovered label, not the raw value, to be displayed")
	require.Contains(t, rendered, "Staging")
	require.False(t, strings.Contains(rendered, "Production"), "expected the unselected option to be filtered out entirely")

	// The bound Go value -- what actually flows into answers/templates --
	// must be the plain value "dev", never the label "Development".
	require.Equal(t, "dev", defaultEnvGetter(), "expected the bound value to be the option's Value, not its Label")
}

// TestCreateFieldInContext_RequiredSelectValidatesDynamicOptionsOnBlur
// proves a Required select field whose Options is a dynamic dot-path wires
// its answers snapshot correctly into the field's own huh.Validate
// closure: huh.Select's real Blur() (which calls the stored validate func
// exactly like a Tab/Enter keypress does) must reject an empty selection,
// accept a value present in the dynamically resolved options, and reject
// one that isn't -- driven through huh's own Blur()/Error(), not by calling
// validateFieldValue directly.
func TestCreateFieldInContext_RequiredSelectValidatesDynamicOptionsOnBlur(t *testing.T) {
	ctx := newSharedFieldFormContext(nil) // dot-path source needs no renderer.

	envsField := &FieldDefinition{Name: "envs", Type: "multiselect", Options: []string{"dev", "staging", "prod"}}
	_, envsGetter := createFieldInContext("envs", envsField, map[string]interface{}{"envs": []string{"dev", "staging"}}, ctx)
	ctx.valueGetters["envs"] = envsGetter

	defaultEnvField := &FieldDefinition{Name: "default_env", Type: "select", Required: true, Options: "answers.envs"}
	defaultEnvHuhField, _ := createFieldInContext("default_env", defaultEnvField, map[string]interface{}{}, ctx)

	sel, ok := defaultEnvHuhField.(*huh.Select[string])
	require.True(t, ok, "expected default_env to be a *huh.Select[string]")

	envPtr, ok := ctx.fieldPointers["default_env"].(*string)
	require.True(t, ok, "expected default_env's field pointer to be registered as *string")

	// Empty selection: the required-field check fires before options are
	// even resolved.
	*envPtr = ""
	sel.Blur()
	require.Error(t, sel.Error())
	assert.ErrorIs(t, sel.Error(), errUtils.ErrGeneratorFieldRequired)

	// A value present in the dynamically resolved ["dev", "staging"] options.
	*envPtr = "dev"
	sel.Blur()
	assert.NoError(t, sel.Error())

	// A value absent from the dynamically resolved options.
	*envPtr = "prod"
	sel.Blur()
	require.Error(t, sel.Error())
}

// TestCreateFieldInContext_RequiredMultiSelectDynamicOptionsValidates proves
// the multiselect counterpart, driven through huh's real Update cycle
// (pumpGroup) rather than a direct closure call: a Required multiselect
// field whose own Options is itself a dynamic dot-path (not just a *source*
// for another field's Options, as every other dynamic-options huh test in
// this file exercises) resolves its OptionsFunc, re-selects its own
// previously-bound values against the freshly resolved options, and
// re-validates via huh's real updateValue()/Validate() path -- collapsing to
// empty (and firing the "at least one" required error) once its dynamic
// source itself becomes empty.
func TestCreateFieldInContext_RequiredMultiSelectDynamicOptionsValidates(t *testing.T) {
	ctx := newSharedFieldFormContext(nil) // dot-path source needs no renderer.

	envsField := &FieldDefinition{Name: "envs", Type: "multiselect", Options: []string{"dev", "staging", "prod"}}
	envsHuhField, envsGetter := createFieldInContext("envs", envsField, map[string]interface{}{"envs": []string{"dev"}}, ctx)
	ctx.valueGetters["envs"] = envsGetter

	regionsField := &FieldDefinition{Name: "regions", Type: "multiselect", Required: true, Options: "answers.envs"}
	regionsHuhField, regionsGetter := createFieldInContext("regions", regionsField, map[string]interface{}{"regions": []string{"dev"}}, ctx)
	ctx.valueGetters["regions"] = regionsGetter

	ms, ok := regionsHuhField.(*huh.MultiSelect[string])
	require.True(t, ok, "expected regions to be a *huh.MultiSelect[string]")

	group := huh.NewGroup(envsHuhField, regionsHuhField)

	// First real update cycle: envs resolves to ["dev"], and regions's own
	// pre-selected "dev" value is still valid against that -- no error.
	pumpGroup(t, group)
	assert.NoError(t, ms.Error())
	assert.Equal(t, []string{"dev"}, regionsGetter())

	// Mutate envs to empty: regions's dynamically resolved options collapse
	// to empty, so its own selection collapses to empty too, firing the
	// "at least one" required error.
	envsPtr, ok := ctx.fieldPointers["envs"].(*[]string)
	require.True(t, ok, "expected envs's field pointer to be registered as *[]string")
	*envsPtr = []string{}

	pumpGroup(t, group)
	require.Error(t, ms.Error())
	assert.ErrorIs(t, ms.Error(), errUtils.ErrGeneratorFieldRequired)
}
