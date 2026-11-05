package create

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	generatorui "github.com/cloudposse/atmos/pkg/generator/ui"
	project "github.com/cloudposse/atmos/pkg/project/config"
)

func requestRecord(req *request, template *templates.Configuration, plan *generatorui.CreationPlan) (engine.File, error) {
	outputs := make(map[string]string)
	for _, output := range plan.Files {
		outputs[filepath.ToSlash(output.Path)] = fmt.Sprintf("%x", sha256.Sum256([]byte(output.Content)))
	}
	snapshot := *plan.Config
	snapshot.Spec.Values = plan.Values
	record := struct {
		Manifest  string                  `yaml:"manifest,omitempty"`
		Target    string                  `yaml:"target,omitempty"`
		Manifests []string                `yaml:"manifests,omitempty"`
		Template  string                  `yaml:"template"`
		Source    string                  `yaml:"source"`
		Revision  string                  `yaml:"revision,omitempty"`
		Scaffold  *project.ScaffoldConfig `yaml:"scaffold"`
		Outputs   map[string]string       `yaml:"outputs"`
	}{Template: template.Name, Source: template.Source, Revision: template.ResolvedRef, Scaffold: &snapshot, Outputs: outputs}
	key := fmt.Sprintf("%x.yaml", sha256.Sum256([]byte(filepath.ToSlash(req.file))))
	if req.target == "" {
		record.Manifest = filepath.ToSlash(req.file)
	} else {
		record.Target, record.Manifests = filepath.ToSlash(req.target), req.manifests
		key = fmt.Sprintf("directory-%x.yaml", sha256.Sum256([]byte(record.Target)))
	}
	content, err := yaml.Marshal(record)
	return engine.File{Path: filepath.Join(recordDirectory, key), Content: string(content), Permissions: recordPermissions}, err
}

// validateDirectoryRecord permits replacing directory history only after every previous output is gone.
func (p *Plan) validateDirectoryRecord() error {
	if p.Target == "" || p.staleRecord == "" {
		return nil
	}
	path := filepath.Join(p.Root, p.staleRecord)
	content, err := readDirectoryRecord(path)
	if err != nil {
		return err
	}
	var record struct {
		Target  string            `yaml:"target"`
		Outputs map[string]string `yaml:"outputs"`
	}
	if err := yaml.Unmarshal(content, &record); err != nil {
		return fmt.Errorf("%w: read generation record: %w", errUtils.ErrInvalidConfig, err)
	}
	if record.Target != filepath.ToSlash(p.Target) || len(record.Outputs) == 0 {
		return fmt.Errorf("%w: generation record does not describe target %s", errUtils.ErrInvalidConfig, p.Target)
	}
	return checkRecordedOutputs(p.Root, p.Target, record.Outputs)
}

func checkRecordedOutputs(root, target string, outputs map[string]string) error {
	for output := range outputs {
		relative, err := filepath.Rel(target, filepath.FromSlash(output))
		if err != nil || !filepath.IsLocal(output) || !filepath.IsLocal(relative) {
			return fmt.Errorf("%w: invalid recorded output %s", errUtils.ErrInvalidConfig, output)
		}
		if err := rejectRecordedOutput(root, output); err != nil {
			return err
		}
	}
	return nil
}

func rejectRecordedOutput(root, output string) error {
	if _, err := os.Lstat(filepath.Join(root, output)); err == nil {
		return fmt.Errorf("%w: previous directory generation still has output %s; choose a new target or remove its outputs before recreating", errUtils.ErrFileExists, output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %w", errUtils.ErrReadFile, err)
	}
	return nil
}

func readDirectoryRecord(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read generation record: %w", errUtils.ErrReadFile, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular generation record", errUtils.ErrInvalidConfig, path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrReadFile, err)
	}
	return content, nil
}
