locals {
  enabled            = module.this.enabled
  is_on_demand       = local.enabled && var.billing_mode == "PAY_PER_REQUEST"
  autoscaler_enabled = !local.is_on_demand && var.autoscaler_enabled
}

module "dynamodb_table" {
  source  = "cloudposse/dynamodb/aws"
  version = "0.37.0"

  table_name          = var.table_name
  billing_mode        = var.billing_mode
  replicas            = var.replicas
  dynamodb_attributes = var.dynamodb_attributes
  import_table        = var.import_table

  global_secondary_index_map = var.global_secondary_index_map
  local_secondary_index_map  = var.local_secondary_index_map

  hash_key       = var.hash_key
  hash_key_type  = var.hash_key_type
  range_key      = var.range_key
  range_key_type = var.range_key_type

  enable_autoscaler            = local.autoscaler_enabled
  autoscale_write_target       = local.autoscaler_enabled ? var.autoscale_write_target : null
  autoscale_read_target        = local.autoscaler_enabled ? var.autoscale_read_target : null
  autoscale_min_read_capacity  = local.autoscaler_enabled ? var.autoscale_min_read_capacity : null
  autoscale_max_read_capacity  = local.autoscaler_enabled ? var.autoscale_max_read_capacity : null
  autoscale_min_write_capacity = local.autoscaler_enabled ? var.autoscale_min_write_capacity : null
  autoscale_max_write_capacity = local.autoscaler_enabled ? var.autoscale_max_write_capacity : null
  autoscaler_attributes        = local.autoscaler_enabled ? var.autoscaler_attributes : []
  autoscaler_tags              = local.autoscaler_enabled ? var.autoscaler_tags : null

  enable_encryption                  = var.encryption_enabled
  server_side_encryption_kms_key_arn = var.server_side_encryption_kms_key_arn

  enable_streams   = var.streams_enabled
  stream_view_type = var.stream_view_type

  ttl_enabled   = var.ttl_enabled
  ttl_attribute = var.ttl_attribute

  enable_point_in_time_recovery = var.point_in_time_recovery_enabled

  deletion_protection_enabled = var.deletion_protection_enabled

  context = module.this.context
}
