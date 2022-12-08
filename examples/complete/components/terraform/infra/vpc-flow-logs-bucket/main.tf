module "flow_logs_s3_bucket" {
  source  = "cloudposse/vpc-flow-logs-s3-bucket/aws"
  version = "0.12.0"

  lifecycle_prefix                   = var.lifecycle_prefix
  lifecycle_tags                     = var.lifecycle_tags
  lifecycle_rule_enabled             = var.lifecycle_rule_enabled
  noncurrent_version_expiration_days = var.noncurrent_version_expiration_days
  noncurrent_version_transition_days = var.noncurrent_version_transition_days
  standard_transition_days           = var.standard_transition_days
  glacier_transition_days            = var.glacier_transition_days
  expiration_days                    = var.expiration_days
  traffic_type                       = var.traffic_type
  force_destroy                      = var.force_destroy
  arn_format                         = var.arn_format
  flow_log_enabled                   = false

  context = module.this.context
}
