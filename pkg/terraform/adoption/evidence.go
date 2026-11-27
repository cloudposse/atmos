package adoption

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

type evidence map[string]string

func (h evidence) add(command, source string) {
	if h[command] == "" {
		h[command] = source
	}
}

// The nearest manifest containing engine pins wins. Stop at the project or Git
// boundary so an unrelated ancestor's development tools cannot select a CLI.
func evidenceDirectories(dir, project string) []string {
	// Git metadata may be a file in a worktree. When invoked from a subdirectory,
	// its enclosing repository still owns CI definitions and tool pins.
	boundary := project
	rel, err := filepath.Rel(project, dir)
	if err != nil || (rel != "." && !filepath.IsLocal(rel)) {
		boundary = dir
	}
	for here := dir; ; here = filepath.Dir(here) {
		if _, err := os.Stat(filepath.Join(here, ".git")); err == nil {
			boundary = here
			break
		}
		if filepath.Dir(here) == here {
			break
		}
	}
	var dirs []string
	for here := dir; ; here = filepath.Dir(here) {
		dirs = append(dirs, here)
		if here == boundary || filepath.Dir(here) == here {
			return dirs
		}
	}
}

func projectPins(dirs []string) (map[string]string, error) {
	pins := map[string]string{}
	for _, here := range dirs {
		manifest, err := toolchain.LoadToolVersions(filepath.Join(here, ".tool-versions"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		pins, err = enginePins(manifest, here)
		if err != nil {
			return nil, err
		}
		if len(pins) > 0 {
			break
		}
	}
	return pins, nil
}

func deploymentHints(dirs []string, dir string) (evidence, error) {
	hints := evidence{}
	for _, here := range dirs {
		if err := atlantisHints(here, dir, hints); err != nil {
			return nil, err
		}
		if err := workflowHints(here, dir, hints); err != nil {
			return nil, err
		}
	}
	return hints, nil
}

func atlantisHints(repo, dir string, h evidence) error {
	path := filepath.Join(repo, "atlantis.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var cfg struct {
		Projects []struct {
			Name         string
			Dir          string
			Distribution string `yaml:"terraform_distribution"`
			Workflow     string
		}
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, p := range cfg.Projects {
		// A custom workflow may invoke a different CLI or omit the native plan entirely.
		if filepath.Join(repo, p.Dir) != dir || p.Dir == "" || p.Workflow != "" {
			continue
		}
		switch p.Distribution {
		case terraform:
			h.add(terraform, "atlantis.yaml "+p.Name)
		case "opentofu":
			h.add(tofu, "atlantis.yaml "+p.Name)
		}
	}
	return nil
}

type runDefaults struct {
	Run struct {
		Dir string `yaml:"working-directory"`
	}
}

func workflowHints(repo, dir string, h evidence) error {
	files, err := os.ReadDir(filepath.Join(repo, ".github", "workflows"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, f := range files {
		ext := filepath.Ext(f.Name())
		if f.IsDir() || (ext != ".yaml" && ext != ".yml") {
			continue
		}
		path := filepath.Join(repo, ".github", "workflows", f.Name())
		if err := readWorkflow(path, repo, dir, h); err != nil {
			return err
		}
	}
	return nil
}

func enginePins(manifest *toolchain.ToolVersions, here string) (map[string]string, error) {
	pins := map[string]string{}
	for key, versions := range manifest.Tools {
		var command string
		switch key {
		case terraform, "hashicorp/terraform":
			command = terraform
		case tofu, "opentofu", "opentofu/opentofu":
			command = tofu
		default:
			continue
		}
		if prev := pins[command]; prev != "" && prev != versions[0] {
			return nil, fmt.Errorf("%w: conflicting %s versions in %s/.tool-versions", errUtils.ErrInvalidToolSpec, command, here)
		}
		pins[command] = versions[0]
	}
	return pins, nil
}

func readWorkflow(path, repo, dir string, h evidence) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var workflow struct {
		Defaults runDefaults
		Jobs     map[string]struct {
			Defaults runDefaults
			Steps    []struct {
				Dir string `yaml:"working-directory"`
				Run string
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			wd := cmp.Or(step.Dir, job.Defaults.Run.Dir, workflow.Defaults.Run.Dir)
			if filepath.Join(repo, wd) != dir {
				continue
			}
			if cmd := literalPlan(step.Run); cmd != "" {
				h.add(cmd, filepath.Join(".github", "workflows", filepath.Base(path)))
			}
		}
	}
	return nil
}

func literalPlan(run string) string {
	// Only use a single literal command. Shell scripts and computed directories
	// require information that static discovery does not have.
	line := strings.TrimSpace(run)
	if strings.ContainsAny(line, "\n\r;&|$`<>\\()") {
		return ""
	}
	args := strings.Fields(line)
	if len(args) >= 2 && args[1] == "plan" && (args[0] == terraform || args[0] == tofu) {
		return args[0]
	}
	return ""
}
