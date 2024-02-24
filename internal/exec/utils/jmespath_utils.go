// https://github.com/jmespath/go-jmespath
// https://jmespath.org
// https://jmespath.org/tutorial.html
// https://jmespath.site/main
//
// AWS CLI and Azure CLI support JMESPath:
// https://docs.aws.amazon.com/cli/latest/userguide/cli-usage-filter.html
// https://opensourceconnections.com/blog/2015/07/27/advanced-aws-cli-jmespath-query/
// https://learn.microsoft.com/en-us/cli/azure/query-azure-cli?tabs=concepts%2Cbash

package utils

import (
	"github.com/jmespath/go-jmespath"
)

// EvaluateJmesPath evaluate a JMESPath expression
func EvaluateJmesPath(expression string, data any) (any, error) {
	return jmespath.Search(expression, data)
}
