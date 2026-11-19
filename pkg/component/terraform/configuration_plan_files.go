package terraform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// publicFileRecord binds one non-sensitive generated file without storing its bytes.
// A provider can mark a file public yet have a before-hook put a private value in it, so the
// record keeps only a presence marker and a digest; apply reads the bytes back from the workdir.
type publicFileRecord struct {
	// Present distinguishes an absent file from an empty one.
	Present bool
	// Digest is the hex SHA-256 of the observed bytes; empty when the file is absent.
	Digest string
}

// isRemovalPolicy reports whether the file policy deletes the file instead of writing it.
func isRemovalPolicy(policy string) bool {
	return policy == "remove" || policy == "remove_terragrunt"
}

// observePublicFile reads one file below root and describes it for the record.
func observePublicFile(root *os.Root, path string) (publicFileRecord, error) {
	data, err := root.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return publicFileRecord{}, nil
	}
	if err != nil {
		return publicFileRecord{}, planIOError(err)
	}
	sum := sha256.Sum256(data)
	return publicFileRecord{Present: true, Digest: hex.EncodeToString(sum[:])}, nil
}

// snapshotPublicFiles records the presence and digest of retained generated files after before-hooks have run.
// Neither sensitive nor public bytes enter the record: the Sensitive flag cannot be trusted to cover
// values a hook writes later, so apply verifies the workdir bytes against these digests instead.
func snapshotPublicFiles(workdir string, files []schema.ComponentConfigurationFile) (string, error) {
	root, err := os.OpenRoot(workdir)
	if err != nil {
		return "", planIOError(err)
	}
	defer root.Close()
	records := make(map[string]publicFileRecord)
	for _, file := range files {
		if file.Sensitive || isRemovalPolicy(file.IfExists) {
			continue
		}
		record, err := observePublicFile(root, file.Path)
		if err != nil {
			return "", err
		}
		records[file.Path] = record
	}
	data, err := json.Marshal(records)
	return string(data), err
}

// restorePublicContent reads each recorded public file back from the retained workdir and accepts it
// only when presence and bytes match the record. Absent files keep the removal policy set at record time.
func restorePublicContent(workdir string, files []schema.ComponentConfigurationFile, records map[string]publicFileRecord) error {
	root, err := os.OpenRoot(workdir)
	if err != nil {
		return planIOError(err)
	}
	defer root.Close()
	for i := range files {
		if files[i].Sensitive {
			continue
		}
		record, ok := records[files[i].Path]
		if !ok {
			if isRemovalPolicy(files[i].IfExists) {
				continue
			}
			return fmt.Errorf("%w: reviewed generated file %s has no recorded digest", errUtils.ErrPlanVerificationFailed, files[i].Path)
		}
		data, err := verifyPublicFile(root, files[i].Path, record)
		if err != nil {
			return err
		}
		files[i].Content = data
	}
	return nil
}

// verifyPublicFile returns the workdir bytes of a public file after checking them against its record.
func verifyPublicFile(root *os.Root, path string, record publicFileRecord) ([]byte, error) {
	data, err := root.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if record.Present {
			return nil, fmt.Errorf("%w: reviewed generated file %s is missing: %w", errUtils.ErrPlanVerificationFailed, path, err)
		}
		return nil, nil
	}
	if err != nil {
		return nil, planIOError(err)
	}
	sum := sha256.Sum256(data)
	if !record.Present || hex.EncodeToString(sum[:]) != record.Digest {
		return nil, fmt.Errorf("%w: reviewed generated file %s changed", errUtils.ErrPlanVerificationFailed, path)
	}
	if data == nil {
		data = []byte{}
	}
	return data, nil
}

// planStackEnvironmentDigest binds overrides without persisting evaluated values.
// Explicit apply execution retains its provider defaults and is filtered per invocation.
func planStackEnvironmentDigest(info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) (string, error) {
	if prepared.Execution == nil || prepared.Execution.Apply != nil {
		return "", nil
	}
	return backendIdentityDigest(info.ComponentEnvSection)
}

func verifyPlanStackEnvironment(info *schema.ConfigAndStacksInfo, plan *configurationPlan) error {
	digest, err := planStackEnvironmentDigest(info, plan.Prepared)
	if err != nil {
		return err
	}
	if digest != plan.StackEnvironmentDigest {
		return fmt.Errorf("%w: reviewed stack environment overrides changed; create a new plan", errUtils.ErrPlanVerificationFailed)
	}
	return nil
}
