variable "project" {
  description = "Project name, used in resource names."
  type        = string
}

variable "stage" {
  description = "SDLC environment (dev, staging, prod)."
  type        = string
}

variable "parameters" {
  description = "Environment metadata written under /<project>/<stage>/ in SSM Parameter Store."
  type        = map(string)
  default     = {}
}

variable "kms_key_arn" {
  description = "ARN of a KMS key used to encrypt SecureString parameters. Empty string uses the AWS-managed alias/aws/ssm key instead."
  type        = string
  default     = ""
}
