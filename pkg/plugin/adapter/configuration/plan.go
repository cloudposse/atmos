package configuration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

type identityProvider interface {
	Identity(context.Context) (string, error)
}

// PlanIdentity fingerprints provider/options and watched source bytes without preparing the unit.
func (s *Session) PlanIdentity(ctx context.Context, info *schema.ConfigAndStacksInfo, sources []schema.ComponentConfigurationSource) (string, error) {
	defer perf.Track(nil, "configuration.Session.PlanIdentity")()

	selected, err := s.componentRequest(ctx, info)
	if err != nil {
		return "", err
	}
	request := selected.request
	identified, ok := selected.provider.(identityProvider)
	if !ok {
		return "", errUtils.ErrPluginUnsupportedExtension
	}
	identity, err := identified.Identity(ctx)
	if err != nil {
		return "", err
	}
	// Apply uses the preparation reviewed by plan, not another HCL evaluation.
	request.Operation = &pluginv1.ComponentOperation{Command: "plan", Mode: request.Operation.Mode, Options: request.Operation.Options}
	request.Invocation = nil
	request.RootDirectory = ""
	raw, err := (proto.MarshalOptions{Deterministic: true}).Marshal(request)
	if err != nil {
		return "", err
	}
	files, err := sourceDigests(ctx, s.root, sources)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		Provider string
		Request  string
		Sources  map[string]string
	}{identity, string(raw), files})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func sourceDigests(ctx context.Context, root string, sources []schema.ComponentConfigurationSource) (map[string]string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, source := range sources {
		if !filepath.IsLocal(source.Path) {
			return nil, errUtils.ErrPluginInvalidConfig
		}
		path := filepath.Join(root, filepath.FromSlash(source.Path))
		if !source.Directory {
			err = digestSource(ctx, root, path, result)
		} else {
			err = digestSourceDirectory(ctx, root, source.Path, result)
		}
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// digestSourceDirectory digests every non-excluded file below a watched directory source.
// A directory that does not exist is recorded as absent.
func digestSourceDirectory(ctx context.Context, root, sourcePath string, result map[string]string) error {
	top := filepath.Join(root, filepath.FromSlash(sourcePath))
	err := filepath.WalkDir(top, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != top && excludedPlanSource(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if excludedPlanSource(entry.Name()) {
			return nil
		}
		return digestSource(ctx, root, path, result)
	})
	if os.IsNotExist(err) {
		result[sourcePath] = "absent"
		return nil
	}
	return err
}

func excludedPlanSource(name string) bool {
	return name == ".atmos" || name == ".git" || name == ".terraform" || name == ".terragrunt-cache" || name == ".terragrunt-stack" || strings.HasPrefix(name, "terraform.tfstate") || strings.HasSuffix(name, ".planfile") || strings.HasSuffix(name, ".atmos.json")
}

func digestSource(ctx context.Context, root, path string, result map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(path)
	if os.IsNotExist(err) {
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		result[filepath.ToSlash(relative)] = "absent"
		return nil
	}
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, real)
	if err != nil || !filepath.IsLocal(relative) {
		return fmt.Errorf("%w: source leaves project root", errUtils.ErrPluginInvalidConfig)
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	relative, err = filepath.Rel(root, path)
	if err != nil {
		return err
	}
	result[filepath.ToSlash(relative)] = hex.EncodeToString(sum[:])
	return nil
}
