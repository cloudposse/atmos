package security

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/securityhub"
	shtypes "github.com/aws/aws-sdk-go-v2/service/securityhub/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// mockSecurityHubClient implements SecurityHubAPI for testing.
type mockSecurityHubClient struct {
	findings  []shtypes.AwsSecurityFinding
	standards []shtypes.StandardsSubscription
	err       error
}

func (m *mockSecurityHubClient) GetFindings(_ context.Context, _ *securityhub.GetFindingsInput, _ ...func(*securityhub.Options)) (*securityhub.GetFindingsOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &securityhub.GetFindingsOutput{
		Findings: m.findings,
	}, nil
}

func (m *mockSecurityHubClient) GetEnabledStandards(_ context.Context, _ *securityhub.GetEnabledStandardsInput, _ ...func(*securityhub.Options)) (*securityhub.GetEnabledStandardsOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &securityhub.GetEnabledStandardsOutput{
		StandardsSubscriptions: m.standards,
	}, nil
}

func (m *mockSecurityHubClient) DescribeStandardsControls(_ context.Context, _ *securityhub.DescribeStandardsControlsInput, _ ...func(*securityhub.Options)) (*securityhub.DescribeStandardsControlsOutput, error) {
	return &securityhub.DescribeStandardsControlsOutput{}, nil
}

func TestNormalizeSecurityHubFinding(t *testing.T) {
	tests := []struct {
		name     string
		input    shtypes.AwsSecurityFinding
		wantID   string
		wantSev  Severity
		wantSrc  Source
		wantARN  string
		wantType string
	}{
		{
			name: "critical finding from Security Hub",
			input: shtypes.AwsSecurityFinding{
				Id:          aws.String("arn:aws:securityhub:us-east-1:123:finding/abc"),
				Title:       aws.String("S3 Bucket Public Access"),
				Description: aws.String("S3 bucket has public access enabled"),
				ProductName: aws.String("Security Hub"),
				Severity: &shtypes.Severity{
					Label: shtypes.SeverityLabelCritical,
				},
				AwsAccountId: aws.String("123456789012"),
				Resources: []shtypes.Resource{
					{
						Id:     aws.String("arn:aws:s3:::my-public-bucket"),
						Type:   aws.String("AwsS3Bucket"),
						Region: aws.String("us-east-1"),
					},
				},
				CreatedAt: aws.String("2026-03-01T10:00:00Z"),
				UpdatedAt: aws.String("2026-03-09T12:00:00Z"),
			},
			wantID:   "arn:aws:securityhub:us-east-1:123:finding/abc",
			wantSev:  SeverityCritical,
			wantSrc:  SourceSecurityHub,
			wantARN:  "arn:aws:s3:::my-public-bucket",
			wantType: "AwsS3Bucket",
		},
		{
			name: "GuardDuty finding",
			input: shtypes.AwsSecurityFinding{
				Id:          aws.String("gd-finding-1"),
				Title:       aws.String("Unusual API Activity"),
				ProductName: aws.String("GuardDuty"),
				Severity: &shtypes.Severity{
					Label: shtypes.SeverityLabelHigh,
				},
				Resources: []shtypes.Resource{
					{
						Id:     aws.String("arn:aws:ec2:us-west-2:123:instance/i-12345"),
						Type:   aws.String("AwsEc2Instance"),
						Region: aws.String("us-west-2"),
					},
				},
			},
			wantID:   "gd-finding-1",
			wantSev:  SeverityHigh,
			wantSrc:  SourceGuardDuty,
			wantARN:  "arn:aws:ec2:us-west-2:123:instance/i-12345",
			wantType: "AwsEc2Instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeSecurityHubFinding(&tt.input)
			assert.Equal(t, tt.wantID, result.ID)
			assert.Equal(t, tt.wantSev, result.Severity)
			assert.Equal(t, tt.wantSrc, result.Source)
			assert.Equal(t, tt.wantARN, result.ResourceARN)
			assert.Equal(t, tt.wantType, result.ResourceType)
		})
	}
}

func TestFetchFindings_WithMock(t *testing.T) {
	mock := &mockSecurityHubClient{
		findings: []shtypes.AwsSecurityFinding{
			{
				Id:           aws.String("finding-1"),
				Title:        aws.String("Test Finding 1"),
				ProductName:  aws.String("Security Hub"),
				Severity:     &shtypes.Severity{Label: shtypes.SeverityLabelHigh},
				AwsAccountId: aws.String("123456789012"),
				Resources: []shtypes.Resource{
					{
						Id:     aws.String("arn:aws:s3:::test-bucket"),
						Type:   aws.String("AwsS3Bucket"),
						Region: aws.String("us-east-1"),
					},
				},
			},
			{
				Id:           aws.String("finding-2"),
				Title:        aws.String("Test Finding 2"),
				ProductName:  aws.String("Inspector"),
				Severity:     &shtypes.Severity{Label: shtypes.SeverityLabelCritical},
				AwsAccountId: aws.String("123456789012"),
				Resources: []shtypes.Resource{
					{
						Id:     aws.String("arn:aws:ec2:us-east-1:123:instance/i-abc"),
						Type:   aws.String("AwsEc2Instance"),
						Region: aws.String("us-east-1"),
					},
				},
			},
		},
	}

	fetcher := &awsFindingFetcher{
		atmosConfig: &schema.AtmosConfiguration{},
		clients:     newAWSClientCache(),
	}
	// Pre-populate cached client with mock.
	fetcher.clients.securityHub["us-east-1"] = mock

	opts := QueryOptions{
		Severity:    []Severity{SeverityCritical, SeverityHigh},
		MaxFindings: 50,
	}

	findings, err := fetcher.FetchFindings(context.Background(), &opts)
	require.NoError(t, err)
	assert.Len(t, findings, 2)
	assert.Equal(t, "finding-1", findings[0].ID)
	assert.Equal(t, SeverityHigh, findings[0].Severity)
	assert.Equal(t, SourceSecurityHub, findings[0].Source)
	assert.Equal(t, "finding-2", findings[1].ID)
	assert.Equal(t, SeverityCritical, findings[1].Severity)
	assert.Equal(t, SourceInspector, findings[1].Source)
}

func TestFetchFindings_MaxLimit(t *testing.T) {
	// Create 5 findings.
	var findings []shtypes.AwsSecurityFinding
	for i := 0; i < 5; i++ {
		findings = append(findings, shtypes.AwsSecurityFinding{
			Id:          aws.String("finding-" + string(rune('a'+i))),
			Title:       aws.String("Finding"),
			ProductName: aws.String("Security Hub"),
			Severity:    &shtypes.Severity{Label: shtypes.SeverityLabelHigh},
			Resources: []shtypes.Resource{
				{Id: aws.String("arn:aws:s3:::bucket-" + string(rune('a'+i)))},
			},
		})
	}

	mock := &mockSecurityHubClient{findings: findings}
	fetcher := &awsFindingFetcher{
		atmosConfig: &schema.AtmosConfiguration{},
		clients:     newAWSClientCache(),
	}
	fetcher.clients.securityHub["us-east-1"] = mock

	// Limit to 3.
	opts := QueryOptions{MaxFindings: 3}
	result, err := fetcher.FetchFindings(context.Background(), &opts)
	require.NoError(t, err)
	assert.Len(t, result, 3)
}

func TestDetectSource(t *testing.T) {
	tests := []struct {
		productName string
		want        Source
	}{
		{"Security Hub", SourceSecurityHub},
		{"AWS Security Hub", SourceSecurityHub},
		{"GuardDuty", SourceGuardDuty},
		{"Amazon Inspector", SourceInspector},
		{"AWS Config", SourceConfig},
		{"Amazon Macie", SourceMacie},
		{"IAM Access Analyzer", SourceAccessAnalyzer},
		{"Unknown Service", SourceSecurityHub},
	}

	for _, tt := range tests {
		t.Run(tt.productName, func(t *testing.T) {
			finding := &shtypes.AwsSecurityFinding{
				ProductName: aws.String(tt.productName),
			}
			assert.Equal(t, tt.want, detectSource(finding))
		})
	}
}

func TestFrameworkToStandardID(t *testing.T) {
	tests := []struct {
		framework string
		want      string
	}{
		{"cis-aws", "cis-aws-foundations-benchmark"},
		{"pci-dss", "pci-dss"},
		{"nist", "nist-800-53"},
		{"soc2", "soc2"},
		{"hipaa", "hipaa"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.framework, func(t *testing.T) {
			assert.Equal(t, tt.want, frameworkToStandardID(tt.framework))
		})
	}
}

func TestBuildComplianceReport(t *testing.T) {
	findings := []Finding{
		{
			ID:                 "ctrl-1",
			Title:              "MFA not enabled",
			Severity:           SeverityCritical,
			ComplianceStandard: "CIS.1.2",
		},
		{
			ID:                 "ctrl-2",
			Title:              "Root account used",
			Severity:           SeverityHigh,
			ComplianceStandard: "CIS.1.1",
		},
		{
			ID:                 "ctrl-3",
			Title:              "Another finding for CIS.1.2",
			Severity:           SeverityCritical,
			ComplianceStandard: "CIS.1.2", // Duplicate control.
		},
	}

	report := buildComplianceReport(findings, "cis-aws", "CIS AWS Foundations Benchmark", "prod-us-east-1")

	assert.Equal(t, "cis-aws", report.Framework)
	assert.Equal(t, "CIS AWS Foundations Benchmark", report.FrameworkTitle)
	assert.Equal(t, "prod-us-east-1", report.Stack)
	assert.Equal(t, 2, report.FailingControls) // Deduplicated by control ID.
	assert.Len(t, report.FailingDetails, 2)
}
