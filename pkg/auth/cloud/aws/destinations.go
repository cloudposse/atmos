package aws

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ServiceDestinations maps common AWS service aliases to their console URLs.
// This allows users to use shorthand like "s3" instead of full URLs.
var ServiceDestinations = map[string]string{
	// Storage Services.
	"s3":      "https://console.aws.amazon.com/s3",
	"efs":     "https://console.aws.amazon.com/efs",
	"fsx":     "https://console.aws.amazon.com/fsx",
	"glacier": "https://console.aws.amazon.com/glacier",
	"backup":  "https://console.aws.amazon.com/backup",

	// Compute Services.
	"ec2":              "https://console.aws.amazon.com/ec2",
	"lambda":           "https://console.aws.amazon.com/lambda",
	"lightsail":        "https://console.aws.amazon.com/lightsail",
	"batch":            "https://console.aws.amazon.com/batch",
	"elasticbeanstalk": "https://console.aws.amazon.com/elasticbeanstalk",
	"serverless":       "https://console.aws.amazon.com/lambda/home#/applications",

	// Container Services.
	"ecs":     "https://console.aws.amazon.com/ecs",
	"ecr":     "https://console.aws.amazon.com/ecr",
	"eks":     "https://console.aws.amazon.com/eks",
	"fargate": "https://console.aws.amazon.com/ecs",

	// Database Services.
	"rds":         "https://console.aws.amazon.com/rds",
	"dynamodb":    "https://console.aws.amazon.com/dynamodb",
	"elasticache": "https://console.aws.amazon.com/elasticache",
	"neptune":     "https://console.aws.amazon.com/neptune",
	"timestream":  "https://console.aws.amazon.com/timestream",
	"documentdb":  "https://console.aws.amazon.com/docdb",
	"keyspaces":   "https://console.aws.amazon.com/keyspaces",
	"qldb":        "https://console.aws.amazon.com/qldb",
	"memorydb":    "https://console.aws.amazon.com/memorydb",

	// Networking Services.
	"vpc":               "https://console.aws.amazon.com/vpc",
	"cloudfront":        "https://console.aws.amazon.com/cloudfront",
	"route53":           "https://console.aws.amazon.com/route53",
	"apigateway":        "https://console.aws.amazon.com/apigateway",
	"directconnect":     "https://console.aws.amazon.com/directconnect",
	"elb":               "https://console.aws.amazon.com/ec2/v2/home#LoadBalancers",
	"globalaccelerator": "https://console.aws.amazon.com/globalaccelerator",
	"transitgateway":    "https://console.aws.amazon.com/vpc/home#TransitGateways",
	"privatelink":       "https://console.aws.amazon.com/vpc/home#Endpoints",

	// Security & Identity.
	"iam":            "https://console.aws.amazon.com/iam",
	"organizations":  "https://console.aws.amazon.com/organizations",
	"cognito":        "https://console.aws.amazon.com/cognito",
	"secretsmanager": "https://console.aws.amazon.com/secretsmanager",
	"kms":            "https://console.aws.amazon.com/kms",
	"acm":            "https://console.aws.amazon.com/acm",
	"waf":            "https://console.aws.amazon.com/wafv2",
	"shield":         "https://console.aws.amazon.com/shield",
	"guardduty":      "https://console.aws.amazon.com/guardduty",
	"inspector":      "https://console.aws.amazon.com/inspector",
	"macie":          "https://console.aws.amazon.com/macie",
	"securityhub":    "https://console.aws.amazon.com/securityhub",
	"detective":      "https://console.aws.amazon.com/detective",
	"sso":            "https://console.aws.amazon.com/singlesignon",
	"directory":      "https://console.aws.amazon.com/directoryservicev2",

	// Management & Governance.
	"cloudwatch":       "https://console.aws.amazon.com/cloudwatch",
	"cloudtrail":       "https://console.aws.amazon.com/cloudtrail",
	"config":           "https://console.aws.amazon.com/config",
	"cloudformation":   "https://console.aws.amazon.com/cloudformation",
	"servicecatalog":   "https://console.aws.amazon.com/servicecatalog",
	"systems-manager":  "https://console.aws.amazon.com/systems-manager",
	"ssm":              "https://console.aws.amazon.com/systems-manager",
	"opsworks":         "https://console.aws.amazon.com/opsworks",
	"controltower":     "https://console.aws.amazon.com/controltower",
	"license-manager":  "https://console.aws.amazon.com/license-manager",
	"resource-groups":  "https://console.aws.amazon.com/resource-groups",
	"tag-editor":       "https://console.aws.amazon.com/resource-groups/tag-editor",
	"trusted-advisor":  "https://console.aws.amazon.com/trustedadvisor",
	"well-architected": "https://console.aws.amazon.com/wellarchitected",

	// Developer Tools.
	"codecommit":   "https://console.aws.amazon.com/codecommit",
	"codebuild":    "https://console.aws.amazon.com/codebuild",
	"codedeploy":   "https://console.aws.amazon.com/codedeploy",
	"codepipeline": "https://console.aws.amazon.com/codepipeline",
	"codeartifact": "https://console.aws.amazon.com/codeartifact",
	"cloud9":       "https://console.aws.amazon.com/cloud9",
	"xray":         "https://console.aws.amazon.com/xray",

	// Analytics.
	"athena":         "https://console.aws.amazon.com/athena",
	"emr":            "https://console.aws.amazon.com/emr",
	"cloudSearch":    "https://console.aws.amazon.com/cloudsearch",
	"elasticsearch":  "https://console.aws.amazon.com/aos",
	"opensearch":     "https://console.aws.amazon.com/aos",
	"kinesis":        "https://console.aws.amazon.com/kinesis",
	"quicksight":     "https://console.aws.amazon.com/quicksight",
	"glue":           "https://console.aws.amazon.com/glue",
	"lake-formation": "https://console.aws.amazon.com/lakeformation",
	"msk":            "https://console.aws.amazon.com/msk",
	"redshift":       "https://console.aws.amazon.com/redshift",

	// Application Integration.
	"sns":            "https://console.aws.amazon.com/sns",
	"sqs":            "https://console.aws.amazon.com/sqs",
	"eventbridge":    "https://console.aws.amazon.com/events",
	"mq":             "https://console.aws.amazon.com/amazon-mq",
	"step-functions": "https://console.aws.amazon.com/states",
	"appflow":        "https://console.aws.amazon.com/appflow",

	// Migration & Transfer.
	"dms":                   "https://console.aws.amazon.com/dms",
	"datasync":              "https://console.aws.amazon.com/datasync",
	"transfer":              "https://console.aws.amazon.com/transfer",
	"migration-hub":         "https://console.aws.amazon.com/migrationhub",
	"application-discovery": "https://console.aws.amazon.com/discovery",
	"server-migration":      "https://console.aws.amazon.com/servermigration",

	// Cost Management.
	"billing":       "https://console.aws.amazon.com/billing",
	"cost-explorer": "https://console.aws.amazon.com/cost-management/home#/cost-explorer",
	"budgets":       "https://console.aws.amazon.com/billing/home#/budgets",

	// Machine Learning.
	"sagemaker":   "https://console.aws.amazon.com/sagemaker",
	"rekognition": "https://console.aws.amazon.com/rekognition",
	"textract":    "https://console.aws.amazon.com/textract",
	"comprehend":  "https://console.aws.amazon.com/comprehend",
	"translate":   "https://console.aws.amazon.com/translate",
	"polly":       "https://console.aws.amazon.com/polly",
	"transcribe":  "https://console.aws.amazon.com/transcribe",
	"lex":         "https://console.aws.amazon.com/lex",
	"personalize": "https://console.aws.amazon.com/personalize",
	"forecast":    "https://console.aws.amazon.com/forecast",
	"kendra":      "https://console.aws.amazon.com/kendra",
	"bedrock":     "https://console.aws.amazon.com/bedrock",

	// IoT.
	"iot-core":      "https://console.aws.amazon.com/iot",
	"iot-analytics": "https://console.aws.amazon.com/iotanalytics",
	"iot-events":    "https://console.aws.amazon.com/iotevents",
	"greengrass":    "https://console.aws.amazon.com/greengrass",

	// End User Computing.
	"workspaces": "https://console.aws.amazon.com/workspaces",
	"appstream":  "https://console.aws.amazon.com/appstream2",
	"worklink":   "https://console.aws.amazon.com/worklink",

	// Common Aliases.
	"console":   "https://console.aws.amazon.com/console/home",
	"home":      "https://console.aws.amazon.com/console/home",
	"dashboard": "https://console.aws.amazon.com/console/home",
}

// ResolveDestination converts a destination alias to a full console URL.
// If the input is already a URL (starts with http:// or https://), it returns it unchanged.
// Otherwise, it looks up the alias in the ServiceDestinations map.
func ResolveDestination(destination string) (string, error) {
	defer perf.Track(nil, "aws.ResolveDestination")()

	// If destination is empty, return empty (will use default).
	if destination == "" {
		return "", nil
	}

	// If destination is already a full URL, return it unchanged.
	if strings.HasPrefix(destination, "http://") || strings.HasPrefix(destination, "https://") {
		return destination, nil
	}

	// Normalize the alias: lowercase and trim whitespace.
	alias := strings.ToLower(strings.TrimSpace(destination))

	// Look up the alias in the map.
	if url, ok := ServiceDestinations[alias]; ok {
		return url, nil
	}

	// If no match found, return an error with suggestions.
	return "", fmt.Errorf("unknown service alias %q (try 's3', 'ec2', 'cloudformation', etc. or use a full URL)", destination)
}

// GetAvailableAliases returns a sorted list of all available service aliases.
func GetAvailableAliases() []string {
	defer perf.Track(nil, "aws.GetAvailableAliases")()

	aliases := make([]string, 0, len(ServiceDestinations))
	for alias := range ServiceDestinations {
		aliases = append(aliases, alias)
	}
	return aliases
}

// GetAliasByCategory returns aliases grouped by service category.
func GetAliasByCategory() map[string][]string {
	defer perf.Track(nil, "aws.GetAliasByCategory")()

	return map[string][]string{
		"Storage": {
			"s3", "efs", "fsx", "glacier", "backup",
		},
		"Compute": {
			"ec2", "lambda", "lightsail", "batch", "elasticbeanstalk",
		},
		"Containers": {
			"ecs", "ecr", "eks", "fargate",
		},
		"Database": {
			"rds", "dynamodb", "elasticache", "neptune", "timestream",
			"documentdb", "keyspaces", "qldb", "memorydb",
		},
		"Networking": {
			"vpc", "cloudfront", "route53", "apigateway", "directconnect",
			"elb", "globalaccelerator", "transitgateway", "privatelink",
		},
		"Security": {
			"iam", "organizations", "cognito", "secretsmanager", "kms",
			"acm", "waf", "shield", "guardduty", "inspector", "macie",
			"securityhub", "detective", "sso", "directory",
		},
		"Management": {
			"cloudwatch", "cloudtrail", "config", "cloudformation",
			"servicecatalog", "systems-manager", "ssm", "opsworks",
			"controltower", "license-manager", "resource-groups",
			"tag-editor", "trusted-advisor", "well-architected",
		},
		"Developer Tools": {
			"codecommit", "codebuild", "codedeploy", "codepipeline",
			"codeartifact", "cloud9", "xray",
		},
		"Analytics": {
			"athena", "emr", "elasticsearch", "opensearch", "kinesis",
			"quicksight", "glue", "lake-formation", "msk", "redshift",
		},
		"Application Integration": {
			"sns", "sqs", "eventbridge", "mq", "step-functions", "appflow",
		},
		"Cost Management": {
			"billing", "cost-explorer", "budgets",
		},
		"Machine Learning": {
			"sagemaker", "rekognition", "textract", "comprehend", "translate",
			"polly", "transcribe", "lex", "personalize", "forecast", "kendra", "bedrock",
		},
		"IoT": {
			"iot-core", "iot-analytics", "iot-events", "greengrass",
		},
	}
}
