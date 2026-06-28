variable "region" {
  type        = string
  description = "AWS region. Consumed by the AWS provider; supplied by the region mixin."
  default     = ""
}

variable "namespace" {
  type        = string
  description = "Organization namespace (e.g. `acme`)."
  default     = ""
}

variable "tenant" {
  type        = string
  description = "Tenant/OU name (e.g. `plat`)."
  default     = ""
}

variable "environment" {
  type        = string
  description = "Environment/region code (e.g. `ue2`)."
  default     = ""
}

variable "stage" {
  type        = string
  description = "Stage/account name (e.g. `dev`)."
  default     = ""
}

variable "name" {
  type        = string
  description = "Component name, used to build the resource identifier."
}

variable "bucket_id" {
  type        = string
  description = "Name of the S3 bucket coordinate injected from the s3-bucket component (required)."
}

variable "table_name" {
  type        = string
  description = "Name of the DynamoDB table coordinate injected from the dynamodb-table component (required)."
}

variable "topic_arn" {
  type        = string
  description = "ARN of the SNS topic coordinate injected from the sns-topic component (required)."
}

variable "queue_url" {
  type        = string
  description = "URL of the SQS queue coordinate injected from the sqs-queue component (required)."
}

variable "kms_key_arn" {
  type        = string
  description = "ARN of the KMS key used to encrypt SecureString parameters. When empty, the default AWS-managed key is used."
  default     = ""
}

variable "api_key" {
  type        = string
  description = "Secret API key written to SSM as a SecureString parameter."
  sensitive   = true
  default     = ""
}

variable "db_password" {
  type        = string
  description = "Secret database password written to SSM as a SecureString parameter."
  sensitive   = true
  default     = ""
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to all resources."
  default     = {}
}
