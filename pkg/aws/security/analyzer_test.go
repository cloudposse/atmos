package security

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockAIClient implements registry.Client for testing.
type mockAIClient struct {
	response string
	err      error
}

func (m *mockAIClient) SendMessage(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

func (m *mockAIClient) SendMessageWithTools(_ context.Context, _ string, _ []tools.Tool) (*types.Response, error) {
	return &types.Response{Content: m.response}, m.err
}

func (m *mockAIClient) SendMessageWithHistory(_ context.Context, _ []types.Message) (string, error) {
	return m.response, m.err
}

func (m *mockAIClient) SendMessageWithToolsAndHistory(_ context.Context, _ []types.Message, _ []tools.Tool) (*types.Response, error) {
	return &types.Response{Content: m.response}, m.err
}

func (m *mockAIClient) SendMessageWithSystemPromptAndTools(_ context.Context, _ string, _ string, _ []types.Message, _ []tools.Tool) (*types.Response, error) {
	return &types.Response{Content: m.response}, m.err
}

func (m *mockAIClient) GetModel() string {
	return "test-model"
}

func (m *mockAIClient) GetMaxTokens() int {
	return 4096
}

func TestAnalyzeFinding_Success(t *testing.T) {
	mockResponse := `**Root Cause:** The S3 bucket does not have server-side encryption enabled in the Terraform configuration.

**Remediation:** Add an aws_s3_bucket_server_side_encryption_configuration resource to the component source code.

**Deploy:** ` + "`atmos terraform apply s3-bucket -s ue2-dev`" + `

**Risk:** Low - enabling encryption is a non-destructive change.`

	client := &mockAIClient{response: mockResponse}
	analyzer := newFindingAnalyzerWithClient(client, &schema.AtmosConfiguration{})

	finding := &Finding{
		ID:          "finding-001",
		Title:       "S3 bucket without encryption",
		Description: "The S3 bucket does not have encryption enabled.",
		Severity:    SeverityHigh,
		Source:      SourceSecurityHub,
		ResourceARN: "arn:aws:s3:::my-bucket",
		Mapping: &ComponentMapping{
			Component: "s3-bucket",
			Stack:     "ue2-dev",
			Mapped:    true,
		},
	}

	remediation, err := analyzer.AnalyzeFinding(context.Background(), finding, "resource \"aws_s3_bucket\" {}", "component: s3-bucket")
	require.NoError(t, err)
	assert.NotNil(t, remediation)
	assert.Contains(t, remediation.Description, "Root Cause:")
	assert.Contains(t, remediation.RootCause, "S3 bucket")
	assert.Equal(t, "atmos terraform apply s3-bucket -s ue2-dev", remediation.DeployCommand)
	assert.Equal(t, "low", remediation.RiskLevel)
}

func TestAnalyzeFinding_AIError(t *testing.T) {
	client := &mockAIClient{err: errors.New("AI provider unavailable")}
	analyzer := newFindingAnalyzerWithClient(client, &schema.AtmosConfiguration{})

	finding := &Finding{
		ID:       "finding-002",
		Title:    "Test finding",
		Severity: SeverityMedium,
	}

	remediation, err := analyzer.AnalyzeFinding(context.Background(), finding, "", "")
	assert.Error(t, err)
	assert.Nil(t, remediation)
	assert.Contains(t, err.Error(), "AI analysis failed")
	assert.Contains(t, err.Error(), "AI provider unavailable")
}

func TestBuildAnalysisPrompt(t *testing.T) {
	finding := &Finding{
		ID:                 "finding-003",
		Title:              "Unencrypted EBS volume",
		Description:        "EBS volume is not encrypted.",
		Severity:           SeverityCritical,
		Source:             SourceInspector,
		ResourceARN:        "arn:aws:ec2:us-east-1:123456789012:volume/vol-abc123",
		ResourceType:       "AwsEc2Volume",
		ComplianceStandard: "CIS AWS 1.4",
		Mapping: &ComponentMapping{
			Component:     "ebs-volume",
			Stack:         "ue1-prod",
			ComponentPath: "/components/terraform/ebs-volume",
			Mapped:        true,
		},
	}

	prompt := buildAnalysisPrompt(finding, "resource \"aws_ebs_volume\" {}", "component: ebs-volume\nstack: ue1-prod")

	// Verify prompt contains finding details.
	assert.Contains(t, prompt, "finding-003")
	assert.Contains(t, prompt, "Unencrypted EBS volume")
	assert.Contains(t, prompt, "CRITICAL")
	assert.Contains(t, prompt, "inspector")
	assert.Contains(t, prompt, "AwsEc2Volume")
	assert.Contains(t, prompt, "CIS AWS 1.4")

	// Verify prompt contains component source.
	assert.Contains(t, prompt, "aws_ebs_volume")

	// Verify prompt contains stack config.
	assert.Contains(t, prompt, "component: ebs-volume")
	assert.Contains(t, prompt, "stack: ue1-prod")

	// Verify prompt contains structured analysis request.
	assert.Contains(t, prompt, "Analyze this AWS security finding")
	assert.Contains(t, prompt, "structured remediation")
}

func TestBuildAnalysisPrompt_NoMapping(t *testing.T) {
	finding := &Finding{
		ID:          "finding-004",
		Title:       "Open security group",
		Description: "Security group allows unrestricted ingress.",
		Severity:    SeverityHigh,
		Source:      SourceSecurityHub,
		ResourceARN: "arn:aws:ec2:us-east-1:123456789012:security-group/sg-123",
	}

	prompt := buildAnalysisPrompt(finding, "", "")

	// Should not contain component mapping section.
	assert.NotContains(t, prompt, "## Atmos Component Mapping")
	assert.NotContains(t, prompt, "## Component Source Code")
	assert.NotContains(t, prompt, "## Stack Configuration")
}

func TestParseRemediationResponse(t *testing.T) {
	tests := []struct {
		name           string
		response       string
		expectedRoot   string
		expectedDeploy string
		expectedRisk   string
	}{
		{
			name: "full structured response with bold headers",
			response: `**Root Cause:** Missing encryption configuration in the S3 bucket resource.

**Remediation:** Add server-side encryption block to the Terraform config.

**Deploy:** ` + "`atmos terraform apply s3-bucket -s ue2-dev`" + `

**Risk:** Low - non-destructive change.`,
			expectedRoot:   "Missing encryption configuration in the S3 bucket resource.",
			expectedDeploy: "atmos terraform apply s3-bucket -s ue2-dev",
			expectedRisk:   "low",
		},
		{
			name: "plain headers",
			response: `Root Cause: IAM policy is too permissive.

Remediation: Restrict the IAM policy to specific resources.

Deploy: atmos terraform apply iam-role -s ue1-prod

Risk: High - changing IAM policies can break access.`,
			expectedRoot:   "IAM policy is too permissive.",
			expectedDeploy: "atmos terraform apply iam-role -s ue1-prod",
			expectedRisk:   "high",
		},
		{
			name:           "unstructured response",
			response:       "This finding indicates a misconfigured security group. You should restrict ingress rules.",
			expectedRoot:   "",
			expectedDeploy: "atmos terraform apply sg -s ue2-staging",
			expectedRisk:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finding := &Finding{
				ID: "test-finding",
				Mapping: &ComponentMapping{
					Component: "sg",
					Stack:     "ue2-staging",
					Mapped:    true,
				},
			}

			remediation := parseRemediationResponse(tt.response, finding)

			assert.Equal(t, tt.response, remediation.Description)

			if tt.expectedRoot != "" {
				assert.Equal(t, tt.expectedRoot, remediation.RootCause)
			}

			assert.Equal(t, tt.expectedDeploy, remediation.DeployCommand)

			if tt.expectedRisk != "" {
				assert.Equal(t, tt.expectedRisk, remediation.RiskLevel)
			}
		})
	}
}

func TestAnalyzeFindings_SkipsUnmapped(t *testing.T) {
	callCount := 0
	client := &mockAIClient{
		response: "**Root Cause:** Test\n\n**Risk:** Low",
	}

	// Override SendMessage to count calls.
	originalSend := client.response
	analyzer := &aiAnalyzer{
		client:      client,
		atmosConfig: &schema.AtmosConfiguration{},
	}

	// Override readFile to avoid filesystem access.
	originalReadFile := readFile
	readFile = func(_ string) ([]byte, error) {
		return []byte("resource \"aws_s3_bucket\" {}"), nil
	}
	t.Cleanup(func() { readFile = originalReadFile })

	// Create a counting wrapper.
	countingClient := &countingMockClient{
		inner:     client,
		callCount: &callCount,
	}
	analyzer.client = countingClient
	_ = originalSend

	findings := []Finding{
		{
			ID:       "mapped-finding",
			Title:    "Mapped finding",
			Severity: SeverityHigh,
			Mapping: &ComponentMapping{
				Component:     "vpc",
				Stack:         "ue2-dev",
				ComponentPath: "/components/terraform/vpc",
				Mapped:        true,
			},
		},
		{
			ID:       "unmapped-finding",
			Title:    "Unmapped finding",
			Severity: SeverityMedium,
			Mapping: &ComponentMapping{
				Mapped: false,
			},
		},
		{
			ID:       "nil-mapping-finding",
			Title:    "No mapping at all",
			Severity: SeverityLow,
			// Mapping is nil.
		},
	}

	result, err := analyzer.AnalyzeFindings(context.Background(), findings)
	require.NoError(t, err)
	assert.Len(t, result, 3)

	// Only the mapped finding should have been sent to AI.
	assert.Equal(t, 1, *countingClient.callCount)

	// Mapped finding should have remediation.
	assert.NotNil(t, result[0].Remediation)

	// Unmapped findings should not have remediation.
	assert.Nil(t, result[1].Remediation)
	assert.Nil(t, result[2].Remediation)
}

// countingMockClient wraps a mockAIClient and counts SendMessage calls.
type countingMockClient struct {
	inner     *mockAIClient
	callCount *int
}

func (c *countingMockClient) SendMessage(ctx context.Context, message string) (string, error) {
	*c.callCount++
	return c.inner.SendMessage(ctx, message)
}

func (c *countingMockClient) SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error) {
	return c.inner.SendMessageWithTools(ctx, message, availableTools)
}

func (c *countingMockClient) SendMessageWithHistory(ctx context.Context, messages []types.Message) (string, error) {
	return c.inner.SendMessageWithHistory(ctx, messages)
}

func (c *countingMockClient) SendMessageWithToolsAndHistory(ctx context.Context, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	return c.inner.SendMessageWithToolsAndHistory(ctx, messages, availableTools)
}

func (c *countingMockClient) SendMessageWithSystemPromptAndTools(ctx context.Context, systemPrompt string, atmosMemory string, messages []types.Message, availableTools []tools.Tool) (*types.Response, error) {
	return c.inner.SendMessageWithSystemPromptAndTools(ctx, systemPrompt, atmosMemory, messages, availableTools)
}

func (c *countingMockClient) GetModel() string {
	return c.inner.GetModel()
}

func (c *countingMockClient) GetMaxTokens() int {
	return c.inner.GetMaxTokens()
}

func TestReadComponentSource(t *testing.T) {
	// Override readFile for testing.
	originalReadFile := readFile
	t.Cleanup(func() { readFile = originalReadFile })

	t.Run("empty path returns empty string", func(t *testing.T) {
		result := readComponentSource("")
		assert.Empty(t, result)
	})

	t.Run("reads main.tf content", func(t *testing.T) {
		readFile = func(path string) ([]byte, error) {
			if path == "/components/terraform/vpc/main.tf" {
				return []byte("resource \"aws_vpc\" \"main\" {}"), nil
			}
			return nil, os.ErrNotExist
		}

		result := readComponentSource("/components/terraform/vpc")
		assert.Equal(t, "resource \"aws_vpc\" \"main\" {}", result)
	})

	t.Run("returns empty on file not found", func(t *testing.T) {
		readFile = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}

		result := readComponentSource("/nonexistent/path")
		assert.Empty(t, result)
	})

	t.Run("truncates large files", func(t *testing.T) {
		largeContent := make([]byte, 15000)
		for i := range largeContent {
			largeContent[i] = 'a'
		}
		readFile = func(_ string) ([]byte, error) {
			return largeContent, nil
		}

		result := readComponentSource("/components/terraform/large")
		assert.Contains(t, result, "... (truncated)")
		assert.Less(t, len(result), 15000)
	})
}

func TestFormatStackInfo(t *testing.T) {
	t.Run("nil mapping returns empty", func(t *testing.T) {
		result := formatStackInfo(nil)
		assert.Empty(t, result)
	})

	t.Run("formats mapping with all fields", func(t *testing.T) {
		mapping := &ComponentMapping{
			Component:  "vpc",
			Stack:      "ue2-dev",
			Workspace:  "dev",
			Confidence: ConfidenceExact,
			Method:     "tag",
		}

		result := formatStackInfo(mapping)
		assert.Contains(t, result, "component: vpc")
		assert.Contains(t, result, "stack: ue2-dev")
		assert.Contains(t, result, "workspace: dev")
		assert.Contains(t, result, "confidence: exact")
		assert.Contains(t, result, "method: tag")
	})

	t.Run("omits workspace when empty", func(t *testing.T) {
		mapping := &ComponentMapping{
			Component:  "s3",
			Stack:      "ue1-prod",
			Confidence: ConfidenceHigh,
			Method:     "state",
		}

		result := formatStackInfo(mapping)
		assert.NotContains(t, result, "workspace:")
	})
}

func TestExtractSection(t *testing.T) {
	text := `**Root Cause:** The bucket is public.

**Remediation:** Set the bucket ACL to private.

**Deploy:** atmos terraform apply s3 -s ue2-dev

**Risk:** Medium`

	rootCause := extractSection(text, "**Root Cause:**")
	assert.Equal(t, "The bucket is public.", rootCause)

	deploy := extractSection(text, "**Deploy:**")
	assert.Equal(t, "atmos terraform apply s3 -s ue2-dev", deploy)

	risk := extractSection(text, "**Risk:**")
	assert.Equal(t, "Medium", risk)

	missing := extractSection(text, "**Missing:**")
	assert.Empty(t, missing)
}

func TestNormalizeRiskLevel(t *testing.T) {
	assert.Equal(t, "low", normalizeRiskLevel("Low - non-destructive"))
	assert.Equal(t, "medium", normalizeRiskLevel("Medium risk"))
	assert.Equal(t, "high", normalizeRiskLevel("HIGH - critical change"))
	assert.Equal(t, "minimal", normalizeRiskLevel("minimal"))
}

func TestExtractAtmosCommand(t *testing.T) {
	assert.Equal(t, "atmos terraform apply vpc -s ue2-dev", extractAtmosCommand("`atmos terraform apply vpc -s ue2-dev`"))
	assert.Equal(t, "atmos terraform apply s3 -s ue1-prod", extractAtmosCommand("Run:\natmos terraform apply s3 -s ue1-prod\nto deploy"))
	assert.Equal(t, "just some text", extractAtmosCommand("just some text"))
}

func TestSkillPromptEmbedded(t *testing.T) {
	// Verify the skill prompt is embedded and contains key instructions.
	assert.NotEmpty(t, skillPrompt)
	assert.Contains(t, skillPrompt, "### Root Cause")
	assert.Contains(t, skillPrompt, "### Steps")
	assert.Contains(t, skillPrompt, "### Code Changes")
	assert.Contains(t, skillPrompt, "### Stack Changes")
	assert.Contains(t, skillPrompt, "### Deploy")
	assert.Contains(t, skillPrompt, "### Risk")
	assert.Contains(t, skillPrompt, "### References")
}

func TestParseNumberedList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "numbered list",
			input:    "1. First step\n2. Second step\n3. Third step",
			expected: []string{"First step", "Second step", "Third step"},
		},
		{
			name:     "bullet list",
			input:    "- First item\n- Second item",
			expected: []string{"First item", "Second item"},
		},
		{
			name:     "asterisk list",
			input:    "* Item A\n* Item B",
			expected: []string{"Item A", "Item B"},
		},
		{
			name:     "mixed with blank lines",
			input:    "1. First\n\n2. Second\n\n3. Third",
			expected: []string{"First", "Second", "Third"},
		},
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:     "no list format",
			input:    "Just plain text\nMore text",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseListItems(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseReferenceList(t *testing.T) {
	input := "- https://docs.aws.amazon.com/s3\n- CIS AWS 1.4 Control 2.1\n- https://example.com"
	refs := parseListItems(input)
	require.Len(t, refs, 3)
	assert.Equal(t, "https://docs.aws.amazon.com/s3", refs[0])
	assert.Equal(t, "CIS AWS 1.4 Control 2.1", refs[1])
}

func TestParseRemediationResponse_StructuredFormat(t *testing.T) {
	response := `### Root Cause

The S3 bucket does not have versioning enabled because the component does not set the versioning variable.

### Steps

1. Add versioning_enabled variable to the stack configuration
2. Apply the change with atmos

### Code Changes

No code changes needed — versioning is controlled by a stack variable.

### Stack Changes

` + "```yaml\ncomponents:\n  terraform:\n    s3-bucket:\n      vars:\n        versioning_enabled: true\n```" + `

### Deploy

` + "```\natmos terraform apply s3-bucket -s prod-us-east-1\n```" + `

### Risk

low

### References

- https://docs.aws.amazon.com/AmazonS3/latest/userguide/Versioning.html
- CIS AWS Foundations Benchmark v1.4 - Control 2.1.1`

	finding := &Finding{
		ID: "test-001",
		Mapping: &ComponentMapping{
			Component: "s3-bucket",
			Stack:     "prod-us-east-1",
			Mapped:    true,
		},
	}

	remediation := parseRemediationResponse(response, finding)

	assert.Contains(t, remediation.RootCause, "versioning enabled")
	require.Len(t, remediation.Steps, 2)
	assert.Equal(t, "Add versioning_enabled variable to the stack configuration", remediation.Steps[0])
	assert.Contains(t, remediation.StackChanges, "versioning_enabled: true")
	assert.Equal(t, "atmos terraform apply s3-bucket -s prod-us-east-1", remediation.DeployCommand)
	assert.Equal(t, "low", remediation.RiskLevel)
	require.Len(t, remediation.References, 2)
	assert.Contains(t, remediation.References[0], "docs.aws.amazon.com")
}

func TestParseRemediationResponse_FallbackFormat(t *testing.T) {
	// Old format with bold markers still works.
	response := `**Root Cause:** Missing encryption config.

**Remediation:** Add encryption.

**Deploy:** ` + "`atmos terraform apply vpc -s prod`" + `

**Risk:** Low`

	finding := &Finding{
		ID: "test-002",
		Mapping: &ComponentMapping{
			Component: "vpc",
			Stack:     "prod",
			Mapped:    true,
		},
	}

	remediation := parseRemediationResponse(response, finding)

	assert.Contains(t, remediation.RootCause, "Missing encryption")
	assert.Equal(t, "atmos terraform apply vpc -s prod", remediation.DeployCommand)
	assert.Equal(t, "low", remediation.RiskLevel)
}

func TestRemediationSchema_JSONRoundTrip(t *testing.T) {
	// Verify the schema survives JSON round-trip with all fields.
	remediation := Remediation{
		Description:   "Fix S3 versioning",
		RootCause:     "Versioning not enabled",
		Steps:         []string{"Update stack vars", "Apply change"},
		CodeChanges:   []CodeChange{{FilePath: "main.tf", Before: "old", After: "new"}},
		StackChanges:  "vars:\n  versioning_enabled: true",
		DeployCommand: "atmos terraform apply s3-bucket -s prod",
		RiskLevel:     "low",
		References:    []string{"https://docs.aws.amazon.com/s3"},
	}

	data, err := json.Marshal(remediation)
	require.NoError(t, err)

	var decoded Remediation
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, remediation.Description, decoded.Description)
	assert.Equal(t, remediation.RootCause, decoded.RootCause)
	assert.Equal(t, remediation.Steps, decoded.Steps)
	assert.Equal(t, remediation.StackChanges, decoded.StackChanges)
	assert.Equal(t, remediation.DeployCommand, decoded.DeployCommand)
	assert.Equal(t, remediation.RiskLevel, decoded.RiskLevel)
	assert.Equal(t, remediation.References, decoded.References)
	require.Len(t, decoded.CodeChanges, 1)
}
