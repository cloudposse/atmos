package tfmigrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// ConfigEnvVar is tfmigrate's own config-path environment variable; when the
	// user sets it, Atmos must not override their config.
	ConfigEnvVar = "TFMIGRATE_CONFIG"

	backendKeyBucket    = "bucket"
	defaultMigrationDir = "./migrations"
)

// DefaultConfigInput carries the component context needed to generate a
// default tfmigrate config that inherits the component's Terraform backend as
// history storage.
type DefaultConfigInput struct {
	ComponentDir string
	BackendType  string
	Backend      map[string]any
	History      HistoryValues
}

// EnsureDefaultConfig makes tfmigrate history mode work with zero user
// configuration: when the user has not provided a tfmigrate config (no
// --tfmigrate-config flag or hook config, no .tfmigrate.hcl in the component
// working directory, no TFMIGRATE_CONFIG), it generates one that reuses the
// component's Terraform backend as history storage — S3 and GCS backends store
// history in the same bucket under the namespaced Atmos history key; any other
// backend falls back to a local history file (next to a local-backend state
// file when one is configured, so it survives workdir re-provisioning).
//
// Returns the generated config path and a cleanup function, or "" when the
// user's own configuration should be used untouched.
func EnsureDefaultConfig(input *DefaultConfigInput) (string, func(), error) {
	defer perf.Track(nil, "tfmigrate.EnsureDefaultConfig")()

	if os.Getenv(ConfigEnvVar) != "" { //nolint:forbidigo // TFMIGRATE_CONFIG is tfmigrate's own env var, not Atmos config.
		return "", nil, nil
	}
	if _, err := os.Stat(filepath.Join(input.ComponentDir, defaultConfigFile)); err == nil {
		return "", nil, nil
	}

	file, err := os.CreateTemp("", "atmos-tfmigrate-*.hcl")
	if err != nil {
		return "", nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCreateFile, err)
	}
	cleanup := func() { _ = os.Remove(file.Name()) } //nolint:gosec // G703: path is from os.CreateTemp, not user input.

	content := DefaultConfigHCL(input)
	if _, err := file.Write([]byte(content)); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrWriteFile, err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCloseFile, err)
	}
	return file.Name(), cleanup, nil
}

// DefaultConfigHCL renders the generated tfmigrate configuration.
func DefaultConfigHCL(input *DefaultConfigInput) string {
	defer perf.Track(nil, "tfmigrate.DefaultConfigHCL")()

	f := hclwrite.NewEmptyFile()
	root := f.Body().AppendNewBlock("tfmigrate", nil).Body()
	root.SetAttributeValue("migration_dir", cty.StringVal(MigrationDirFor(input.ComponentDir)))
	history := root.AppendNewBlock("history", nil).Body()

	switch input.BackendType {
	case "s3":
		writeS3HistoryStorage(history, input)
	case "gcs":
		writeGCSHistoryStorage(history, input)
	default:
		writeLocalHistoryStorage(history, input)
	}
	return string(f.Bytes())
}

// MigrationDirFor points tfmigrate at the conventional migrations/ directory
// when the component has one; otherwise the component root. Exported so
// callers building a `--migration <path>` value can strip a redundant
// migration_dir-matching prefix before handing it to tfmigrate, which
// resolves --migration relative to migration_dir itself (see
// StripMigrationDirPrefix).
func MigrationDirFor(componentDir string) string {
	defer perf.Track(nil, "tfmigrate.MigrationDirFor")()

	if HasMigrationsDir(componentDir) {
		return defaultMigrationDir
	}
	return "."
}

// HasMigrationsDir reports whether the component has a conventional
// migrations/ subdirectory. Exported so callers can tell "no migrations
// authored yet" apart from "migrations exist but none are unapplied" before
// invoking tfmigrate in zero-config history mode: without a migrations/ dir,
// MigrationDirFor falls back to the component root, which also holds Atmos's
// generated backend.tf.json/tfvars.json - letting tfmigrate's history-mode
// file scan run there makes it try (and fail) to parse those as migrations.
func HasMigrationsDir(componentDir string) bool {
	defer perf.Track(nil, "tfmigrate.HasMigrationsDir")()

	info, err := os.Stat(filepath.Join(componentDir, "migrations"))
	return err == nil && info.IsDir()
}

// NoMigrationsToRun reports whether zero-config history mode has nothing to
// run for this component: no explicit single-file migration was requested,
// and no migrations/ directory exists yet to hold any. Callers should skip
// invoking tfmigrate entirely in this case and report it, rather than let
// MigrationDirFor's component-root fallback make tfmigrate's history-mode
// file scan try (and fail) to parse Atmos's own generated files as migrations.
func NoMigrationsToRun(migration, componentDir string) bool {
	defer perf.Track(nil, "tfmigrate.NoMigrationsToRun")()

	return migration == "" && !HasMigrationsDir(componentDir)
}

// StripMigrationDirPrefix removes a leading migration_dir-matching prefix
// from a user-supplied --migration path, when the zero-config default
// migration_dir applies (i.e. Atmos generated the tfmigrate config, so Atmos
// - not the user - controls migration_dir). Without this, a natural-looking
// path like "migrations/foo.hcl" (which really is the migration file's path
// relative to the component dir) silently double-prefixes into
// "migrations/migrations/foo.hcl" once tfmigrate resolves it relative to
// migration_dir itself, producing a confusing "no such file" error.
func StripMigrationDirPrefix(migration, componentDir string) string {
	defer perf.Track(nil, "tfmigrate.StripMigrationDirPrefix")()

	if migration == "" {
		return migration
	}
	migrationDir := filepath.ToSlash(MigrationDirFor(componentDir))
	if migrationDir == "." {
		return migration
	}
	prefix := strings.TrimPrefix(migrationDir, "./") + "/"
	normalizedMigration := strings.TrimPrefix(filepath.ToSlash(migration), "./")
	trimmed := strings.TrimPrefix(normalizedMigration, prefix)
	if trimmed == normalizedMigration {
		return migration
	}
	return trimmed
}

func writeS3HistoryStorage(history *hclwrite.Body, input *DefaultConfigInput) {
	storage := history.AppendNewBlock("storage", []string{"s3"}).Body()
	setStringAttr(storage, "bucket", backendString(input.Backend, backendKeyBucket))
	setStringAttr(storage, "key", input.History.Key)
	setStringAttr(storage, "region", backendString(input.Backend, "region"))
	setStringAttr(storage, "profile", backendString(input.Backend, "profile"))
	setStringAttr(storage, "role_arn", s3RoleARN(input.Backend))
	setStringAttr(storage, "kms_key_id", backendString(input.Backend, "kms_key_id"))

	// Custom endpoints (emulators, MinIO, and other S3-compatible stores)
	// usually require path-style addressing and no real AWS account.
	if endpoint := s3Endpoint(input.Backend); endpoint != "" {
		setStringAttr(storage, "endpoint", endpoint)
		storage.SetAttributeValue("force_path_style", cty.BoolVal(true))
		storage.SetAttributeValue("skip_credentials_validation", cty.BoolVal(true))
		storage.SetAttributeValue("skip_metadata_api_check", cty.BoolVal(true))
	}
}

func writeGCSHistoryStorage(history *hclwrite.Body, input *DefaultConfigInput) {
	gcsBackend := input.Backend
	if nested, ok := input.Backend["gcs"].(map[string]any); ok {
		gcsBackend = nested
	}
	storage := history.AppendNewBlock("storage", []string{"gcs"}).Body()
	setStringAttr(storage, "bucket", backendString(gcsBackend, backendKeyBucket))
	setStringAttr(storage, "name", input.History.Key)
}

// writeLocalHistoryStorage stores history beside a local-backend state file
// when one is configured (durable across workdir re-provisioning), otherwise
// under the component working directory. The history key already namespaces by
// stack/component/workspace. EnsureLocalHistoryDir pre-creates the parent
// directory either way.
func writeLocalHistoryStorage(history *hclwrite.Body, input *DefaultConfigInput) {
	path := input.History.Key
	if input.BackendType == "local" {
		if statePath := backendString(input.Backend, "path"); statePath != "" {
			path = filepath.ToSlash(filepath.Join(filepath.Dir(statePath), input.History.Key))
		}
	}
	storage := history.AppendNewBlock("storage", []string{"local"}).Body()
	setStringAttr(storage, "path", path)
}

func setStringAttr(body *hclwrite.Body, name, value string) {
	if value == "" {
		return
	}
	body.SetAttributeValue(name, cty.StringVal(value))
}
