package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

func TestPublish_CreatesAndReportsTypedResult(t *testing.T) {
	e := newEnv(t, envConfig{})
	outcome := e.publish("dst", e.source("hello"))

	result := outcome.Operation
	assert.Equal(t, artifact.ActionPublish, result.Action)
	assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
	assert.Equal(t, "op-1", result.OperationID)
	assert.Equal(t, "dst", result.Repository)
	assert.Equal(t, "tools/example:1.0.0", result.Ref)
	assert.Equal(t, artifact.LockScopeHost, result.LockScope, "the actual lock scope is reported")
	assert.NotEmpty(t, result.RecordDigest)
	assert.Equal(t, outcome.Record.ContentDigest, result.ContentDigest)
	assert.NotEmpty(t, result.Location)
	assert.False(t, outcome.Existing)
	assert.Equal(t, artifact.NewTimestamp(startTime), outcome.Record.CreatedAt)
	assert.Empty(t, e.repos["dst"].HeldLocks(), "locks are released")
	e.assertStagingClean()
	assert.Equal(t, []service.JournalPhase{service.PhasePrepared, service.PhaseLocked, service.PhasePending, service.PhaseCommitted, service.PhaseReleased}, e.journal.phases())

	// The interface method returns the same result.
	again, err := e.svc.Publish(context.Background(), e.publishRequest("dst", e.source("hello")))
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeUnchanged, again.Outcome)
}

func TestPublish_IdenticalRepublishIsUnchangedEvenWhenTimeAndRunMetadataDiffer(t *testing.T) {
	e := newEnv(t, envConfig{})
	src := e.source("hello")
	first := e.publish("dst", src)

	e.clock.Advance(30 * time.Hour)
	request := e.publishRequest("dst", src)
	request.Metadata.Provenance = &artifact.Provenance{CI: &artifact.CIProvenance{Provider: "github", RunID: "999"}}
	second, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)

	assert.True(t, second.Existing)
	assert.Equal(t, artifact.OutcomeUnchanged, second.Operation.Outcome)
	firstBytes, err := first.Record.Canonical()
	require.NoError(t, err)
	secondBytes, err := second.Record.Canonical()
	require.NoError(t, err)
	assert.Equal(t, firstBytes, secondBytes, "the EXISTING record is returned byte for byte")
	assert.Equal(t, artifact.NewTimestamp(startTime), second.Record.CreatedAt, "created_at stays the original")
	assert.Equal(t, first.Operation.RecordDigest, second.Operation.RecordDigest)
	assert.Equal(t, "op-2", second.Operation.OperationID, "the current execution is reported separately")
	assert.Equal(t, 1, e.repos["dst"].Calls("Publish"), "an unchanged no-op writes nothing")
	assert.Nil(t, second.Record.Metadata.Provenance, "run metadata of the new execution never rewrites the stored record")
}

func TestPublish_ChangedIntentConflictsWithoutForceAndReplacesWithIt(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, src string, request *artifact.PublishRequest)
		reason string
	}{
		{"content", func(t *testing.T, src string, _ *artifact.PublishRequest) {
			fixtures.WriteFile(t, src, "data.txt", "changed", 0o644)
		}, "content"},
		{"mode", func(t *testing.T, src string, _ *artifact.PublishRequest) {
			fixtures.RequirePOSIXModes(t)
			require.NoError(t, os.Chmod(filepath.Join(src, "data.txt"), 0o755))
		}, "content"},
		{"document", func(t *testing.T, src string, _ *artifact.PublishRequest) {
			fixtures.WriteFile(t, src, fixtures.ReservedDir+"/README.md", "new readme\n", 0o644)
		}, "content"},
		{"labels", func(_ *testing.T, _ string, request *artifact.PublishRequest) {
			request.Metadata.Labels = map[string]string{"team": "platform"}
		}, "labels"},
		{"description", func(_ *testing.T, _ string, request *artifact.PublishRequest) {
			request.Metadata.Description = "new description"
		}, "description"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, envConfig{})
			src := e.source("hello")
			first := e.publish("dst", src)
			request := e.publishRequest("dst", src)
			tt.mutate(t, src, &request)

			outcome, err := e.svc.PublishDetailed(context.Background(), request)
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
			assert.Contains(t, err.Error(), tt.reason)
			assert.NotNil(t, outcome, "a result is returned alongside the error")
			assert.Equal(t, "op-2", outcome.Operation.OperationID)
			current, ok := e.repos["dst"].RecordDigestOf(exampleRef)
			require.True(t, ok)
			assert.Equal(t, first.Operation.RecordDigest, current, "the stored artifact is untouched")
			assert.Empty(t, e.repos["dst"].HeldLocks())

			request.Force = true
			replaced, err := e.svc.PublishDetailed(context.Background(), request)
			require.NoError(t, err)
			assert.Equal(t, artifact.OutcomeReplaced, replaced.Operation.Outcome)
			current, _ = e.repos["dst"].RecordDigestOf(exampleRef)
			assert.Equal(t, replaced.Operation.RecordDigest.String(), current.String())
			e.assertStagingClean()
		})
	}
}

func TestPublish_ForceOnIdenticalContentStaysUnchanged(t *testing.T) {
	e := newEnv(t, envConfig{})
	src := e.source("hello")
	e.publish("dst", src)
	request := e.publishRequest("dst", src)
	request.Force = true
	outcome, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeUnchanged, outcome.Operation.Outcome)
	assert.Equal(t, 1, e.repos["dst"].Calls("Publish"))
}

// pluginPolicy treats artifacts labeled format=plugin as immutable releases.
func pluginPolicy() service.ImmutabilityPolicy {
	isPlugin := func(labels map[string]string) bool { return labels["format"] == "plugin" }
	return service.ImmutabilityFunc(func(c service.Candidate) (bool, string) {
		switch {
		case c.Inspection != nil && c.Inspection.Record != nil && isPlugin(c.Inspection.Record.Metadata.Labels):
			return true, "plugin format"
		case c.Prepared != nil && isPlugin(c.Prepared.Record.Metadata.Labels):
			return true, "plugin format"
		}
		return false, ""
	})
}

func TestPublish_PluginFormatReleasesStayImmutableEvenWithForce(t *testing.T) {
	plugin := map[string]string{"format": "plugin"}
	tests := []struct {
		name          string
		existing      map[string]string
		incoming      map[string]string
		wantImmutable bool
		reason        string
	}{
		{"existing plugin", plugin, nil, true, "existing release"},
		{"incoming plugin", nil, plugin, true, "incoming release"},
		{"both plugin", plugin, map[string]string{"format": "plugin", "v": "2"}, true, "existing release"},
		{"neither", nil, map[string]string{"x": "y"}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, envConfig{options: []service.Option{service.WithImmutabilityPolicy(pluginPolicy())}})
			src := e.source("hello")
			request := e.publishRequest("dst", src)
			request.Metadata.Labels = tt.existing
			first, err := e.svc.PublishDetailed(context.Background(), request)
			require.NoError(t, err)

			request.Metadata.Labels = tt.incoming
			request.Force = true
			outcome, err := e.svc.PublishDetailed(context.Background(), request)
			if !tt.wantImmutable {
				require.NoError(t, err)
				assert.Equal(t, artifact.OutcomeReplaced, outcome.Operation.Outcome)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
			assert.Contains(t, err.Error(), tt.reason)
			assert.Contains(t, err.Error(), "immutable")
			current, _ := e.repos["dst"].RecordDigestOf(exampleRef)
			assert.Equal(t, first.Operation.RecordDigest, current)
		})
	}
}

func TestPublish_IdenticalPluginReleaseRepublishIsStillAnUnchangedNoOp(t *testing.T) {
	e := newEnv(t, envConfig{options: []service.Option{service.WithImmutabilityPolicy(pluginPolicy())}})
	src := e.source("hello")
	request := e.publishRequest("dst", src)
	request.Metadata.Labels = map[string]string{"format": "plugin"}
	_, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)
	request.Force = true
	outcome, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeUnchanged, outcome.Operation.Outcome)
}

func TestPublish_ImmutabilityPolicyMockSeesBothSides(t *testing.T) {
	ctrl := gomock.NewController(t)
	policy := service.NewMockImmutabilityPolicy(ctrl)
	e := newEnv(t, envConfig{options: []service.Option{service.WithImmutabilityPolicy(policy)}})
	src := e.source("hello")
	e.publish("dst", src)

	var sawExisting, sawIncoming bool
	policy.EXPECT().Immutable(gomock.Any()).DoAndReturn(func(c service.Candidate) (bool, string) {
		if c.Existing {
			sawExisting = true
			assert.NotNil(t, c.Inspection)
		} else {
			sawIncoming = true
			assert.NotNil(t, c.Prepared)
		}
		return false, ""
	}).Times(2)
	request := e.publishRequest("dst", src)
	request.Force = true
	request.Metadata.Description = "changed"
	_, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)
	assert.True(t, sawExisting && sawIncoming, "both the existing and the incoming artifact are admitted")
}

func TestPublish_NameAndVersionResolutionHappensBeforeAnyWrite(t *testing.T) {
	e := newEnv(t, envConfig{})
	src := e.source("hello")
	tests := []struct {
		name    string
		mutate  func(*artifact.PublishRequest)
		wantErr error
	}{
		{"missing version", func(r *artifact.PublishRequest) { r.Version = "" }, errUtils.ErrArtifactRefInvalid},
		{"invalid version", func(r *artifact.PublishRequest) { r.Version = "bad version" }, errUtils.ErrArtifactRefInvalid},
		{"invalid name", func(r *artifact.PublishRequest) { r.Name = "Bad_Name" }, errUtils.ErrArtifactRefInvalid},
		{"traversal name", func(r *artifact.PublishRequest) { r.Name = "../x" }, errUtils.ErrArtifactUnsafePath},
		{"missing repository", func(r *artifact.PublishRequest) { r.Repository = "" }, errUtils.ErrArtifactConfigInvalid},
		{"missing source", func(r *artifact.PublishRequest) { r.Source = "" }, errUtils.ErrArtifactConfigInvalid},
		{"invalid kind", func(r *artifact.PublishRequest) { r.Kind = "Bad Kind" }, errUtils.ErrArtifactRecordInvalid},
		{"unregistered kind", func(r *artifact.PublishRequest) { r.Kind = "toolchain/package" }, errUtils.ErrArtifactConfigInvalid},
		{"bad labels", func(r *artifact.PublishRequest) { r.Metadata.Labels = map[string]string{"a=b": "c"} }, errUtils.ErrArtifactConfigInvalid},
		{"unknown repository", func(r *artifact.PublishRequest) { r.Repository = "nope" }, errUtils.ErrArtifactConfigInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := e.publishRequest("dst", src)
			tt.mutate(&request)
			outcome, err := e.svc.PublishDetailed(context.Background(), request)
			require.ErrorIs(t, err, tt.wantErr)
			require.NotNil(t, outcome)
			assert.NotEmpty(t, outcome.Operation.OperationID)
			assert.NotEmpty(t, outcome.Operation.Diagnostics, "the failure is reported in the result")
			assert.Zero(t, e.repos["dst"].TotalCalls(), "nothing reached the repository")
			e.assertStagingClean()
		})
	}
}

func TestPublish_BasenameNameFallbackAndInvalidBasename(t *testing.T) {
	e := newEnv(t, envConfig{})
	good := filepath.Join(t.TempDir(), "my-app")
	require.NoError(t, os.Mkdir(good, 0o755))
	fixtures.WriteFile(t, good, "a.txt", "a", 0o644)
	outcome, err := e.svc.PublishDetailed(context.Background(), artifact.PublishRequest{Repository: "dst", Source: good, Version: "1"})
	require.NoError(t, err)
	assert.Equal(t, "my-app", outcome.Record.Name)

	bad := filepath.Join(t.TempDir(), "My App")
	require.NoError(t, os.Mkdir(bad, 0o755))
	fixtures.WriteFile(t, bad, "a.txt", "a", 0o644)
	_, err = e.svc.PublishDetailed(context.Background(), artifact.PublishRequest{Repository: "dst", Source: bad, Version: "1"})
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
	assert.Contains(t, err.Error(), "explicit name")
}

func TestPublish_MergesRepositoryDefaultMetadataUnderExplicitValues(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.setInfo("dst", func(info *service.RepositoryInfo) {
		info.Defaults = artifact.Metadata{Description: "repo default", Labels: map[string]string{"team": "platform", "env": "dev"}}
	})
	request := e.publishRequest("dst", e.source("hello"))
	request.Metadata.Labels = map[string]string{"env": "prod"}
	outcome, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"team": "platform", "env": "prod"}, outcome.Record.Metadata.Labels)
	assert.Equal(t, "repo default", outcome.Record.Metadata.Description)

	request = e.publishRequest("dst", e.source("other"))
	request.Version = "2.0.0"
	request.Metadata.Description = "explicit"
	outcome, err = e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, "explicit", outcome.Record.Metadata.Description)
}

func TestPublish_CapabilityAndLockGuaranteesAreCheckedBeforeMutation(t *testing.T) {
	readOnly := artifacttest.DefaultCapabilities()
	readOnly.Write = false
	badPackaging := artifacttest.DefaultCapabilities()
	badPackaging.Packagings = []artifact.Packaging{artifact.PackagingZip}
	tests := []struct {
		name string
		caps artifact.Capabilities
		info service.RepositoryInfo
	}{
		{"read only", readOnly, service.RepositoryInfo{Packaging: artifact.PackagingNone}},
		{"unsupported packaging", badPackaging, service.RepositoryInfo{Packaging: artifact.PackagingTgz}},
		{"insufficient lock scope", artifacttest.DefaultCapabilities(), service.RepositoryInfo{Packaging: artifact.PackagingNone, RequiredLockScope: artifact.LockScopeRepository}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := tt.caps
			e := newEnv(t, envConfig{repos: map[string]artifacttest.Options{"dst": {Capabilities: &caps}}})
			info := tt.info
			info.Capabilities = caps
			e.static.Info["dst"] = info
			outcome, err := e.svc.PublishDetailed(context.Background(), e.publishRequest("dst", e.source("hello")))
			require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
			assert.Zero(t, e.repos["dst"].TotalCalls(), "rejected before any repository call, including Lock")
			assert.NotEmpty(t, outcome.Operation.OperationID)
			e.assertStagingClean()
		})
	}
}

func TestPublish_RepositoryScopeLockIsReportedAsActual(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	caps.LockScopes = []artifact.LockScope{artifact.LockScopeHost, artifact.LockScopeRepository}
	e := newEnv(t, envConfig{repos: map[string]artifacttest.Options{"dst": {Capabilities: &caps}}})
	e.setInfo("dst", func(info *service.RepositoryInfo) { info.RequiredLockScope = artifact.LockScopeRepository })
	outcome := e.publish("dst", e.source("hello"))
	assert.Equal(t, artifact.LockScopeRepository, outcome.Operation.LockScope)
}

func TestPublish_RegisteredKindProducerReceivesCapturedIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	producer := artifact.NewMockProducer(ctrl)
	producer.EXPECT().Kind().Return("scaffold").AnyTimes()
	registry := artifact.NewProducerRegistry()
	require.NoError(t, registry.Register(producer))

	e := newEnv(t, envConfig{})
	svc, err := service.New(e.static, registry, service.WithClock(e.clock.Now), service.WithIDGenerator(func() string { return "fixed-op" }), service.WithStagingRoot(e.staging))
	require.NoError(t, err)

	producer.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request artifact.PrepareRequest) (*artifact.PreparedArtifact, error) {
		assert.Equal(t, "fixed-op", request.OperationID)
		assert.True(t, startTime.Equal(request.PreparedAt))
		assert.Equal(t, time.UTC, request.PreparedAt.Location())
		assert.Equal(t, "scaffold", request.Kind)
		assert.Equal(t, artifact.PackagingNone, request.Packaging)
		return nil, errUtils.ErrArtifactPackagingInvalid
	})
	request := e.publishRequest("dst", e.source("hello"))
	request.Kind = "scaffold"
	_, err = svc.Publish(context.Background(), request)
	require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
}

func TestPublish_DryRunPerformsNoRepositoryOrIdentityCalls(t *testing.T) {
	t.Run("in-memory repository", func(t *testing.T) {
		e := newEnv(t, envConfig{})
		request := e.publishRequest("dst", e.source("hello"))
		request.DryRun = true
		outcome, err := e.svc.PublishDetailed(context.Background(), request)
		require.NoError(t, err)
		assert.Equal(t, artifact.OutcomeDryRun, outcome.Operation.Outcome)
		assert.NotEmpty(t, outcome.Operation.RecordDigest)
		assert.NotEmpty(t, outcome.Record.Inventory)
		assert.Zero(t, e.repos["dst"].TotalCalls())
		_, found := e.repos["dst"].RecordDigestOf(exampleRef)
		assert.False(t, found)
		e.assertStagingClean()
		assert.Empty(t, e.journal.phases(), "a dry run records no lifecycle")
	})

	t.Run("repositories mock asserts Open is never called", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repos := service.NewMockRepositories(ctrl)
		caps := artifacttest.DefaultCapabilities()
		repos.EXPECT().Describe("dst").Return(service.RepositoryInfo{Alias: "dst", Capabilities: caps, Packaging: artifact.PackagingTgz}, nil)
		repos.EXPECT().Open(gomock.Any(), gomock.Any()).Times(0)
		svc, err := service.New(repos, nil, service.WithStagingRoot(t.TempDir()))
		require.NoError(t, err)

		src := t.TempDir()
		fixtures.WriteFilePackage(t, src)
		result, err := svc.Publish(context.Background(), artifact.PublishRequest{Repository: "dst", Source: src, Name: "x/y", Version: "1", DryRun: true})
		require.NoError(t, err)
		assert.Equal(t, artifact.OutcomeDryRun, result.Outcome)
	})
}

func TestPublish_PackagingFollowsRepository(t *testing.T) {
	for _, packaging := range []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip} {
		t.Run(string(packaging), func(t *testing.T) {
			e := newEnv(t, envConfig{})
			e.setInfo("dst", func(info *service.RepositoryInfo) { info.Packaging = packaging })
			e.publish("dst", e.source("hello"))
			result, err := e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: exampleRef})
			require.NoError(t, err)
			assert.Equal(t, packaging, result.Inspection.Representation.Packaging)
		})
	}
}

func TestPublish_ComponentKindReportsDefaultExclusionsAndGenericKindDoesNot(t *testing.T) {
	e := newEnv(t, envConfig{})
	src := e.source("hello")
	fixtures.WriteFile(t, src, "main.tf", "# component\n", 0o644)
	fixtures.WriteFile(t, src, ".terraform/modules.json", "{}\n", 0o644)
	fixtures.WriteFile(t, src, "terraform.tfstate", "{}\n", 0o644)

	excluded := func(outcome *service.PublishOutcome) []string {
		var messages []string
		for _, diagnostic := range outcome.Operation.Diagnostics {
			if diagnostic.Code == "excluded" {
				messages = append(messages, diagnostic.Message)
			}
		}
		return messages
	}

	request := e.publishRequest("dst", src)
	request.Kind = "component/terraform"
	request.DryRun = true
	dryRun, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeDryRun, dryRun.Operation.Outcome)
	assert.Equal(t, "component/terraform", dryRun.Record.Kind)
	assert.Equal(t, []string{"excluded .terraform (rule .terraform/**)", "excluded terraform.tfstate (rule *.tfstate)"}, excluded(dryRun),
		"the dry run shows what the component producer leaves out and why")
	for _, entry := range dryRun.Record.Inventory {
		assert.NotContains(t, entry.Path, "terraform.tfstate")
		assert.NotContains(t, entry.Path, ".terraform/")
	}

	request.DryRun = false
	published, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(t, err)
	assert.Len(t, excluded(published), 2, "a real publication reports the same exclusions")

	generic := e.publishRequest("dst", src)
	generic.DryRun = true
	plain, err := e.svc.PublishDetailed(context.Background(), generic)
	require.NoError(t, err)
	assert.Empty(t, excluded(plain), "generic files never inherit component exclusions")
	assert.Contains(t, inventoryPathsOf(&plain.Record), "terraform.tfstate")
}

func inventoryPathsOf(record *artifact.Record) []string {
	paths := make([]string, 0, len(record.Inventory))
	for index := range record.Inventory {
		paths = append(paths, record.Inventory[index].Path)
	}
	return paths
}
