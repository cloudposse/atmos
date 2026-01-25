locals {
  enabled      = module.this.enabled
  sops_enabled = local.enabled && length(var.sops_source_file) > 0

  sops_yaml     = local.sops_enabled ? yamldecode(data.sops_file.source[0].raw) : ""
  secret_params = local.sops_enabled ? nonsensitive(local.sops_yaml[var.sops_source_key]) : {}

  secret_params_normalized = {
    for key, value in local.secret_params :
    key => {
      name        = key
      value       = value
      description = "SecureString param created from ssm-parameters component from SOPS source file: ${var.sops_source_file} from key at ${var.sops_source_key}"
      overwrite   = true
      type        = "SecureString"
    }
  }

  params     = local.enabled ? merge(var.params, local.secret_params_normalized) : {}
  param_keys = keys(local.params)

  params_apply_value_changes = {
    for k, v in local.params : k => v
    if !try(v.ignore_value_changes, false)
  }

  params_ignore_value_changes = {
    for k, v in local.params : k => v
    if try(v.ignore_value_changes, false)
  }
}

data "sops_file" "source" {
  count       = local.sops_enabled ? 1 : 0
  source_file = "${path.root}/${var.sops_source_file}"
}

resource "aws_ssm_parameter" "destination" {
  for_each = local.params_apply_value_changes

  name        = each.key
  description = each.value.description
  tier        = each.value.tier
  type        = each.value.type
  key_id      = length(var.kms_arn) > 0 ? var.kms_arn : null
  value       = each.value.value
  overwrite   = each.value.overwrite

  tags = module.this.tags
}

resource "aws_ssm_parameter" "destination_ignored" {
  for_each = local.params_ignore_value_changes

  name        = each.key
  description = each.value.description
  tier        = each.value.tier
  type        = each.value.type
  key_id      = length(var.kms_arn) > 0 ? var.kms_arn : null
  value       = each.value.value
  overwrite   = each.value.overwrite

  tags = module.this.tags

  lifecycle {
    ignore_changes = [value]
  }
}
