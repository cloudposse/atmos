package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDestination(t *testing.T) {
	tests := []struct {
		name          string
		destination   string
		expectedURL   string
		expectError   bool
		errorContains string
	}{
		{
			name:        "empty destination",
			destination: "",
			expectedURL: "",
			expectError: false,
		},
		{
			name:        "full HTTP URL",
			destination: "http://example.com",
			expectedURL: "http://example.com",
			expectError: false,
		},
		{
			name:        "full HTTPS URL",
			destination: "https://console.aws.amazon.com/custom-page",
			expectedURL: "https://console.aws.amazon.com/custom-page",
			expectError: false,
		},
		{
			name:        "s3 alias lowercase",
			destination: "s3",
			expectedURL: "https://console.aws.amazon.com/s3",
			expectError: false,
		},
		{
			name:        "S3 alias uppercase",
			destination: "S3",
			expectedURL: "https://console.aws.amazon.com/s3",
			expectError: false,
		},
		{
			name:        "ec2 alias",
			destination: "ec2",
			expectedURL: "https://console.aws.amazon.com/ec2",
			expectError: false,
		},
		{
			name:        "cloudformation alias",
			destination: "cloudformation",
			expectedURL: "https://console.aws.amazon.com/cloudformation",
			expectError: false,
		},
		{
			name:        "lambda alias",
			destination: "lambda",
			expectedURL: "https://console.aws.amazon.com/lambda",
			expectError: false,
		},
		{
			name:        "dynamodb alias",
			destination: "dynamodb",
			expectedURL: "https://console.aws.amazon.com/dynamodb",
			expectError: false,
		},
		{
			name:        "iam alias",
			destination: "iam",
			expectedURL: "https://console.aws.amazon.com/iam",
			expectError: false,
		},
		{
			name:        "vpc alias",
			destination: "vpc",
			expectedURL: "https://console.aws.amazon.com/vpc",
			expectError: false,
		},
		{
			name:        "cloudwatch alias",
			destination: "cloudwatch",
			expectedURL: "https://console.aws.amazon.com/cloudwatch",
			expectError: false,
		},
		{
			name:        "ssm alias (systems manager)",
			destination: "ssm",
			expectedURL: "https://console.aws.amazon.com/systems-manager",
			expectError: false,
		},
		{
			name:        "eks alias",
			destination: "eks",
			expectedURL: "https://console.aws.amazon.com/eks",
			expectError: false,
		},
		{
			name:        "rds alias",
			destination: "rds",
			expectedURL: "https://console.aws.amazon.com/rds",
			expectError: false,
		},
		{
			name:        "secretsmanager alias",
			destination: "secretsmanager",
			expectedURL: "https://console.aws.amazon.com/secretsmanager",
			expectError: false,
		},
		{
			name:        "kms alias",
			destination: "kms",
			expectedURL: "https://console.aws.amazon.com/kms",
			expectError: false,
		},
		{
			name:        "cloudtrail alias",
			destination: "cloudtrail",
			expectedURL: "https://console.aws.amazon.com/cloudtrail",
			expectError: false,
		},
		{
			name:        "route53 alias",
			destination: "route53",
			expectedURL: "https://console.aws.amazon.com/route53",
			expectError: false,
		},
		{
			name:        "sagemaker alias",
			destination: "sagemaker",
			expectedURL: "https://console.aws.amazon.com/sagemaker",
			expectError: false,
		},
		{
			name:        "bedrock alias",
			destination: "bedrock",
			expectedURL: "https://console.aws.amazon.com/bedrock",
			expectError: false,
		},
		{
			name:        "alias with whitespace",
			destination: "  s3  ",
			expectedURL: "https://console.aws.amazon.com/s3",
			expectError: false,
		},
		{
			name:        "mixed case alias",
			destination: "DynamoDB",
			expectedURL: "https://console.aws.amazon.com/dynamodb",
			expectError: false,
		},
		{
			name:          "unknown alias",
			destination:   "unknown-service",
			expectError:   true,
			errorContains: "unknown service alias",
		},
		{
			name:          "invalid alias",
			destination:   "not-a-real-service",
			expectError:   true,
			errorContains: "unknown service alias",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := ResolveDestination(tt.destination)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}

func TestGetAvailableAliases(t *testing.T) {
	aliases := GetAvailableAliases()

	// Should return all aliases.
	assert.NotEmpty(t, aliases)
	assert.GreaterOrEqual(t, len(aliases), 50, "Should have at least 50 service aliases")

	// Check for some common services.
	aliasMap := make(map[string]bool)
	for _, alias := range aliases {
		aliasMap[alias] = true
	}

	commonServices := []string{"s3", "ec2", "lambda", "dynamodb", "iam", "vpc", "cloudformation"}
	for _, service := range commonServices {
		assert.True(t, aliasMap[service], "Should include %s alias", service)
	}
}

func TestGetAliasByCategory(t *testing.T) {
	categories := GetAliasByCategory()

	// Should have multiple categories.
	assert.NotEmpty(t, categories)
	assert.GreaterOrEqual(t, len(categories), 10, "Should have at least 10 categories")

	// Check specific categories exist.
	expectedCategories := []string{
		"Storage",
		"Compute",
		"Database",
		"Networking",
		"Security",
		"Management",
		"Analytics",
	}

	for _, category := range expectedCategories {
		assert.Contains(t, categories, category, "Should have %s category", category)
		assert.NotEmpty(t, categories[category], "Category %s should have services", category)
	}

	// Check specific services are in correct categories.
	assert.Contains(t, categories["Storage"], "s3")
	assert.Contains(t, categories["Compute"], "ec2")
	assert.Contains(t, categories["Compute"], "lambda")
	assert.Contains(t, categories["Database"], "dynamodb")
	assert.Contains(t, categories["Database"], "rds")
	assert.Contains(t, categories["Security"], "iam")
	assert.Contains(t, categories["Networking"], "vpc")
	assert.Contains(t, categories["Management"], "cloudformation")
}

func TestServiceDestinationsCompleteness(t *testing.T) {
	// Verify all services in ServiceDestinations map are valid URLs.
	for alias, url := range ServiceDestinations {
		assert.NotEmpty(t, url, "URL for alias %s should not be empty", alias)
		assert.Contains(t, url, "https://console.aws.amazon.com/", "URL for alias %s should be AWS console URL", alias)
	}
}

func TestResolveDestination_Integration(t *testing.T) {
	// Test that all aliases in categories can be resolved.
	categories := GetAliasByCategory()

	for categoryName, aliases := range categories {
		for _, alias := range aliases {
			t.Run(categoryName+"/"+alias, func(t *testing.T) {
				url, err := ResolveDestination(alias)
				require.NoError(t, err, "Should resolve alias %s in category %s", alias, categoryName)
				assert.NotEmpty(t, url)
				assert.Contains(t, url, "https://console.aws.amazon.com/")
			})
		}
	}
}
