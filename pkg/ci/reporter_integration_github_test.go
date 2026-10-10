package ci_test

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/ci/providers/github/ghtest"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields these tests set.
var (
	_ = schema.CIConfig{Enabled: true, AllowUnsafeForkExecution: true}
	_ = schema.CICommentsConfig{Enabled: ptr(true)}
	_ = schema.CIChecksConfig{Enabled: ptr(true)}
	_ = schema.CIResultsConfig{Enabled: ptr(true)}
	_ = schema.CISummaryConfig{Enabled: ptr(true)}
	_ = schema.CIOutputConfig{Enabled: ptr(true)}
	_ = schema.CIAnnotationsConfig{Enabled: ptr(true)}
	_ = schema.CIGroupsConfig{Mode: "off"}
)

// fullCIConfig enables every reporting surface, including the opt-in ones.
func fullCIConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{CI: schema.CIConfig{
		Enabled:  true,
		Comments: schema.CICommentsConfig{Enabled: ptr(true)},
		Checks:   schema.CIChecksConfig{Enabled: ptr(true)},
		Results:  schema.CIResultsConfig{Enabled: ptr(true)},
	}}
}

// harness wires a Reporter to the real GitHub provider talking to the fake GitHub API,
// with the generic provider registered as the local fallback.
type harness struct {
	server   *ghtest.Server
	env      ghtest.Env
	reporter ci.Reporter
	// local receives the local (generic provider) renderings.
	local *bytes.Buffer
	// stdout receives data and log-group markers.
	stdout *bytes.Buffer
	// uiStderr receives annotations; stderr captures raw add-mask commands.
	uiStderr *bytes.Buffer
	stderr   func() string
}

func newHarness(t *testing.T, cfg *schema.AtmosConfiguration, serverOpts []ghtest.Option, envOpts ...ghtest.EnvOption) *harness {
	t.Helper()

	isolateLocalEnv(t)
	uiStderr := &bytes.Buffer{}
	stdout := initIO(t, uiStderr)
	stderr := captureOSStderr(t)
	s := ghtest.NewServer(t, serverOpts...)
	env := ghtest.SetEnv(t, s, envOpts...)
	// Resolve the SHA from GITHUB_SHA rather than the surrounding git checkout.
	t.Chdir(t.TempDir())
	ghtest.RegisterProvider(t, github.NewProvider())
	ci.Register(generic.NewProvider())

	local := &bytes.Buffer{}
	return &harness{
		server:   s,
		env:      env,
		reporter: ci.NewReporter(cfg).WithOutput(local),
		local:    local,
		stdout:   stdout,
		uiStderr: uiStderr,
		stderr:   stderr,
	}
}

// commands returns workflow commands from both streams, one per line. Log
// lines the logger writes to the same stream are not workflow commands and are left out.
func (h *harness) commands() string {
	var b strings.Builder
	for _, line := range strings.Split(h.stdout.String()+h.uiStderr.String()+h.stderr(), "\n") {
		if strings.HasPrefix(line, "::") {
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// prEnv is the standard pull request run used by most tests.
func prEnv() []ghtest.EnvOption {
	return []ghtest.EnvOption{
		ghtest.WithRepository("owner/repo"),
		ghtest.WithPullRequest(42, "feature", "main"),
		ghtest.WithRunID("777"),
	}
}

// assertNothingWritten verifies that no report reached GitHub through any channel.
func (h *harness) assertNothingWritten(t *testing.T) {
	t.Helper()

	assert.Empty(t, h.server.Comments())
	assert.Empty(t, h.server.Statuses())
	assert.Empty(t, h.server.SARIFUploads())
	assert.Empty(t, ghtest.ReadFile(t, h.env.Summary))
	assert.Empty(t, ghtest.ReadFile(t, h.env.Output))
	assert.Empty(t, ghtest.ReadFile(t, h.env.EnvFile))
	assert.Empty(t, ghtest.ReadFile(t, h.env.Path))
	assert.Empty(t, h.stdout.String(), "no workflow command may reach stdout")
	assert.Empty(t, h.commands(), "no workflow command may be emitted")
}

func TestReporter_GitHubContext(t *testing.T) {
	h := newHarness(t, fullCIConfig(), nil, prEnv()...)

	c, err := h.reporter.Context()
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, github.ProviderName, c.Provider)
	assert.Equal(t, "owner/repo", c.Repository)
	assert.Equal(t, "owner", c.RepoOwner)
	assert.Equal(t, "repo", c.RepoName)
	require.NotNil(t, c.PullRequest)
	assert.Equal(t, 42, c.PullRequest.Number)
	assert.Regexp(t, `/actions/runs/777$`, c.RunURL)
	assert.False(t, c.ElevatedEvent)
}

// TestReporter_GitHubWrites drives every Reporter call against the real GitHub provider.
func TestReporter_GitHubWrites(t *testing.T) {
	const sha = "abcdef0123456789abcdef0123456789abcdef01"
	envOpts := append(prEnv(), ghtest.WithSHA(sha))

	assertGitHub := func(t *testing.T, rc ci.Receipt, err error) {
		t.Helper()
		require.NoError(t, err)
		assert.Equal(t, github.ProviderName, rc.Provider)
		assert.False(t, rc.Local)
		assert.Empty(t, rc.Gate)
	}

	t.Run("summary", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, envOpts...)
		rc, err := h.reporter.Summary("## Plan\n\n3 to add")
		assertGitHub(t, rc, err)
		assert.Contains(t, ghtest.ReadFile(t, h.env.Summary), "## Plan\n\n3 to add")
		assert.Empty(t, h.local.String(), "nothing renders locally when the provider handled the write")
	})

	t.Run("output including a multiline value", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, envOpts...)
		rc, err := h.reporter.Output("k", "v")
		assertGitHub(t, rc, err)
		rc, err = h.reporter.Output("k2", "line1\nline2")
		assertGitHub(t, rc, err)

		out := ghtest.ReadFile(t, h.env.Output)
		assert.Contains(t, out, "k=v\n")
		assert.Contains(t, out, "k2<<EOF\nline1\nline2\nEOF\n")
	})

	t.Run("env and path", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, envOpts...)
		rc, err := h.reporter.Env("TF_IN_AUTOMATION", "true")
		assertGitHub(t, rc, err)
		rc, err = h.reporter.Path("/opt/tools/bin")
		assertGitHub(t, rc, err)

		assert.Contains(t, ghtest.ReadFile(t, h.env.EnvFile), "TF_IN_AUTOMATION=true\n")
		assert.Equal(t, "/opt/tools/bin\n", ghtest.ReadFile(t, h.env.Path))
	})

	t.Run("comment with key is created then edited in place", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, envOpts...)
		const marker = "<!-- atmos:ci:plan -->"

		first, err := h.reporter.Comment(context.Background(), ci.CommentRequest{Body: "first body", Key: "plan"})
		assertGitHub(t, first, err)
		require.NotNil(t, first.Comment)
		assert.True(t, first.Comment.Created)
		assert.NotEmpty(t, first.Comment.URL)

		second, err := h.reporter.Comment(context.Background(), ci.CommentRequest{Body: "second body", Key: "plan"})
		assertGitHub(t, second, err)
		require.NotNil(t, second.Comment)
		assert.False(t, second.Comment.Created)
		assert.Equal(t, first.Comment.ID, second.Comment.ID)

		writes := h.server.Comments()
		require.Len(t, writes, 2)
		assert.False(t, writes[0].Edited)
		assert.Contains(t, writes[0].Body, marker)
		assert.Contains(t, writes[0].Body, "first body")
		assert.True(t, writes[1].Edited)
		assert.Contains(t, writes[1].Body, marker)
		assert.Contains(t, writes[1].Body, "second body")
		assert.Equal(t, "owner", writes[0].Owner)
		assert.Equal(t, "repo", writes[0].Repo)
		assert.Equal(t, 42, writes[0].Number)

		current := h.server.CommentsFor("owner", "repo", 42)
		require.Len(t, current, 1)
		assert.Contains(t, current[0].Body, "second body")
		assert.NotContains(t, current[0].Body, "first body")
	})

	t.Run("comment without key always creates", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, envOpts...)
		for _, body := range []string{"one", "two"} {
			rc, err := h.reporter.Comment(context.Background(), ci.CommentRequest{Body: body})
			assertGitHub(t, rc, err)
			require.NotNil(t, rc.Comment)
			assert.True(t, rc.Comment.Created)
		}
		assert.Len(t, h.server.CommentsFor("owner", "repo", 42), 2)
	})

	t.Run("check then update check", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, envOpts...)
		runURL, err := runURLOf(h)
		require.NoError(t, err)

		rc, err := h.reporter.Check(context.Background(), ci.CheckRequest{Name: "atmos/plan", State: ci.CheckRunStateInProgress, Description: "planning"})
		assertGitHub(t, rc, err)
		require.NotNil(t, rc.Check)
		assert.Equal(t, "atmos/plan", rc.Check.Name)

		rc, err = h.reporter.UpdateCheck(context.Background(), ci.CheckRequest{Name: "atmos/plan", State: ci.CheckRunStateSuccess, Description: "3 to add"})
		assertGitHub(t, rc, err)
		require.NotNil(t, rc.Check)
		assert.Equal(t, ci.CheckRunStateSuccess, rc.Check.Status)

		statuses := h.server.Statuses()
		require.Len(t, statuses, 2)
		// The commit status API has no in-progress state, so it reports pending.
		assert.Equal(t, "pending", statuses[0].State)
		assert.Equal(t, "planning", statuses[0].Description)
		assert.Equal(t, "success", statuses[1].State)
		assert.Equal(t, "3 to add", statuses[1].Description)
		for _, st := range statuses {
			assert.Equal(t, "atmos/plan", st.Context)
			assert.Equal(t, runURL, st.TargetURL)
			assert.Equal(t, sha, st.SHA)
			assert.Equal(t, "owner", st.Owner)
			assert.Equal(t, "repo", st.Repo)
		}
	})

	t.Run("check honors an explicit details URL", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, envOpts...)
		_, err := h.reporter.Check(context.Background(), ci.CheckRequest{Name: "atmos/plan", URL: "https://example.test/details"})
		require.NoError(t, err)
		statuses := h.server.Statuses()
		require.Len(t, statuses, 1)
		assert.Equal(t, "https://example.test/details", statuses[0].TargetURL)
		assert.Equal(t, "pending", statuses[0].State, "an empty state means pending")
	})

	t.Run("mask emits an unmasked add-mask on stderr", func(t *testing.T) {
		t.Cleanup(atmosio.Reset)
		h := newHarness(t, fullCIConfig(), nil, envOpts...)
		const secret = "mask-me-integration-value"
		rc, err := h.reporter.Mask(secret)
		require.NoError(t, err)
		assert.Equal(t, github.ProviderName, rc.Provider)
		assert.False(t, rc.Local)
		assert.Contains(t, h.stderr(), "::add-mask::"+secret, "the command must carry the secret, not the masker's placeholder")
		assert.Empty(t, h.stdout.String(), "workflow commands stay off the data channel")
	})

	t.Run("annotations use stderr and groups bracket stdout", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, envOpts...)
		rc, err := h.reporter.Annotate(ci.Annotation{Path: "main.tf", StartLine: 12, Level: ci.AnnotationError, Message: "bucket is public"})
		assertGitHub(t, rc, err)
		end, rc, err := h.reporter.Group("Deploy vpc")
		assertGitHub(t, rc, err)
		end()

		assert.Equal(t, "::error file=main.tf,line=12::bucket is public\n", h.uiStderr.String())
		assert.Equal(t, "::group::Deploy vpc\n::endgroup::\n", h.stdout.String())
	})

	t.Run("sarif uploads to code scanning", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, envOpts...)
		report := ci.SARIFReport{Body: []byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"tfsec"}}}]}`), Category: "tfsec"}
		rc, err := h.reporter.SARIF(context.Background(), report)
		assertGitHub(t, rc, err)

		uploads := h.server.SARIFUploads()
		require.Len(t, uploads, 1)
		assert.Equal(t, sha, uploads[0].CommitSHA)
		assert.Equal(t, "owner", uploads[0].Owner)
		assert.Equal(t, "repo", uploads[0].Repo)
		assert.Contains(t, uploads[0].SARIF, `"id":"tfsec/"`, "the category is carried in automationDetails.id")
	})
}

// runURLOf returns the run URL the reporter resolved for the harness.
func runURLOf(h *harness) (string, error) {
	c, err := h.reporter.Context()
	if err != nil {
		return "", err
	}
	return c.RunURL, nil
}

// gateCall is one Reporter call that is gated by a single configuration switch.
type gateCall struct {
	name string
	// feature is the gate reported when the switch is off.
	feature ci.Feature
	// disable turns the switch off on a config that otherwise enables everything.
	disable func(*schema.CIConfig)
	call    func(r ci.Reporter) (ci.Receipt, error)
	// want is rendered into the local writer instead of reaching GitHub.
	want string
}

func gateCalls() []gateCall {
	ctx := context.Background()
	return []gateCall{
		{
			name: "comments", feature: ci.FeatureComments,
			disable: func(c *schema.CIConfig) { c.Comments.Enabled = ptr(false) },
			call: func(r ci.Reporter) (ci.Receipt, error) {
				return r.Comment(ctx, ci.CommentRequest{Body: "gated body", Key: "plan"})
			},
			want: "PR comment preview",
		},
		{
			name: "checks", feature: ci.FeatureChecks,
			disable: func(c *schema.CIConfig) { c.Checks.Enabled = ptr(false) },
			call: func(r ci.Reporter) (ci.Receipt, error) {
				return r.Check(ctx, ci.CheckRequest{Name: "atmos/plan"})
			},
			want: "Check run created: atmos/plan [pending]",
		},
		{
			name: "update check", feature: ci.FeatureChecks,
			disable: func(c *schema.CIConfig) { c.Checks.Enabled = ptr(false) },
			call: func(r ci.Reporter) (ci.Receipt, error) {
				return r.UpdateCheck(ctx, ci.CheckRequest{Name: "atmos/plan", State: ci.CheckRunStateSuccess})
			},
			want: "Check run completed: atmos/plan [success]",
		},
		{
			name: "summary", feature: ci.FeatureSummary,
			disable: func(c *schema.CIConfig) { c.Summary.Enabled = ptr(false) },
			call:    func(r ci.Reporter) (ci.Receipt, error) { return r.Summary("# Gated heading") },
			want:    "Gated heading",
		},
		{
			name: "output", feature: ci.FeatureOutput,
			disable: func(c *schema.CIConfig) { c.Output.Enabled = ptr(false) },
			call:    func(r ci.Reporter) (ci.Receipt, error) { return r.Output("k", "v") },
			want:    "k=v",
		},
		{
			name: "env", feature: ci.FeatureOutput,
			disable: func(c *schema.CIConfig) { c.Output.Enabled = ptr(false) },
			call:    func(r ci.Reporter) (ci.Receipt, error) { return r.Env("FOO", "bar") },
			want:    "export FOO=",
		},
		{
			name: "path", feature: ci.FeatureOutput,
			disable: func(c *schema.CIConfig) { c.Output.Enabled = ptr(false) },
			call:    func(r ci.Reporter) (ci.Receipt, error) { return r.Path("/opt/bin") },
			want:    "export PATH=",
		},
		{
			name: "annotations", feature: ci.FeatureAnnotations,
			disable: func(c *schema.CIConfig) { c.Annotations.Enabled = ptr(false) },
			call: func(r ci.Reporter) (ci.Receipt, error) {
				return r.Annotate(ci.Annotation{Path: "main.tf", StartLine: 3, Level: ci.AnnotationWarning, Message: "gated finding"})
			},
			want: "main.tf:3",
		},
		{
			name: "results", feature: ci.FeatureResults,
			disable: func(c *schema.CIConfig) { c.Results.Enabled = ptr(false) },
			call: func(r ci.Reporter) (ci.Receipt, error) {
				return r.SARIF(ctx, ci.SARIFReport{Body: []byte(`{"runs":[]}`), Category: "tfsec"})
			},
			want: "not uploaded",
		},
		{
			name: "groups", feature: ci.FeatureGroups,
			disable: func(c *schema.CIConfig) { c.Groups.Mode = "off" },
			call: func(r ci.Reporter) (ci.Receipt, error) {
				end, rc, err := r.Group("Gated group")
				end()
				return rc, err
			},
			want: "Gated group",
		},
	}
}

// TestReporter_GatedByOwnSwitch verifies that turning off one switch skips GitHub for exactly that
// feature, renders locally instead, and reports the switch in the receipt.
func TestReporter_GatedByOwnSwitch(t *testing.T) {
	for _, gc := range gateCalls() {
		t.Run(gc.name, func(t *testing.T) {
			cfg := fullCIConfig()
			gc.disable(&cfg.CI)
			h := newHarness(t, cfg, nil, prEnv()...)

			rc, err := gc.call(h.reporter)

			require.NoError(t, err)
			assert.True(t, rc.Local)
			assert.Equal(t, gc.feature, rc.Gate)
			assert.Equal(t, generic.ProviderName, rc.Provider)
			assert.Contains(t, h.local.String(), gc.want)
			h.assertNothingWritten(t)
		})
	}
}

// TestReporter_GatedByMasterSwitch verifies that ci.enabled=false gates every feature and that the
// receipt names ci.enabled, the switch that is actually off, rather than the per-feature key (every
// per-feature switch folds the master switch into its answer). Mask never reports a gate: it
// always registers the secret locally.
func TestReporter_GatedByMasterSwitch(t *testing.T) {
	for _, gc := range gateCalls() {
		t.Run(gc.name, func(t *testing.T) {
			cfg := fullCIConfig()
			cfg.CI.Enabled = false
			h := newHarness(t, cfg, nil, prEnv()...)

			rc, err := gc.call(h.reporter)

			require.NoError(t, err)
			assert.True(t, rc.Local)
			assert.Equal(t, ci.FeatureEnabled, rc.Gate, "the master switch is the one that is off")
			assert.True(t, rc.Gated())
			assert.Contains(t, h.local.String(), gc.want)
			h.assertNothingWritten(t)
		})
	}

	t.Run("mask", func(t *testing.T) {
		t.Cleanup(atmosio.Reset)
		cfg := fullCIConfig()
		cfg.CI.Enabled = false
		h := newHarness(t, cfg, nil, prEnv()...)

		rc, err := h.reporter.Mask("master-switch-secret-7Hd2")

		require.NoError(t, err)
		assert.True(t, rc.Local)
		assert.Empty(t, rc.Gate)
		h.assertNothingWritten(t)
	})
}

// TestReporter_ForkGate verifies that on an elevated event a fork pull request never posts unless
// ci.allow_unsafe_fork_execution is set, while a same-repository pull request posts normally.
func TestReporter_ForkGate(t *testing.T) {
	forkEnv := func() []ghtest.EnvOption {
		return []ghtest.EnvOption{
			ghtest.WithRepository("owner/repo"),
			ghtest.WithEvent("pull_request_target", map[string]any{"action": "synchronize"}),
			ghtest.WithForkPullRequest(42, "fork-branch", "main"),
		}
	}
	sameRepoEnv := func() []ghtest.EnvOption {
		return []ghtest.EnvOption{
			ghtest.WithRepository("owner/repo"),
			ghtest.WithEvent("pull_request_target", map[string]any{"action": "synchronize"}),
			ghtest.WithPullRequest(42, "feature", "main"),
		}
	}
	calls := []struct {
		name string
		call func(r ci.Reporter) (ci.Receipt, error)
		want string
		sent func(t *testing.T, h *harness)
	}{
		{
			name: "comment",
			call: func(r ci.Reporter) (ci.Receipt, error) {
				return r.Comment(context.Background(), ci.CommentRequest{Body: "fork body", Key: "plan"})
			},
			want: "PR comment preview",
			sent: func(t *testing.T, h *harness) {
				require.Len(t, h.server.Comments(), 1)
				assert.Contains(t, h.server.Comments()[0].Body, "fork body")
			},
		},
		{
			name: "check",
			call: func(r ci.Reporter) (ci.Receipt, error) {
				return r.Check(context.Background(), ci.CheckRequest{Name: "atmos/plan"})
			},
			want: "Check run created: atmos/plan",
			sent: func(t *testing.T, h *harness) {
				require.Len(t, h.server.Statuses(), 1)
				assert.Equal(t, "atmos/plan", h.server.Statuses()[0].Context)
			},
		},
		{
			name: "update check",
			call: func(r ci.Reporter) (ci.Receipt, error) {
				return r.UpdateCheck(context.Background(), ci.CheckRequest{Name: "atmos/plan", State: ci.CheckRunStateFailure})
			},
			want: "Check run failed: atmos/plan",
			sent: func(t *testing.T, h *harness) {
				require.Len(t, h.server.Statuses(), 1)
				assert.Equal(t, "failure", h.server.Statuses()[0].State)
			},
		},
	}

	for _, tc := range calls {
		t.Run(tc.name+" is blocked for a fork pull request on an elevated event", func(t *testing.T) {
			h := newHarness(t, fullCIConfig(), nil, forkEnv()...)
			c, err := h.reporter.Context()
			require.NoError(t, err)
			assert.True(t, c.ElevatedEvent)
			require.NotNil(t, c.PullRequest)
			assert.True(t, c.PullRequest.Fork)

			rc, err := tc.call(h.reporter)

			require.NoError(t, err)
			assert.Equal(t, ci.FeatureForkGate, rc.Gate)
			assert.True(t, rc.Local)
			assert.Equal(t, generic.ProviderName, rc.Provider)
			assert.Contains(t, h.local.String(), tc.want)
			h.assertNothingWritten(t)
		})

		t.Run(tc.name+" is sent for a same-repository pull request on an elevated event", func(t *testing.T) {
			h := newHarness(t, fullCIConfig(), nil, sameRepoEnv()...)
			c, err := h.reporter.Context()
			require.NoError(t, err)
			assert.True(t, c.ElevatedEvent)
			require.NotNil(t, c.PullRequest)
			assert.False(t, c.PullRequest.Fork)

			rc, err := tc.call(h.reporter)

			require.NoError(t, err)
			assert.Empty(t, rc.Gate)
			assert.False(t, rc.Local)
			assert.Equal(t, github.ProviderName, rc.Provider)
			assert.Empty(t, h.local.String())
			tc.sent(t, h)
		})

		t.Run(tc.name+" is sent for a fork pull request when unsafe fork execution is allowed", func(t *testing.T) {
			cfg := fullCIConfig()
			cfg.CI.AllowUnsafeForkExecution = true
			h := newHarness(t, cfg, nil, forkEnv()...)

			rc, err := tc.call(h.reporter)

			require.NoError(t, err)
			assert.Empty(t, rc.Gate)
			assert.False(t, rc.Local)
			assert.Equal(t, github.ProviderName, rc.Provider)
			assert.Empty(t, h.local.String())
			tc.sent(t, h)
		})
	}
}

// TestReporter_ErrorsKeepSentinels verifies provider failures reach the caller with their sentinels.
func TestReporter_ErrorsKeepSentinels(t *testing.T) {
	ctx := context.Background()

	t.Run("comment post failure", func(t *testing.T) {
		opts := []ghtest.Option{ghtest.WithFailure(http.MethodPost, "/repos/owner/repo/issues/42/comments", http.StatusInternalServerError, "boom")}
		h := newHarness(t, fullCIConfig(), opts, prEnv()...)

		rc, err := h.reporter.Comment(ctx, ci.CommentRequest{Body: "body", Key: "plan"})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCICommentPostFailed)
		assert.Nil(t, rc.Comment)
		assert.Empty(t, h.server.Comments())
	})

	t.Run("check create failure", func(t *testing.T) {
		opts := []ghtest.Option{ghtest.WithFailure(http.MethodPost, "/repos/owner/repo/statuses", http.StatusInternalServerError, "boom")}
		h := newHarness(t, fullCIConfig(), opts, prEnv()...)

		rc, err := h.reporter.Check(ctx, ci.CheckRequest{Name: "atmos/plan"})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCICheckRunCreateFailed)
		assert.Nil(t, rc.Check)
		assert.Empty(t, h.server.Statuses())
	})

	t.Run("check update failure", func(t *testing.T) {
		opts := []ghtest.Option{ghtest.WithFailure(http.MethodPost, "/repos/owner/repo/statuses", http.StatusInternalServerError, "boom")}
		h := newHarness(t, fullCIConfig(), opts, prEnv()...)

		_, err := h.reporter.UpdateCheck(ctx, ci.CheckRequest{Name: "atmos/plan", State: ci.CheckRunStateSuccess})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCICheckRunUpdateFailed)
	})

	t.Run("sarif upload failure", func(t *testing.T) {
		opts := []ghtest.Option{ghtest.WithFailure(http.MethodPost, "/repos/owner/repo/code-scanning/sarifs", http.StatusForbidden, "no GHAS")}
		h := newHarness(t, fullCIConfig(), opts, prEnv()...)

		_, err := h.reporter.SARIF(ctx, ci.SARIFReport{Body: []byte(`{"runs":[]}`), Category: "tfsec"})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCISARIFUploadFailed)
	})

	t.Run("unknown pull request when the target is pr", func(t *testing.T) {
		// A push event carries no pull request, and the request names none.
		h := newHarness(t, fullCIConfig(), nil, ghtest.WithRepository("owner/repo"))

		_, err := h.reporter.Comment(ctx, ci.CommentRequest{Body: "body", Key: "plan", PR: 0, Target: ci.CommentTargetPR})

		require.ErrorIs(t, err, errUtils.ErrCIPullRequestUnknown)
		assert.Empty(t, h.server.Comments())
	})

	t.Run("explicit pull request overrides the missing context", func(t *testing.T) {
		h := newHarness(t, fullCIConfig(), nil, ghtest.WithRepository("owner/repo"))

		rc, err := h.reporter.Comment(ctx, ci.CommentRequest{Body: "body", Key: "plan", PR: 9})

		require.NoError(t, err)
		assert.False(t, rc.Local)
		require.Len(t, h.server.CommentsFor("owner", "repo", 9), 1)
		assert.Empty(t, h.server.CommentsFor("owner", "repo", 42))
	})
}
