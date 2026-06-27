variable "stage" {
  description = "The deployment stage (e.g. dev, staging, prod)."
  type        = string
}

variable "name" {
  description = "Base name for the S3 bucket; the stage is appended to form a unique name."
  type        = string
}
